# Arkroute Phase 3 Control Plane Design

Date: 2026-06-02
Status: Ready for user review

## Goal

Phase 3 makes Arkroute safer to run as a long-lived local gateway for Claude Code CLI by adding a runtime control plane with hot config reload.

After Phase 3, users should be able to edit provider, model, route, timeout, and alias settings, then reload Arkroute without restarting the process. New requests use the new config. In-flight requests keep using the config generation they started with.

The long-term product remains multi-tool routing, but Phase 3 intentionally strengthens the runtime boundary before adding OpenCode, Cursor, Cline, Codex, dashboard, or advanced routing policies.

## Why This Phase Exists

Phase 2 moved request execution behind a reusable runtime executor. That made the Claude Code client adapter much cleaner, but the server still holds a fixed config snapshot at startup:

- `client/claude.Server` stores one `config.Snapshot`.
- `runtime.Executor` stores one `config.Snapshot`, router, and upstream HTTP client timeout.
- Admin status/config/routes endpoints read that fixed snapshot.
- CLI `status` can read live admin state, but there is no way to mutate runtime state.

If Arkroute adds more client ingress before runtime reload exists, every new client will need to be revisited later. Phase 3 prevents that by making config access generation-aware and centralized now.

## Non-Goals

Phase 3 does not include:

- OpenAI-compatible ingress for OpenCode, Cursor, Cline, Continue, or Codex.
- OpenAI Responses API ingress.
- Web dashboard.
- Mutating provider/model/route editing endpoints beyond the single reload endpoint.
- File watching with automatic reload.
- SQLite or durable metrics storage.
- Health-aware, latency-aware, cost-aware, quota-aware, or weighted routing.
- Secrets manager integration.
- Multi-user admin, remote admin, CORS, OAuth, or cloud sync.
- Changing the public config schema version from `version: 1`.

## Target Architecture

Phase 3 introduces a small runtime control layer between HTTP handlers and the executor:

```text
serve
  -> load initial config snapshot
  -> create RuntimeState
  -> create Claude server with RuntimeState

client/claude handlers
  -> read current generation from RuntimeState
  -> execute request through generation executor
  -> encode Anthropic-compatible response

admin reload / SIGHUP / CLI reload
  -> load config file
  -> migrate/default/validate/build snapshot
  -> create new generation executor
  -> atomically publish generation
```

The control layer is responsible for current config generation and reload history. The executor remains responsible for request execution. Claude handlers remain responsible for Claude protocol decode/encode.

## Runtime State

Add a new runtime state object in `internal/runtime`:

```go
type State struct {
    // exact internals chosen during implementation
}
```

Responsibilities:

- Hold the current immutable runtime generation.
- Return a generation handle for request execution.
- Atomically replace the current generation on successful reload.
- Keep reload metadata separate from config snapshots.
- Keep health and trace sinks shared across generations.
- Expose read-only status for admin endpoints.

The current generation should contain:

- generation number
- loaded timestamp
- config snapshot
- router created from that snapshot
- executor created from that snapshot
- upstream HTTP client using that snapshot's timeout

Reload metadata should contain:

- config path
- current generation number
- loaded timestamp for current generation
- last reload attempt timestamp
- last successful reload timestamp
- last failed reload timestamp
- last reload error class/message
- reload count
- failed reload count

The state object must use a concurrency-safe mechanism such as `atomic.Value`, `atomic.Pointer`, or a short-held `sync.RWMutex`. Request execution must not hold a write lock while performing upstream calls.

## Reload Semantics

Reload is an all-or-nothing operation:

1. Emit `config_reload_started`.
2. Read the config file from the server's configured path.
3. Run migration and defaults.
4. Validate config.
5. Build a new immutable snapshot.
6. Build a new router and executor for that snapshot.
7. Atomically publish the new generation.
8. Emit `config_reload_succeeded` or `config_reload_failed`.

If any step fails, Arkroute keeps serving with the previous generation. The failed config must not partially update routes, models, providers, auth, or timeout.

Reload can change:

- providers
- models
- routes
- profiles
- upstream timeout
- Claude model discovery flag
- server client key used for local gateway/admin auth

Reload cannot change the listening socket without restart:

- `server.host`
- `server.port`

If the config file changes `server.host` or `server.port`, reload should accept the config only if the values match the running listener captured at process start. Otherwise reload fails with a clear `listener_change_requires_restart` error. This prevents confusing behavior where status says one port while the process still listens on another.

The first successful startup generation is generation `1`. Each successful reload increments by exactly one. Failed reload attempts do not increment the generation.

## Request Semantics

Each incoming request takes one current generation handle at the start of handling. The auth middleware must acquire that generation once, authenticate against that generation's `server.client_key`, and pass the same generation to the handler through a request-local mechanism such as request context. Handlers must not fetch a second generation after auth.

Rules:

- A non-streaming request uses one generation for decode, route planning, upstream execution, and response mapping.
- A streaming request uses one generation until stream close.
- Reload does not cancel in-flight requests.
- New requests after successful reload use the new generation.
- Admin config/routes/status endpoints read one generation consistently per response.
- Count-token requests use the same route planning generation as message requests.

This rule keeps request behavior predictable and avoids mixed-generation execution.

## Shared Runtime State

Health and trace should survive reloads:

- Health store remains process-level and shared across generations.
- Trace sink remains process-level and shared across generations.
- Reload emits trace events using the same sink.
- Existing health entries are not wiped on reload.
- Health entries for removed providers may remain visible until overwritten or until a later explicit pruning phase.

Keeping health across reload avoids hiding recent upstream failures. Pruning old health is useful later, but it is not needed for Phase 3.

## Admin API

Phase 3 keeps all admin APIs local and authenticated.

Existing endpoints continue:

- `GET /internal/status`
- `GET /internal/config`
- `GET /internal/routes`
- `GET /internal/health`
- `GET /healthz`

Add one mutating endpoint:

- `POST /internal/reload`

All internal endpoints require the local client auth key from the current generation at the time the request starts.

`GET /internal/status` should include:

- `schema_version: 1`
- version metadata
- config path
- config generation
- config loaded timestamp
- last reload attempt timestamp
- last successful reload timestamp
- last failed reload timestamp
- last reload error, sanitized
- provider/model/route counts for the current generation
- health summary
- trace stats

`GET /internal/config` should include the redacted current generation config and generation number.

`GET /internal/routes` should include route data for the current generation and generation number.

`GET /internal/health` remains process-level health and should include generation metadata for context.

`GET /healthz` remains unauthenticated and simple. It should not expose secrets, admin metadata, route details, or provider health internals. It may include the current generation's loaded timestamp.

`POST /internal/reload` response on success:

```json
{
  "schema_version": 1,
  "status": "reloaded",
  "generation": 2,
  "config_loaded_at": "2026-06-02T00:00:00Z"
}
```

`POST /internal/reload` response on failure:

```json
{
  "schema_version": 1,
  "status": "failed",
  "generation": 1,
  "error_class": "config_reload_failed",
  "error": "config validation failed: routes[0].targets: must contain at least one target"
}
```

Failure responses should use a non-2xx HTTP status. The previous generation remains active.

Recommended status codes:

- `401` for missing or invalid local client auth.
- `400` for config validation failure or `listener_change_requires_restart`.
- `500` for config read errors, config build errors, or unexpected reload errors.

`POST /internal/reload` must not accept a client-provided config path. It reloads the config path stored by the running process at startup. This prevents an authenticated client from making the server read arbitrary local files.

## CLI UX

Add:

```sh
arkroute reload
```

Behavior:

- Reads config path from `--config` or default path.
- Sends `POST /internal/reload` to the running server.
- Authenticates with the local client key from the config file.
- Prints a short human-readable result.
- Returns non-zero on server unreachable, auth failure, malformed response, or reload failure.

The `--config` flag is used by the CLI to discover the expected local server address and auth key. It does not tell the server which file to reload; the server always reloads its startup config path.

If the edited config changes `server.host` or `server.port`, the default CLI address discovery may point at the new address instead of the running listener. Phase 3 should therefore support:

```sh
arkroute reload --addr http://127.0.0.1:20128
```

`--addr` targets the current running listener and lets the server return the intended `listener_change_requires_restart` error. Without `--addr`, this edge case may surface as `server_unreachable`, which is acceptable CLI behavior but must be explained in the error message.

Example success output:

```text
reloaded generation 2
config_loaded_at: 2026-06-02T00:00:00Z
```

Example failure output:

```text
reload failed: config validation failed: routes[0].targets: must contain at least one target
active generation: 1
```

Update existing commands:

- `arkroute status` prints config generation and last reload state when server is reachable.
- `arkroute doctor` reports whether reload endpoint is reachable when server is running.
- `arkroute config show` remains file-based by default; live config is available through admin API, not a new CLI flag in Phase 3.

## Signal Handling

`arkroute serve` should handle:

- `SIGINT`: graceful shutdown
- `SIGTERM`: graceful shutdown
- `SIGHUP`: reload config

`SIGHUP` reload uses the same code path as `POST /internal/reload`.

Rules:

- A successful `SIGHUP` reload logs a concise message to stdout/stderr.
- A failed `SIGHUP` reload logs the sanitized error and keeps serving the old generation.
- `SIGHUP` should not interrupt graceful shutdown handling.
- On platforms where `SIGHUP` is unavailable, the code should degrade cleanly through build-compatible signal setup. Use a small platform-specific signal helper if needed instead of referencing unavailable constants directly in portable code.

## Auth Behavior

The local client key is part of the current generation.

Rules:

- Existing requests authenticate against the generation selected at request start.
- After a reload changes `server.client_key`, new requests must use the new key.
- `arkroute reload` reads the key from the config file it is asked to reload. This allows key rotation if the process is currently using the same key or if the current key is still accepted by the server.

Key rotation has one edge case: if the file has already changed to a new key, the server still requires the old key for `/internal/reload`. Phase 3 should support an optional CLI flag:

```sh
arkroute reload --client-key <current-running-key>
```

The flag is used only for authenticating the reload request. It is not written to config and must not be logged.

## Error Handling

Add control-plane error categories without replacing Phase 2 upstream error classes:

- `config_reload_failed`
- `config_validation_failed`
- `config_read_failed`
- `listener_change_requires_restart`
- `admin_auth_failed`
- `admin_malformed_response`
- `server_unreachable`

These categories should live in the neutral `internal/failure` package or another dependency-neutral package. They must not live in `internal/runtime` if that would force admin, CLI, config, or adapter packages into an import cycle.

Rules:

- Config validation errors should preserve field paths.
- Secret values must be redacted from reload errors, trace events, admin responses, and CLI output.
- Failed reload must not return provider API keys, authorization headers, prompt bodies, response bodies, or resolved `env:` secret values.
- CLI should distinguish server unreachable from reload failure.

## Trace Events

Add reload events to the existing JSONL trace model:

- `config_reload_started`
- `config_reload_succeeded`
- `config_reload_failed`

Fields:

- schema version
- event type
- timestamp
- request ID when triggered by HTTP
- client name: `admin` or `signal`
- config generation before reload
- config generation after reload on success
- config path
- latency in milliseconds
- error class on failure
- sanitized reason on failure

Trace events must not include raw config contents.

The trace event type should gain explicit optional fields for reload metadata instead of overloading unrelated request fields:

- `config_generation`
- `previous_config_generation`
- `next_config_generation`
- `config_path`

These fields must be omitted when empty or zero for normal request events.

## Testing Strategy

Required test areas:

- Runtime state returns the initial generation.
- Successful reload increments generation and changes routes for new requests.
- Failed reload keeps old generation active.
- Reload failure records sanitized last reload error.
- Reload rejects host/port changes with `listener_change_requires_restart`.
- Health and trace sink survive reload.
- Claude `/v1/models` reflects a newly added route after reload.
- In-flight streaming request keeps its original generation while new requests use the next generation.
- `/internal/status` includes generation and reload metadata.
- `/internal/config` redacts secrets and includes generation.
- `/internal/routes` reflects current generation.
- `POST /internal/reload` requires auth.
- `POST /internal/reload` returns schema version on success and failure.
- `arkroute reload` handles success.
- `arkroute reload` handles server unreachable.
- `arkroute reload --client-key` authenticates against the current running key.
- `arkroute reload --addr` targets the current listener when edited config host/port changed.
- `SIGHUP` uses the same reload path as admin reload.
- `go test -count=1 ./...` passes.

Tests should use `httptest` and temporary config files. Tests that bind local loopback ports may need the same sandbox permission note already documented in README.

## Acceptance Criteria

Phase 3 is complete when:

- There is a centralized runtime state/generation boundary.
- Claude handlers no longer store or depend on a fixed startup snapshot.
- Runtime executor creation is generation-aware.
- New requests after reload use the new generation.
- In-flight requests are not cancelled by reload.
- Failed reload keeps the old generation active.
- Host/port changes during reload fail with a clear restart-required error.
- `/internal/status`, `/internal/config`, `/internal/routes`, and `/internal/health` include generation context.
- `POST /internal/reload` exists, requires auth, and returns stable `schema_version: 1` responses.
- `arkroute reload` works and reports clear success/failure output.
- `arkroute reload --addr` supports reload attempts after config host/port edits.
- `SIGHUP` triggers reload without stopping the server.
- Reload trace events are emitted and redacted.
- All existing Phase 1 and Phase 2 behavior remains compatible.
- `go test -count=1 ./...` passes.

## Future Phase Impact

Phase 3 prepares Arkroute for:

- OpenAI-compatible ingress for OpenCode, Cursor, Cline, Continue, and Codex.
- Dashboard controls backed by a real local admin API.
- Health-aware and latency-aware policies that read stable process-level state.
- Config editing commands that can save and reload atomically.
- Optional file watch reload in a later phase.
- Safer key rotation and provider switching during daily Claude Code usage.
