VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build
build:
	go build -ldflags "-X main.version=$(VERSION)" .

.PHONY: install
install:
	go install -ldflags "-X main.version=$(VERSION)" .
