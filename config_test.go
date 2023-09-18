package main

import (
	"fmt"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	cases := map[string]Config{
		"empty.yaml": {},
		"all-providers.yaml": {
			Feeds: []Feed{"example.com/feed.xml"},
			Outputs: []Output{
				{
					Kind: "s3",
					Config: map[string]string{
						"endpoint":     "https://s3.example.com",
						"region":       "test-region",
						"accesskeyid":  "test-key",
						"accesssecret": "test-secret",
						"bucket":       "test-bucket",
						"keyformat":    "{{ .PublishedParsed.UTC.Year }}/{{ .PublishedParsed.UTC.Month }}/{{ .PublishedParsed.UTC.Day }}",
					},
				},
				{
					Kind: "file",
					Config: map[string]string{
						"pathformat": "./data/{{ .PublishedParsed.UTC.Year }}/{{ .PublishedParsed.UTC.Month }}/{{ .PublishedParsed.UTC.Day }}",
					},
				},
				{
					Kind: "kafka",
					Config: map[string]string{
						"broker": "127.0.0.1:9092",
						"topic":  "feeds",
					},
				},
				{
					Kind: "discord",
					Config: map[string]string{
						"url": "https://discord.example.com/webhook",
					},
				},
			},
			Redis: RedisConfig{
				Host: "127.0.0.1:6379",
			},
		},
	}
	for file, expected := range cases {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			filename := fmt.Sprintf("testdata/%s", file)
			config := loadConfig(filename)
			if !reflect.DeepEqual(config, expected) {
				t.Errorf("Configs do not match:\n%s\n%s", config, expected)
			}
		})
	}
}
