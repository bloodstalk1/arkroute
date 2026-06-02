# Arkrouter

Arkrouter is a local AI model router for coding tools. Phase 1 focuses on Claude Code CLI through an Anthropic-compatible local gateway.

## Development

```sh
go test ./...
go run ./cmd/arkrouter help
```

## Build And Install

```sh
make test
make build
make install
```

By default `make install` writes to `~/bin/arkrouter`.

```sh
make install PREFIX=/usr/local
```

## Phase 1 Commands

```sh
arkrouter init
arkrouter validate
arkrouter serve
eval "$(arkrouter activate claude)"
claude
```

## Claude Code Usage

```sh
arkrouter init
arkrouter validate
arkrouter serve
```

In another shell:

```sh
eval "$(arkrouter activate claude)"
claude
```

## Operator Commands

```sh
arkrouter config path
arkrouter config show
arkrouter provider list
arkrouter model list
arkrouter route list
arkrouter status
arkrouter doctor
arkrouter test sonnet "hello"
arkrouter logs --tail 50
arkrouter version --debug
```

## Config

Default config path:

```text
~/.arkrouter/config.yaml
```

Generated provider keys use `env:NAME` references. Export provider keys in your shell before starting `arkrouter serve`.

## Safety

Arkrouter binds to `127.0.0.1` by default and does not log prompt or response bodies.

## Troubleshooting

Some tests use `httptest` and bind a local loopback port. If a restricted sandbox blocks local port binding, run the same `go test -count=1 ./...` command with permission to bind loopback.

Arkrouter redacts provider API keys and the local client key in status/config output. It does not log prompt or response bodies.
