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
