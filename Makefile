BINARY := arkrouter
PREFIX ?= $(HOME)
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X bat.dev/arkrouter/internal/buildinfo.Version=$(VERSION) -X bat.dev/arkrouter/internal/buildinfo.Commit=$(COMMIT) -X bat.dev/arkrouter/internal/buildinfo.BuildDate=$(BUILD_DATE)

.PHONY: test build install clean

test:
	go test -count=1 ./...

build:
	mkdir -p dist
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) ./cmd/arkrouter

install: build
	mkdir -p $(BINDIR)
	cp dist/$(BINARY) $(BINDIR)/$(BINARY)

clean:
	rm -rf dist
