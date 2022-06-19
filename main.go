package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/mmcdole/gofeed"

	"gopkg.in/yaml.v2"

	redisbloom "github.com/RedisBloom/redisbloom-go"
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

type RedisConfig struct {
	Host string
}

type Config struct {
	Feeds   []Feed
	Outputs []Output
	Redis   RedisConfig
}

func loadConfig(configFile string) Config {
	b, err := os.ReadFile(configFile)
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

type ItemFilter func(gofeed.Item, gofeed.Feed) bool

const BLOOM_FILTER_KEY = "items-exist"

func main() {
	startTime := time.Now()
	var newOnly bool
	flag.BoolVar(&newOnly, "newOnly", false, "Only process items published after program start time. Default false")
	flag.Usage = func() {
		fmt.Println("Usage: feed-ingress [-newOnly] CONFIG_FILE")
		flag.PrintDefaults()
	}
	flag.Parse()

	var configFile string
	if flag.NArg() == 0 {
		configFile = "config.yaml"
	} else if flag.NArg() == 1 {
		configFile = flag.Arg(0)
	} else {
		flag.Usage()
		os.Exit(1)
	}

	config := loadConfig(configFile)
	log.Println("Config loaded:", config)

	// Global filtering function that can be defined according to the config/CLI parameters
	filterFunc := func(item gofeed.Item, _ gofeed.Feed) bool {
		if newOnly {
			return startTime.Before(*item.PublishedParsed)
		}
		return true
	}

	// It's a bit hacky, but it's easier to just do round-trip conversion to get
	// everything converted cleanly
	// log.Println("Parsing from", string(configYaml))
	writers := parseWriters(config)

	// We could probably just use regular keys in redis to track everything, but
	// everyone loves bloom filters
	redisClient := redisbloom.NewClient(config.Redis.Host, "nohelp", nil)
	// We could reserve if we wanted to. Apparently BF.ADD will create the filter
	// if it doesn't exist and the filter should expand its capacity as needed (though
	// the docs don't seem to give the parameters for auto-creation and performance
	// will degrade through expansions)
	// redisClient.Reserve(BLOOM_FILTER_KEY, 0.0001, 1e9)

	for _, writer := range writers {
		log.Printf("%#v\n", writer)
	}

	for {
		// TODO Each one should be its own goroutine
		for _, feed := range config.Feeds {
			go processFeed(feed, redisClient, writers, filterFunc)
		}
		time.Sleep(15 * time.Minute)
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
			log.Println("Parsing s3 output")
			var writer S3Output
			err = yaml.Unmarshal(configYaml, &writer)
			if err == nil {
				writers = append(writers, writer)
			}
		case "file":
			log.Println("Parsing file output")
			var writer FileOutput
			err = yaml.Unmarshal(configYaml, &writer)
			if err == nil {
				writers = append(writers, writer)
			}
		case "kafka":
			log.Println("Parsing kafka output")
			var writer KafkaOutput
			err = yaml.Unmarshal(configYaml, &writer)
			if err == nil {
				writers = append(writers, writer)
			}
		case "discord":
			log.Println("Parsing discord webhook output")
			writer := NewDiscordWebhookOutput(output.Config["url"])
			writers = append(writers, writer)
		default:
			log.Fatal("Could not parse kind: ", output.Kind)
		}

		if err != nil {
			log.Fatal("Failed to provider output", output)
		}
	}
	log.Println("Parsed writers", writers)
	return writers
}

func processFeed(feed Feed, redisClient *redisbloom.Client, writers []Writer, filterFunc ItemFilter) {
	fp := gofeed.NewParser()
	log.Println("Processing feed", feed)
	parsed, err := fp.ParseURL(string(feed))
	if err != nil {
		log.Println("Unable to parse feed", feed, err)
		return
	}
	for _, item := range parsed.Items {
		if filterFunc(*item, *parsed) {
			// TODO This should probably be in a goroutine, but we need to use channels and such
			go processItem(parsed, *item, redisClient, writers)
		}
	}
}

func processItem(feed *gofeed.Feed, item gofeed.Item, redisClient *redisbloom.Client, writers []Writer) {
	// Hash the GUID to make a uniform format
	// We could also base64 it so it's reversible, but then the filenames may not be the same length
	identifier := blake2b.Sum256([]byte(item.GUID))
	log.Printf("Item: %s - %s, GUID: %s\n", feed.Title, item.Title, item.GUID)
	identifierStr := fmt.Sprintf("%x", identifier)

	// Note that errors are ignored here. It's not the end of the world if we re-process items, though
	// it might be worth doing something in the case of many errors
	if exists, err := redisClient.Exists(BLOOM_FILTER_KEY, identifierStr); exists {
		log.Printf("Already processed %x\n", identifier)
		return
	} else if err != nil {
		log.Printf("Redis error: %s\n", err)
		// It could be debated whether or not you want to process an item here anyway
		// This takes the assumption that if the error is transient, the next run of the feed will try
		// to process the item again because the key isn't stored. If the connection has been restored
		// at that point, the item will be processed and the key will be stored properly
		return
	} else {
		redisClient.Add(BLOOM_FILTER_KEY, identifierStr)
	}

	for _, writer := range writers {
		// Note that there's a bit of an assumption here that writers are thread-safe
		// (and potentially feeds/items, but we shouldn't be modifying those)
		// Really it would be safer to have workers and channels
		go func(writer Writer) {
			log.Printf("Writing with %T\n", writer)
			if err := writer.Write(feed, item, identifierStr); err != nil {
				log.Println("Error writing with", writer, err)
			}
		}(writer)
	}
}
