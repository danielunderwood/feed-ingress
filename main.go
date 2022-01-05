package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/mmcdole/gofeed"

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
