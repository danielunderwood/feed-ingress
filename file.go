package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/template"

	"github.com/mmcdole/gofeed"
)

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
		log.Println("ERROR: Could not create ", path, err)
		return err
	}

	// Prefix for data files to identify their source. We should probably be careful to make sure this
	// is something that's reasonable to create a file with, but whatever
	prefix := feed.Title
	file := fmt.Sprintf("%s/%s-%s.json", path, prefix, identifier)
	log.Println("Saving to ", file)
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
		log.Println("ERROR: Could not marshal", item)
		return err
	}
	err = os.WriteFile(file, data, 0o0640)
	if err != nil {
		log.Println("ERROR: Failed to write", string(data))
		return err
	}
	return nil
}
