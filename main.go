package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/mmcdole/gofeed"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"gopkg.in/yaml.v2"
)

type Feed string

type Writer interface {
	Write(feed *gofeed.Feed, item gofeed.Item, identifier string) error
}

type Output struct {
	Kind string
	// TODO Not sure how, but maybe this can be specialized on kind
	Config map[string]string
}

type S3Output struct {
	Endpoint     string
	Region       string
	AccessKeyId  string
	AccessSecret string
	Bucket       string
	KeyFormat    string
}

func (out S3Output) Write(feed *gofeed.Feed, item gofeed.Item, identifier string) error {
	// https://help.backblaze.com/hc/en-us/articles/360047629713-Using-the-AWS-Go-SDK-with-B2
	// Yes, it is awful
	// Also, for actual AWS, you may have things like IAM for auth
	bucket := aws.String(out.Bucket)

	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(out.AccessKeyId, out.AccessSecret, ""),
		Endpoint:         aws.String(out.Endpoint),
		Region:           aws.String(out.Region),
		S3ForcePathStyle: aws.Bool(true),
	}
	newSession := session.New(s3Config)
	s3Client := s3.New(newSession)

	data, err := json.Marshal(item)

	// TODO This should probably go in initialization
	pathTemplate, err := template.New(out.KeyFormat).Parse(out.KeyFormat)
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	err = pathTemplate.Execute(&buffer, item)
	if err != nil {
		return err
	}
	keyPrefix := buffer.String()
	key := fmt.Sprintf("%s/%s-%s.json", keyPrefix, feed.Title, identifier)
	_, err = s3Client.PutObject(&s3.PutObjectInput{
		Body:   bytes.NewReader(data),
		Bucket: bucket,
		Key:    &key,
	})
	if err != nil {
		fmt.Printf("Failed to upload object %s/%s, %s\n", *bucket, key, err.Error())
		return err
	} else {
		fmt.Printf("Successfully uploaded key %s\n", key)
	}
	return nil
}

type FileOutput struct {
	PathFormat string
}

func (out FileOutput) Write(feed *gofeed.Feed, item gofeed.Item, identifier string) error {
	// TODO This should probably go in initialization
	pathTemplate, err := template.New(out.PathFormat).Parse(out.PathFormat)
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	err = pathTemplate.Execute(&buffer, item)
	if err != nil {
		return err
	}
	path := buffer.String()

	err = os.MkdirAll(path, 0o0750)
	if err != nil {
		fmt.Println("ERROR: Could not create ", path, err)
		return err
	}

	// Prefix for data files to identify their source. We should probably be careful to make sure this
	// is something that's reasonable to create a file with, but whatever
	prefix := feed.Title
	file := fmt.Sprintf("%s/%s-%s.json", path, prefix, identifier)
	fmt.Println("Saving to ", file)
	data, err := json.Marshal(item)
	// TODO gzip these files. It shouldn't be too difficult, but the API is a bit weird
	// var compressedWriter
	// writer := gzip.NewWriter(compressedWriter)
	// defer writer.Close()
	// defer compressedWriter.Close()
	// writer.Write(data)
	// writer.Flush()
	// compressedData := compressedWriter.
	if err != nil {
		fmt.Println("ERROR: Could not marshal", item)
		return err
	}
	err = os.WriteFile(file, data, 0o0640)
	if err != nil {
		fmt.Println("ERROR: Failed to write", string(data))
		return err
	}
	return nil
}

type Config struct {
	Feeds   []Feed
	Outputs []Output
}

func loadConfig() Config {
	b, err := os.ReadFile("./config.yaml")
	if err != nil {
		log.Panic("Could not read config file", err)
	}
	var config Config
	err = yaml.Unmarshal(b, &config)
	if err != nil {
		log.Panic("Cloud not parse config file", err)
	}

	return config
}

func main() {
	config := loadConfig()
	fmt.Println("Config loaded:", config)

	// It's a bit hacky, but it's easier to just do round-trip conversion to get
	// everything converted cleanly
	// fmt.Println("Parsing from", string(configYaml))
	writers := parseWriters(config)

	for _, writer := range writers {
		fmt.Printf("%#v\n", writer)
	}

	// This could be a set, but it looks like go doesn't have a native set type, so might as well
	// store the processed time
	// Really, we could use redis to avoid everything getting reset on a restart (or we could just dump
	//  the map to a file)
	processed := make(map[[32]byte]time.Time)
	for {
		// TODO Each one should be its own goroutine
		for _, feed := range config.Feeds {
			processFeed(feed, processed, writers)
		}
		time.Sleep(1 * time.Minute)
	}
}

func parseWriters(config Config) []Writer {
	writers := make([]Writer, 0)
	for _, output := range config.Outputs {

		configYaml, err := yaml.Marshal(output.Config)
		if err != nil {
			log.Fatal("Could not parse", configYaml)
		}

		switch strings.ToLower(output.Kind) {
		case "s3":
			fmt.Println("Parsing s3 output")
			var writer S3Output
			err = yaml.Unmarshal(configYaml, &writer)
			if err == nil {
				writers = append(writers, writer)
			}
		case "file":
			fmt.Println("Parsing file output")
			var writer FileOutput
			err = yaml.Unmarshal(configYaml, &writer)
			if err == nil {
				writers = append(writers, writer)
			}
		default:
			log.Fatal("Could not parse kind", output.Kind)
		}

		if err != nil {
			log.Fatal("Failed to provider output", output)
		}
	}
	return writers
}

func processFeed(feed Feed, processed map[[32]byte]time.Time, writers []Writer) {
	fp := gofeed.NewParser()
	fmt.Println("Processing feed", feed)
	parsed, _ := fp.ParseURL(string(feed))
	for _, item := range parsed.Items {
		// TODO This should probably be in a goroutine, but we need to use channels and such
		processItem(parsed, *item, processed, writers)
	}
}

func processItem(feed *gofeed.Feed, item gofeed.Item, processed map[[32]byte]time.Time, writers []Writer) {
	// Hash the GUID to make a uniform format
	// We could also base64 it so it's reversible, but then the filenames may not be the same length
	identifier := blake2b.Sum256([]byte(item.GUID))
	if _, exists := processed[identifier]; exists {
		fmt.Printf("Already processed %x\n", identifier)
		return
	} else {
		processed[identifier] = time.Now().UTC()
	}

	for _, writer := range writers {
		// Note that there's a bit of an assumption here that writers are thread-safe
		// (and potentially feeds/items, but we shouldn't be modifying those)
		// Really it would be safer to have workers and channels
		go func(writer Writer) {
			fmt.Printf("Writing with %T\n", writer)
			if err := writer.Write(feed, item, fmt.Sprintf("%x", identifier)); err != nil {
				fmt.Println("Error writing with", writer, err)
			}
		}(writer)
	}
}
