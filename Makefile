SHELL := /bin/bash

ROOT := $(CURDIR)
DIST_DIR := $(ROOT)/dist
CLI_BIN := $(DIST_DIR)/dotward
APP_BUNDLE := $(DIST_DIR)/Dotward.app
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY ?= make
LDFLAGS := -X github.com/stefanos/dotward/internal/version.Version=$(VERSION) \
	-X github.com/stefanos/dotward/internal/version.Commit=$(COMMIT) \
	-X github.com/stefanos/dotward/internal/version.BuildDate=$(BUILD_DATE) \
	-X github.com/stefanos/dotward/internal/version.BuiltBy=$(BUILT_BY)
GOBIN ?= $(shell go env GOBIN)
ifeq ($(strip $(GOBIN)),)
GOBIN := $(shell go env GOPATH)/bin
endif

.PHONY: build build-cli build-app test install clean fmt

build: build-cli build-app

build-cli:
	@mkdir -p "$(DIST_DIR)"
	go build -trimpath -ldflags "$(LDFLAGS)" -o "$(CLI_BIN)" ./cmd/cli

build-app:
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILD_DATE="$(BUILD_DATE)" BUILT_BY="$(BUILT_BY)" ./scripts/build_app.sh

test:
	go test ./...

install:
	@mkdir -p "$(GOBIN)"
	@mkdir -p "$(DIST_DIR)"
	go build -trimpath -ldflags "$(LDFLAGS)" -o "$(CLI_BIN)" ./cmd/cli
	cp "$(CLI_BIN)" "$(GOBIN)/dotward"
	@echo "Installed dotward to $(GOBIN)/dotward"

fmt:
	gofmt -w $$(rg --files -g '*.go')

clean:
	rm -rf "$(DIST_DIR)"
