.PHONY: build build-linux build-darwin clean test lint install

BINARY := casaos-webhook-emitter
VERSION := 0.1.0
GOFLAGS := -ldflags "-X main.Version=$(VERSION)"
PKG := github.com/ChonSong/casaos-webhook-emitter

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/emitter

build-linux:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o bin/$(BINARY)-linux-amd64 ./cmd/emitter
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -o bin/$(BINARY)-linux-arm64 ./cmd/emitter

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) -o bin/$(BINARY)-darwin-amd64 ./cmd/emitter
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -o bin/$(BINARY)-darwin-arm64 ./cmd/emitter

clean:
	rm -rf bin/

test:
	go test ./...

lint:
	golangci-lint run || golint ./...

install: build
	install -Dm755 bin/$(BINARY) ~/.local/bin/$(BINARY)

all: build
