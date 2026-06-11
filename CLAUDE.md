# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```sh
make test          # go test -count=1 ./...
make build         # build to dist/arkroute
make install       # build + install to ~/bin/arkroute

# Single package test
go test ./internal/router/...

# Single test
go test -run TestValidateAccepts ./internal/config/...
```

No linter config — standard `go vet` applies. Only external dependency is `gopkg.in/yaml.v3`.

## Architecture

Arkroute is a **local AI model router** acting as an Anthropic-compatible HTTP gateway. Claude Code CLI connects to it thinking it's talking to the Anthropic API; arkroute routes requests to upstream providers (OpenAI-compatible, Gemini, Anthropic).

### Data flow (request lifecycle)

```
Claude Code CLI → POST /v1/messages (Anthropic wire format)
  → client/claude.Server (auth via Bearer token, decode Anthropic JSON)
  → normalize to protocol.Request
  → runtime.Generation.Execute / Stream
    → router.Router.Resolve(alias, requirements) → []Target
    → router.Policy.Select (priority=first only, fallback=ordered list)
    → for each target:
      → adapter.BuildRequest (normalized → provider-specific HTTP request)
      → http.Client.Do (upstream call)
      → adapter.MapResponse (provider-specific → normalized protocol.Response)
      → on failure: retry next target if fallback strategy AND error is retryable
  → map normalized response back to Anthropic wire format
  → HTTP response
```

Streaming follows the same path but uses SSE (Server-Sent Events) with `StreamMapper` per adapter to translate per-provider stream chunks into `protocol.StreamEvent`.

### Package responsibilities

| Package | Role |
|---------|------|
| `cmd/arkroute` | Entry point, calls `cli.Run()` |
| `internal/cli` | Flag parsing, command dispatch to `app` layer |
| `internal/app` | Command implementations: serve, init, activate, doctor, status, reload, etc. |
| `internal/client/claude` | HTTP server exposing Anthropic-compatible API (`/v1/messages`, `/v1/models`, `/healthz`, `/internal/*` admin endpoints) |
| `internal/runtime` | Core: `State` holds the current `Generation` (config snapshot + router + executor). `Executor` handles upstream HTTP calls with retry/fallback. `State.Reload()` handles SIGHUP and admin reload. |
| `internal/router` | Resolves model/route aliases to provider targets. Supports `priority` (first healthy) and `fallback` (ordered retry) strategies. `HealthStore` tracks per-upstream health. |
| `internal/adapter` | `ProviderAdapter` interface: `BuildRequest`, `MapResponse`, `NewStreamMapper`, `ClassifyError`. Implementations in `adapter/openai`, `adapter/gemini`, `adapter/anthropic`. `adapter/builtin` wires the registry. |
| `internal/protocol` | Normalized types: `Request`, `Response`, `Message`, `ContentBlock`, `StreamEvent`. Sub-packages `protocol/anthropic` and `protocol/openai` hold provider-specific wire types. |
| `internal/config` | YAML load, validate, migrate (versioned), snapshot build. `Snapshot` is the immutable, validated, indexed form used at runtime. Keys can reference `env:NAME` for env-var resolution. |
| `internal/failure` | Typed `ErrorClass` constants with `Retryable()` method. Used by executor to decide fallback behavior. |
| `internal/security` | `crypto/rand` key generation, string/map redaction. |
| `internal/observability` | JSONL trace sink. Events track request lifecycle, config reloads. Never logs prompt/response bodies. |
| `internal/buildinfo` | Version, commit, build date (injected via ldflags). |

### Config lifecycle

1. `config.LoadFile(path)` — read YAML, migrate to current version, apply defaults
2. `config.BuildSnapshot(cfg)` — validate, index providers/models/routes into maps, resolve `env:NAME` refs
3. `runtime.NewState(deps)` — build first `Generation` from snapshot + router + executor
4. SIGHUP or `POST /internal/reload` triggers `State.Reload()` — acquires mutex, re-loads file, validates, swaps `current` atomically. Rejects changes to `server.host` or `server.port` (requires restart).

### Route resolution

`router.Router.Resolve(alias, requirements)`:
1. Look up alias in `RoutesByAlias` and `RoutesByDiscoveryAlias`
2. If found: filter targets by capability match, respect `priority` strategy (return first match only)
3. If not found: direct model lookup via `ModelsByExposedAlias`
4. Returns `[]Target` (provider + model configs)

`claude_discovery_alias` allows multiple exposed names to map to the same route — Claude Code queries `/v1/models` and gets these aliases back, then uses them in requests. Validation requires this alias to start with `claude` or `anthropic` (`config/validate.go:134`).

### Streaming support per adapter

All adapters support streaming: `openai_compatible` maps OpenAI SSE deltas, `anthropic` forwards Anthropic native streaming events (message_start, content_block_start/delta/stop, message_delta/message_stop), and `gemini` diffs accumulated full-state Gemini SSE chunks into text deltas with usage metadata.

### Adapter security differences

- **OpenAI-compatible**: API key via `Authorization: Bearer` header
- **Anthropic**: API key via `x-api-key` header
- **Gemini**: API key passed via `x-goog-api-key` header (was previously leaked in URL query — fixed)

### Snapshot immutability

`Generation.Snapshot()` (`runtime/state.go:155`) performs a deep clone of all config maps, slices, and nested structs on every call via ~15 clone helpers. This guarantees callers cannot mutate the router's internal state. When modifying config types (`ProviderConfig`, `RouteConfig`, etc.), ensure the corresponding clone helper is updated.

### Test conventions

- Standard library `testing` only, no assertion libraries
- Table-driven tests preferred: `[]struct{name, input, want, wantErr}`
- Use `config.MinimalValidConfig("test-key")` for test setup
- `config.BuildSnapshot(cfg)` when router tests need a snapshot
- Test file naming: `<pkg>_test.go`, package `<pkg>_test` (external) or `<pkg>` (internal)
- Two custom subagents exist: `test-writer` (generates tests following these patterns) and `security-reviewer` (audits key handling, injection vectors, TLS, adapter security)
