BINARY := arkroute
PREFIX ?= $(HOME)
BINDIR ?= $(PREFIX)/bin
VERSION ?= 0.0.1
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/bloodstalk1/arkroute/internal/buildinfo.Version=$(VERSION) -X github.com/bloodstalk1/arkroute/internal/buildinfo.Commit=$(COMMIT) -X github.com/bloodstalk1/arkroute/internal/buildinfo.BuildDate=$(BUILD_DATE)

.PHONY: test build install clean build-npm build-frontend

build-frontend:
	cd web-ui && npm run build
	rm -f internal/panel/assets/*
	cp -r web-ui/dist/* internal/panel/assets/
	mv internal/panel/assets/index.html internal/panel/assets/panel.html

test:
	go test -count=1 ./...

build:
	mkdir -p dist
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) ./cmd/arkroute

install: build
	mkdir -p $(BINDIR)
	install -m 755 dist/$(BINARY) $(BINDIR)/.$(BINARY).tmp
	mv -f $(BINDIR)/.$(BINARY).tmp $(BINDIR)/$(BINARY)

clean:
	rm -rf dist
	rm -rf web-ui/dist


build-npm: build-frontend
	mkdir -p npm/platform/darwin-arm64/bin npm/platform/darwin-x64/bin npm/platform/linux-arm64/bin npm/platform/linux-x64/bin npm/platform/win32-x64/bin
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o npm/platform/darwin-arm64/bin/$(BINARY) ./cmd/arkroute
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o npm/platform/darwin-x64/bin/$(BINARY) ./cmd/arkroute
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o npm/platform/linux-arm64/bin/$(BINARY) ./cmd/arkroute
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o npm/platform/linux-x64/bin/$(BINARY) ./cmd/arkroute
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o npm/platform/win32-x64/bin/$(BINARY).exe ./cmd/arkroute
