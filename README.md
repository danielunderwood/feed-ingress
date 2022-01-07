# feed-ingress

An ingress for RSS, Atom, JSONFeed, and other news feeds that provides configurable output to multiple sources for storage or processing.

## Running

### Local Go

```shell
$ nix-shell # Or local environment with go + docker-compose
$ $EDITOR config.yaml
$ docker-compose -f docker-compose.dependencies.yml up -d
$ go run *.go
```

### Docker-Compose

```shell
$ docker-compose build
$ docker-compose -f docker-compose.dependencies.yml -f docker-compose.yml up
```

### Docker

```shell
# Set up dependencies (at least redisbloom)
$ $EDITOR config.yaml
$ docker run --name feed-ingress -v ./config.yaml:/app/config.yaml:ro ghcr.io/danielunderwood/feed-ingress
```

## Configuration

### Feeds

Feeds are a simple list of URLs in the config file:

```yaml
feeds:
  - https://example.com/rss.xml
  - https://example-2.com/feed.xml
```

These URLs are parsed by [gofeed's universal parser](https://github.com/mmcdole/gofeed#universal-feed-parser-1) with the hope of handling most things thrown at it.

### Redis

[redisbloom](https://oss.redis.com/redisbloom/) is currently used for deduplication of feed items. At the moment, it only supports the configuration of a host:

```yaml
redis:
  host: redis:6379
```

### Outputs

#### Local Files

`kind: file` will store to local JSON files based on the path format (note that the file name itself currently has a hardcoded format):

```yaml
  - kind: file
    config:
      pathformat: "./data/{{ .PublishedParsed.UTC.Year }}/{{ .PublishedParsed.UTC.Month }}/{{ .PublishedParsed.UTC.Day }}"
```

#### S3-Compatible Storage

The `kind: s3` will work with any S3-compatible storage, such as AWS S3, Backblaze B2, or MinIO:

```yaml
  - kind: s3
    config:
      endpoint: https://s3.some-region.provider.com
      region: some-region
      accesskeyid: access-key
      accesssecret: access-secret
      bucket: my-feed-data
      keyformat: "{{ .PublishedParsed.UTC.Year }}/{{ .PublishedParsed.UTC.Month }}/{{ .PublishedParsed.UTC.Day }}"
```

#### Kafka

`kind: kafka` will output to a Kafka (or compatible service, such as redpanda) topic:

```yaml
  - kind: kafka
    config:
      broker: "127.0.0.1:9092"
      topic: feeds
```

The included `docker-compose.yml` will set up redpanda for testing.