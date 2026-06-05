# Arkrouter Phase 2 Hardening Design

Date: 2026-06-02
Status: Ready for user review

## Goal

Phase 2 turns Arkrouter from a working Claude Code local gateway into a durable runtime foundation for future multi-tool routing.

The priority is not adding OpenCode, Cursor, dashboard, token compression, or cloud features yet. The priority is extracting reusable runtime boundaries from Phase 1 so later phases can add those features without copying Claude-specific request execution logic.

## Why This Phase Exists

Phase 1 delivered the first working vertical slice:

- Claude Code-compatible `/v1/messages`, `/v1/messages/count_tokens`, and `/v1/models`.
- OpenAI-compatible, Gemini, and Anthropic provider adapters.
- Basic fallback, health, JSONL trace primitives, CLI commands, and config validation.

The current implementation is correct for Phase 1, but a self-review found these structural risks:

- `client/claude/messages.go` owns too much: Claude decode, route resolution, adapter selection, upstream HTTP calls, fallback, health, and response mapping.
- Streaming is partially hardcoded to the OpenAI stream mapper in the Claude handler.
- Router output is `[]Target`, not an explicit `RoutePlan`, so future policy work would likely leak into handlers.
- Trace types exist, but request execution does not yet emit structured lifecycle events consistently.
- `status` and `doctor` mostly read config and do not reflect live runtime state.
- There is no read-only admin API for a future dashboard or richer CLI.
- There is no config migration framework or release metadata foundation.

Phase 2 fixes these boundaries while keeping product scope narrow.

## Non-Goals

Phase 2 does not include:

- OpenAI-compatible ingress for OpenCode, Cursor, Cline, Continue, or other tools.
- OpenAI Responses API ingress.
- Web dashboard.
- Cloud sync.
- Team mode.
- OAuth or subscription account pooling.
- Token compression.
- Cost-aware, quota-aware, weighted, or latency-aware routing policies beyond interface preparation.
- SQLite migration from JSONL logs.

## Target Architecture

Phase 2 introduces a runtime execution layer between client adapters and provider adapters:

```text
client/claude
  - Anthropic HTTP decode
  - Anthropic HTTP/SSE encode
  - local auth
  - no provider execution logic

runtime
  - normalized request execution
  - route plan execution
  - adapter dispatch
  - upstream HTTP calls
  - fallback
  - health updates
  - trace events

router
  - alias resolution
  - capability filtering
  - RoutePlan creation
  - policy selection

policy
  - priority
  - fallback
  - future strategies plug in here

adapter/*
  - provider protocol mapping only
  - stream mapper creation
  - upstream error classification

admin
  - read-only local status/config/routes/health APIs
  - reused by CLI now and dashboard later
```

The client package should only know how to translate client protocol to and from normalized protocol. It should not know how to call OpenRouter, Gemini, or Anthropic upstreams.

Compatibility rule: Phase 2 is an internal hardening phase. Existing Phase 1 user-facing behavior must remain compatible unless explicitly listed in this spec. Claude Code endpoints, config format, local auth behavior, and existing CLI commands must continue to work while the internals move behind runtime boundaries.

Refactor rule: introduce new boundaries behind existing tests first, then move behavior. Do not delete working Phase 1 paths until equivalent runtime tests and HTTP integration tests pass.

## Runtime Executor

Add a new package:

```text
internal/runtime/
```

Primary responsibilities:

- Accept a `protocol.Request`.
- Ask router for a `RoutePlan`.
- Select provider adapters through a registry.
- Execute upstream HTTP requests.
- Apply fallback rules.
- Update health.
- Emit trace events.
- Return a normalized non-streaming response or a normalized stream.

The runtime package owns execution; client packages own protocol surface.

Proposed public types:

```go
type Executor struct {
    Snapshot config.Snapshot
    Router   *router.Router
    Adapters adapter.Registry
    Health   *router.HealthStore
    Trace    observability.TraceSink
    Client   *http.Client
}

type ExecuteRequest struct {
    RequestID    string
    Client       string
    Model        string
    Requirements router.Requirements
    Request      protocol.Request
}

type ExecuteResult struct {
    Response protocol.Response
    Target   router.Target
    Attempts []Attempt
}

type Attempt struct {
    Target       router.Target
    StatusCode   int
    Latency      time.Duration
    Retryable    bool
    ErrorMessage string
}
```

Streaming needs an explicit path:

```go
type StreamResult struct {
    Target   router.Target
    Attempts []Attempt
    Events   <-chan protocol.StreamEvent
    Close    func() error
}
```

The exact Go API can evolve during implementation, but the boundary must remain: HTTP client handlers do not execute provider-specific upstream calls directly.

Runtime execution requirements:

- Use request contexts for all upstream work.
- Reuse a configured `http.Client` instead of constructing one per request.
- Close upstream response bodies exactly once.
- Never write client HTTP responses from runtime. Runtime returns normalized data; client adapters encode it.
- Preserve before-first-byte fallback for streaming. After a stream event has been emitted to the client adapter, runtime must not switch targets.
- Avoid goroutine leaks in streaming. Event channels must close on upstream EOF, mapper error, context cancellation, or client close.
- If trace writes fail, request execution should continue. Trace failures should be observable through status or logs, not returned as model errors.
- Runtime can still use a single immutable snapshot in Phase 2. Hot reload is not part of this phase, but runtime construction should not make hot reload harder later.

Server lifecycle requirements:

- Replace direct `http.ListenAndServe` usage with an `http.Server`.
- Support graceful shutdown on `SIGINT` and `SIGTERM`.
- Shutdown should use a bounded timeout.
- Startup errors and shutdown errors should be distinguishable in CLI output.

## Router Plan And Policy

Replace handler-facing `Resolve(alias, requirements) []Target` usage with a route plan:

```go
type RoutePlan struct {
    Alias        string
    Strategy     string
    Requirements Requirements
    Targets      []Target
}
```

Router responsibilities:

- Resolve route alias, Claude discovery alias, or exposed model alias.
- Filter by capabilities.
- Preserve route strategy.
- Return a `RoutePlan`.

Policy responsibilities:

- Choose target ordering from a plan and runtime health.
- Keep policy-specific logic out of HTTP handlers.

Phase 2 implements only:

- `priority`
- `fallback`

But it should introduce interfaces that later support:

- `weighted`
- `cost_aware`
- `quota_aware`
- `latency_aware`

No future strategy should require changing Claude handlers.

Policy interface requirements:

- Policies receive a `RoutePlan` and a read-only health snapshot.
- Policies return an ordered target list plus a reason string for traceability.
- Policies do not perform upstream calls.
- Policies do not mutate health.
- Unknown route strategies fail at config validation time, not during request execution.

## Adapter Registry And Streaming

Phase 1 selects adapters directly in `client/claude/messages.go`. Phase 2 moves this into an adapter registry:

```go
type Registry interface {
    Get(providerType string) (ProviderAdapter, bool)
}
```

Provider adapters should support:

```go
BuildRequest(...)
MapResponse(...)
NewStreamMapper() (StreamMapper, bool)
ClassifyError(status int, body []byte) ErrorClass
```

`NewStreamMapper` is critical. Claude streaming should not hardcode the OpenAI stream mapper. If a provider does not support stream mapping, runtime should return a structured unsupported-capability error before streaming starts. Do not advertise streaming support with a lossy placeholder mapper; return `false` until a provider-specific parser maps content deltas correctly.

Stream mapper contract:

- A stream mapper consumes provider wire stream frames and emits normalized stream events.
- It is per request and may keep state.
- It must not write directly to HTTP responses.
- It must return sanitized errors.
- It should tolerate empty heartbeat lines.
- Provider-specific stream parser details stay inside provider adapter packages.

Provider adapters still must not:

- Read config files.
- Update health.
- Write trace logs.
- Decide fallback.
- Know about Claude Code.

## Trace And Observability Model

Phase 2 turns trace logging from a passive helper into an execution event model.

Trace events should include a schema version from the start:

```json
{"schema_version":1,"event":"request_started"}
```

Trace event types:

- `request_started`
- `route_planned`
- `target_selected`
- `upstream_request_started`
- `upstream_response`
- `fallback`
- `stream_started`
- `request_finished`
- `request_failed`

Trace fields:

- schema version
- event type
- timestamp
- request ID
- client name
- route alias
- strategy
- provider ID
- provider type
- model ID
- upstream model
- HTTP status
- latency in milliseconds
- retryable flag
- fallback reason
- error class

Trace events must not include:

- prompt body
- response body
- API keys
- authorization headers
- resolved secret values

Trace sink interface:

```go
type TraceSink interface {
    Emit(event TraceEvent)
    Stats() Stats
}
```

Initial sinks:

- no-op sink for tests and quiet mode
- JSONL sink for local logs

The JSONL schema should be stable enough for future dashboard, cost tracking, and analytics.

Trace sink rules:

- `Emit` must be safe to call from concurrent requests.
- JSONL writes must be serialized.
- Trace sink failures must not expose secrets.
- Trace body fields are forbidden even behind debug flags in Phase 2.
- Trace sinks should expose lightweight counters for emitted events, failed writes, and dropped events.

## Health Model

Health should remain runtime state, separate from immutable config snapshot.

Phase 2 health state should include:

- status: `unknown`, `ok`, `degraded`, `unhealthy`
- last status code
- last error class
- last error message, sanitized
- last latency
- last updated timestamp

Health writes happen in runtime executor. Client packages and adapters do not mutate health directly.

`GET /healthz` remains public and simple. It should continue to show overall process health. Richer state moves to internal admin endpoints.

Health read/write rules:

- Reads return copies, not shared mutable maps.
- Error messages stored in health are sanitized and length-limited.
- Health updates include the provider ID and upstream model where possible.
- Health state must not be stored inside the immutable config snapshot.

## Admin API

Phase 2 adds read-only local admin endpoints:

- `GET /internal/status`
- `GET /internal/config`
- `GET /internal/routes`
- `GET /internal/health`

Rules:

- Require the same local client auth as Claude endpoints.
- Bind only to the same local server.
- Return redacted config.
- Never expose API keys or authorization headers.
- Redact the local client key as well as upstream provider keys.
- Return JSON shaped for CLI and future dashboard reuse.
- Do not add CORS headers in Phase 2.
- Do not add mutating admin endpoints in Phase 2.
- Return stable response envelopes with `schema_version: 1`.

Endpoint responsibilities:

`/internal/status`:

- server running
- version metadata
- config loaded at
- route/model/provider counts
- health summary

`/internal/config`:

- redacted effective config
- config path if available

`/internal/routes`:

- route aliases
- discovery aliases
- strategies
- target model IDs
- target provider IDs
- capabilities relevant to routing

`/internal/health`:

- full sanitized health state by provider ID

CLI commands should call these endpoints when the server is running and fall back to config-only output when the server is unreachable.

Admin endpoint failures should be explicit. CLI output should distinguish:

- server unreachable
- invalid local auth
- config read failure
- admin endpoint returned malformed JSON

## CLI Operator UX

Phase 2 improves local operations without introducing UI.

Commands:

```sh
arkrouter config path
arkrouter config show
arkrouter provider list
arkrouter model list
arkrouter route list
arkrouter logs --tail 50
arkrouter status
arkrouter doctor
arkrouter test sonnet "hello"
arkrouter version --debug
```

Expected behavior:

- `config path`: prints the effective config path.
- `config show`: prints redacted effective config.
- `provider list`: table with provider ID, type, enabled, base URL.
- `model list`: table with model ID, provider, upstream model, exposed alias, key capabilities.
- `route list`: table with alias, strategy, targets.
- `logs --tail N`: prints last N JSONL trace events.
- `status`: uses `/internal/status` if server is reachable; otherwise prints config summary plus `server: unreachable`.
- `doctor`: checks config validity, referenced environment variables, port availability, server reachability, and route target sanity.
- `test`: reports clear categories: gateway unreachable, route missing, auth failure, upstream auth failure, timeout, retry exhausted.
- `version --debug`: prints version, commit, build date, Go version, OS/arch.

All output must redact secrets by default.

Output format rule: human-readable tables are the default for CLI commands in Phase 2. JSON output can be added later, but the admin API must already return JSON so dashboard and automation can build on it.

## Config Store And Migration Foundation

Phase 2 does not need to change the config schema substantially, but it should add a boundary for future migrations.

Add a config store abstraction:

```go
type Store interface {
    Path() string
    Load() (config.Config, error)
    Save(config.Config) error
}
```

Add a migration package or file:

```text
internal/config/migrate.go
```

Phase 2 behavior:

- `version: 1` remains the only supported schema.
- `Migrate(cfg)` returns the same config for version 1.
- Unknown future versions fail with a clear error.

This prevents future config evolution from being bolted into CLI commands.

Migration rules:

- Loading config runs migration validation before snapshot creation.
- Version 1 input produces version 1 output.
- Unknown versions return a typed error.
- Migration code must not resolve provider secrets or mutate environment variables.

## Release Foundation

Phase 2 adds basic release hygiene:

- `Makefile`
- build metadata injected via `-ldflags`
- `arkrouter version --debug`
- install target for local binary
- README troubleshooting section

Make targets:

```sh
make test
make build
make install
make clean
```

Version metadata:

- version
- commit
- build date
- Go version
- OS/arch

The project does not need full GoReleaser in Phase 2 unless it remains simple. A Makefile-based release foundation is enough for now.

Release foundation must not require network access beyond the normal Go module cache. It should work for local development with:

```sh
make test
make build
```

Install behavior:

- `make build` writes `dist/arkrouter`.
- `make install` installs to `$(HOME)/bin/arkrouter` by default.
- `make install PREFIX=/usr/local` installs to `/usr/local/bin/arkrouter`.
- The install target must create the destination directory if needed.
- The install target must not require sudo unless the chosen prefix requires it.

## Error Handling

Phase 2 should classify errors consistently:

- `invalid_request`
- `route_not_found`
- `unsupported_capability`
- `gateway_auth`
- `upstream_auth`
- `upstream_rate_limit`
- `upstream_timeout`
- `upstream_retryable`
- `upstream_fatal`
- `stream_error`

The runtime executor should use this classification for:

- fallback decision
- health update
- trace event
- CLI test output
- Anthropic-compatible error response

Avoid matching raw error strings in CLI code. Use structured error categories.

Implementation boundary: error class constants should live in a small neutral package, such as `internal/failure`, instead of `internal/runtime`. Provider adapters and runtime both need these constants; putting them in runtime would create an import cycle because runtime also depends on adapters.

Error classification rules:

- `401` and `403` from upstream are `upstream_auth` and do not fallback.
- `429` is `upstream_rate_limit` and may fallback.
- `408`, `500`, `502`, `503`, and `504` are retryable.
- Context deadline exceeded is `upstream_timeout` and may fallback before streaming starts.
- Unsupported capability is a routing error and does not call upstream.
- Provider response bodies may be used for classification but must be sanitized before logs, health, or CLI output.

## Testing Strategy

Required test areas:

- Runtime executor non-streaming success path.
- Runtime fallback on retryable status.
- Runtime no fallback on auth/config errors.
- Runtime trace events for success and fallback.
- Runtime health updates.
- RoutePlan creation and policy target ordering.
- Adapter registry selection and unsupported provider handling.
- Streaming path uses provider `NewStreamMapper`, not an OpenAI hardcoded mapper in Claude handler.
- Admin endpoints require auth and return redacted data.
- CLI list/show commands redact secrets.
- `status` works when server is reachable and when unreachable.
- `doctor` detects missing env var references.
- `logs --tail N` reads only the requested number of JSONL events.
- `version --debug` includes build metadata.
- Phase 1 HTTP behavior remains compatible after runtime extraction.
- Streaming cancellation closes event channels and upstream bodies.
- Admin API responses include `schema_version`.
- `go test -count=1 ./...` passes.

Tests that bind local ports may require running outside restricted sandboxes. This should be documented in troubleshooting.

## Acceptance Criteria

Phase 2 is complete when:

- Existing Phase 1 tests still pass without weakening assertions.
- `client/claude` no longer directly selects provider adapters for non-streaming or streaming execution.
- `client/claude` no longer directly performs upstream HTTP calls.
- Runtime executor owns upstream execution, fallback, health updates, and trace emission.
- Router exposes a route plan instead of only a raw target list for handler use.
- Adapter registry supports provider lookup and stream mapper creation.
- Streaming no longer hardcodes OpenAI mapper in Claude handler.
- Trace JSONL receives real request lifecycle events.
- `/internal/status`, `/internal/config`, `/internal/routes`, and `/internal/health` exist and require auth.
- Admin API responses include `schema_version: 1` and never expose provider secrets.
- CLI config/provider/model/route list commands work and redact secrets.
- `arkrouter status` uses live admin status when available and degrades gracefully when unreachable.
- `arkrouter doctor` checks config, env references, port, and server reachability.
- `arkrouter logs --tail N` works.
- `arkrouter version --debug` works.
- `make test`, `make build`, and `make install` exist.
- `go test -count=1 ./...` passes.

## Future Phase Impact

Phase 2 prepares these later phases:

- Phase 3 OpenAI-compatible ingress can call `runtime.Executor` directly instead of copying Claude handler logic.
- Phase 4 dashboard can read `/internal/*` endpoints instead of duplicating CLI status logic.
- Phase 5 cost/quota/latency-aware routing can add policies without rewriting client handlers.
- Phase 6 token compression can hook into normalized protocol before runtime execution.
- Future config schema changes can use migration boundaries instead of ad hoc command logic.

## Approved Decisions

- Phase 2 scope: hardening and architecture foundation.
- No OpenAI ingress in Phase 2.
- No dashboard in Phase 2.
- Runtime executor is the central architectural change.
- Admin API is read-only and local-auth protected.
- Release foundation is Makefile and build metadata first, not a full release platform.
