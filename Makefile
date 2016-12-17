.PHONY: build test

build: test
	go build -ldflags "-X main.revision=$(shell git describe --tags --always --dirty=-dev)"

test:
	go test $(go list ./... | grep -v /vendor/)
