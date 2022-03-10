FROM golang:1.17 AS build

WORKDIR /go/src/github.com/danielunderwood/feed-parser
COPY *.go ./
COPY go.* ./
RUN go build -v ./...

FROM debian:11-slim
WORKDIR /app
COPY --from=build /go/src/github.com/danielunderwood/feed-parser/feed-parser /app
# Install ca-certificates so TLS can be verified
RUN apt-get update && apt-get install -y ca-certificates && apt-get clean && rm -rf /var/lib/apt/lists/*
CMD ["/app/feed-ingress"]