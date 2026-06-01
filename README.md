# Arkrouter

Arkrouter is a local AI model router for coding tools. Phase 1 focuses on Claude Code CLI through an Anthropic-compatible local gateway.

## Development

```sh
go test ./...
go run ./cmd/arkrouter help
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

## Config

Default config path:

```text
~/.arkrouter/config.yaml
```

Generated provider keys use `env:NAME` references. Export provider keys in your shell before starting `arkrouter serve`.

## Safety

Arkrouter binds to `127.0.0.1` by default and does not log prompt or response bodies.
