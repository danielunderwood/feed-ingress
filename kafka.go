package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/mmcdole/gofeed"
	"github.com/segmentio/kafka-go"
)

type KafkaOutput struct {
	Broker string
	Topic  string
}

func (out KafkaOutput) Write(feed *gofeed.Feed, item gofeed.Item, identifier string) error {
	w := &kafka.Writer{
		Addr:     kafka.TCP(out.Broker),
		Topic:    out.Topic,
		Balancer: &kafka.LeastBytes{},
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Fatal("Failed to marshal", item, err)
		return err
	}

	err = w.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte("Item"),
			Value: data,
		},
	)
	if err != nil {
		log.Fatal("failed to write messages:", err)
		return err
	}

	if err := w.Close(); err != nil {
		log.Fatal("failed to close writer:", err)
		return err
	}
	return nil
}
