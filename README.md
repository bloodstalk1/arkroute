# Arkroute

Arkroute is a local AI model router for coding tools. Phase 1 focuses on Claude Code CLI through an Anthropic-compatible local gateway.

## Development

```sh
go test ./...
go run ./cmd/arkroute help
```

## Build And Install

```sh
make test
make build
make install
```

By default `make install` writes to `~/bin/arkroute`.

```sh
make install PREFIX=/usr/local
```

## Phase 1 Commands

```sh
arkroute init
arkroute validate
arkroute serve
eval "$(arkroute activate claude)"
claude
```

## Claude Code Usage

```sh
arkroute init
arkroute validate
arkroute serve
```

In another shell:

```sh
eval "$(arkroute activate claude)"
claude
```

## Operator Commands

```sh
arkroute config path
arkroute config show
arkroute provider list
arkroute model list
arkroute route list
arkroute status
arkroute doctor
arkroute test sonnet "hello"
arkroute logs --tail 50
arkroute version --debug
```

## Config

Default config path:

```text
~/.arkroute/config.yaml
```

Generated provider keys use `env:NAME` references. Export provider keys in your shell before starting `arkroute serve`.

## Safety

Arkroute binds to `127.0.0.1` by default and does not log prompt or response bodies.

## Troubleshooting

Some tests use `httptest` and bind a local loopback port. If a restricted sandbox blocks local port binding, run the same `go test -count=1 ./...` command with permission to bind loopback.

Arkroute redacts provider API keys and the local client key in status/config output. It does not log prompt or response bodies.
