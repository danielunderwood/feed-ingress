package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/mmcdole/gofeed"
)

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
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		return err
	}
	s3Client := s3.New(newSession)

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

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
