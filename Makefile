.PHONY: build test docker

DOCKER_IMAGE_NAME ?= alertmanager2es
DOCKER_IMAGE_TAG  ?= $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))

build: test
	go build -ldflags "-X main.revision=$(shell git describe --tags --always --dirty=-dev)" -tags netgo

test:
	go test $(go list ./... | grep -v /vendor/)

docker:
	@docker build -t "$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" .
