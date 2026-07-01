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

# Version stamp injected into the binary via -ldflags. Overridable from the
# environment (e.g. CI) but defaults to git-derived values.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# Linting
GOLINT=golangci-lint

# Install location for a user-local install (no sudo). Overridable, e.g.
# `make install INSTALL_DIR=/usr/local/bin`.
INSTALL_DIR ?= $(HOME)/.local/bin

.PHONY: all build install uninstall clean test coverage lint fmt mod-tidy download-golangci-lint hooks help

all: lint fmt build

build:
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) -v $(MAIN_PATH)

install:
	@mkdir -p $(INSTALL_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(INSTALL_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "installed $(INSTALL_DIR)/$(BINARY_NAME)"
	@case ":$(PATH):" in *":$(INSTALL_DIR):"*) ;; *) echo "note: $(INSTALL_DIR) is not on your PATH" ;; esac

uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "removed $(INSTALL_DIR)/$(BINARY_NAME)"

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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v2.11.3

# Enable the repo's git hooks (.githooks/pre-commit, pre-push). One-time
# per clone. Disable with `git config --unset core.hooksPath`.
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks enabled (core.hooksPath=.githooks)"

# cross-compilation for different platforms
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_UNIX) -v $(MAIN_PATH)

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME).exe -v $(MAIN_PATH)

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)_darwin -v $(MAIN_PATH)

help:
	@echo "Available commands:"
	@echo "  make build           - Build the application"
	@echo "  make install         - Build and install to INSTALL_DIR (default ~/.local/bin)"
	@echo "  make uninstall       - Remove the installed binary from INSTALL_DIR"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make test            - Run tests"
	@echo "  make coverage        - Generate test coverage report"
	@echo "  make lint            - Run linter"
	@echo "  make fmt             - Format code"
	@echo "  make mod-tidy        - Tidy Go modules"
	@echo "  make download-golangci-lint - Download golangci-lint"
	@echo "  make hooks           - Enable git pre-commit/pre-push hooks"
	@echo "  make build-linux     - Build for Linux"
	@echo "  make build-windows   - Build for Windows"
	@echo "  make build-darwin    - Build for macOS"