# Makefile for rotki-sync Go project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
BINARY_NAME=rotki-sync
BINARY_UNIX=$(BINARY_NAME)_unix
MAIN_PATH=./cmd/sync

# Linting
GOLINT=golangci-lint

.PHONY: all build clean test coverage lint fmt mod-tidy download-golangci-lint help

all: lint fmt build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

test:
	$(GOTEST) -v ./...

coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

lint:
	$(GOLINT) run ./...

fmt:
	$(GOFMT) -w -s .

mod-tidy:
	$(GOMOD) tidy

download-golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v2.1.6

# cross-compilation for different platforms
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v $(MAIN_PATH)

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME).exe -v $(MAIN_PATH)

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)_darwin -v $(MAIN_PATH)

help:
	@echo "Available commands:"
	@echo "  make build           - Build the application"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make test            - Run tests"
	@echo "  make coverage        - Generate test coverage report"
	@echo "  make lint            - Run linter"
	@echo "  make fmt             - Format code"
	@echo "  make mod-tidy        - Tidy Go modules"
	@echo "  make download-golangci-lint - Download golangci-lint"
	@echo "  make build-linux     - Build for Linux"
	@echo "  make build-windows   - Build for Windows"
	@echo "  make build-darwin    - Build for macOS"