package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

// Yep, all copied from https://github.com/danielunderwood/log2http/blob/main/discord.go
type Author struct {
	Name string `json:"name"`
}

type Provider struct {
	Name string `json:"name"`
}

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// https://discord.com/developers/docs/resources/channel#embed-object
type Embed struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Author      Author   `json:"author,omitempty"`
	Provider    Provider `json:"provider,omitempty"`
	Fields      []Field  `json:"fields,omitempty"`
	Url         string   `json:"url,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
}

type DiscordMessage struct {
	Content string  `json:"content,omitempty"`
	Embeds  []Embed `json:"embeds,omitempty"`
}

type DiscordWebhook struct {
	Url          string
	MessageQueue chan DiscordMessage
}

type RateLimitResponse struct {
	Global     bool   `json:"global"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after"`
}

func NewDiscordWebhook(url string) *DiscordWebhook {
	// TODO This could be a weird edge case, though it hopefully isn't too likely
	c := make(chan DiscordMessage, 1000)
	d := DiscordWebhook{Url: url, MessageQueue: c}
	go d.ProcessMessages()
	return &d
}

func (d *DiscordWebhook) ProcessMessages() {
	for m := range d.MessageQueue {
		// Retry for any rate limits
		// This could also be approached with re-queueing messages, but the naive
		// solution to that (just doing c <- message here) results in a possible deadlock
		for {
			delay := d.sendMessage(m)

			if delay == 0 {
				break
			}

			fmt.Printf("Rate Limited: Delaying for %f seconds\n", delay.Seconds())
			time.Sleep(delay)
		}
	}
}

func (d *DiscordWebhook) Close() {
	close(d.MessageQueue)
}

// The design of this doesn't make a ton of sense, but it returns the duration
// that we need to wait until the next request. If 0, we can proceed. If we
// get rate-limited, this will be non-zero and tell us how long to wait (along
// with implicitly telling us that the sending failed).
func (d *DiscordWebhook) sendMessage(message DiscordMessage) time.Duration {
	body, err := json.Marshal(message)
	if err != nil {
		fmt.Print("Failed to marshal JSON", err)
		return 0
	}

	resp, err := http.Post(
		d.Url,
		"application/json",
		bytes.NewReader(body),
	)

	if err != nil {
		fmt.Println("ERROR", err)
		return 0
	}

	body, _ = ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 204 {
		fmt.Println("Request failed", resp.StatusCode, string(body))
	}

	if resp.StatusCode == 429 {
		var content RateLimitResponse
		err := json.Unmarshal(body, &content)
		if err != nil {
			fmt.Printf("ERROR: Could not parse delay time: %s. Defaulting to 10 seconds\n", err)
			return 10 * time.Second
		}
		// The API docs say that this field is in seconds, but values that I have
		// been getting are >1000, so I'm going to assume milliseconds. time.Duration
		// takes nanoseconds, so we convert here.
		return time.Duration(content.RetryAfter * 1e6)
	}

	return 0
}

// Outputs a feed to a discord webhook
// This is probably best used with lower-frequency feeds
type DiscordWebhookOutput struct {
	Client *DiscordWebhook
}

func NewDiscordWebhookOutput(url string) *DiscordWebhookOutput {
	return &DiscordWebhookOutput{Client: NewDiscordWebhook(url)}
}

func (out *DiscordWebhookOutput) Close() {
	out.Client.Close()
}

func (out *DiscordWebhookOutput) Write(feed *gofeed.Feed, item gofeed.Item, identifier string) error {
	var authorStr string
	for i, author := range item.Authors {
		authorStr += author.Name
		if i != len(item.Authors)-1 {
			authorStr += ", "
		}
	}
	out.Client.MessageQueue <- DiscordMessage{
		Embeds: []Embed{
			{
				// TODO It would be nice to allow all of this as configuration
				// But we need to introduce a struct that takes in both the feed and the item, which would
				// break compatibility with the way the s3 template currently works -- save for next breaking release
				Title:       fmt.Sprintf("[%s] %s", feed.Title, item.Title),
				Description: item.Description,
				Author:      Author{Name: authorStr},
				Url:         item.Link,
				Timestamp:   item.Published,
			},
		},
	}
	return nil
}
