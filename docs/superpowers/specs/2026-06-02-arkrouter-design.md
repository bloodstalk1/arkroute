# Arkrouter Design

Date: 2026-06-02
Status: Approved for implementation planning

## Goal

Arkrouter is a local AI model router for coding tools. The long-term product is multi-tool, but the first milestone focuses on Claude Code CLI only.

Phase 1 must make Claude Code CLI work reliably through a local Anthropic-compatible gateway while keeping the internal architecture ready for future OpenCode, Cursor, Cline, Codex, and other clients.

## Design Principles

- Keep the core router independent from Claude Code specifics.
- Separate client protocols from provider protocols.
- Prefer local-first operation with no telemetry by default.
- Make debugging easy because coding CLIs often surface upstream failures unclearly.
- Use versioned config so future schema changes can be migrated instead of forcing manual rewrites.
- Avoid large product features in MVP until the protocol and routing core are stable.

## References And Patterns

Arkrouter borrows these patterns from existing tools:

- OmniRoute and 9Router: one local endpoint, multiple upstream providers, local-first routing, fallback as a core feature.
- claude-code-router: model aliases, scenario-oriented routing, and Claude Code activation ergonomics.
- CCRelay: strict separation between incoming client protocol and outgoing upstream provider protocol.
- Claude Code LLM gateway docs: Anthropic Messages compatibility, model discovery, and environment-based activation.
- OpenCode provider docs: future OpenAI-compatible client ingress should be easy to add, but it is outside phase 1.

## Phase 1 Scope

Phase 1 supports Claude Code CLI through Anthropic-compatible HTTP endpoints:

- `GET /v1/models`
- `POST /v1/messages`
- `POST /v1/messages/count_tokens`
- `GET /healthz`

Supported upstream provider adapters:

- `openai_compatible`: OpenRouter, DeepSeek, Groq, LiteLLM, local vLLM, and similar APIs.
- `gemini`: Google Gemini API.
- `anthropic`: direct Anthropic passthrough.

Supported routing strategies:

- `priority`: use the first enabled healthy target.
- `fallback`: try the next target on retryable upstream failures.

Phase 1 does not include:

- OpenCode, Cursor, Cline, or Codex ingress endpoints.
- Web dashboard.
- Cloud sync.
- OAuth or subscription account pooling.
- MCP or A2A server support.
- Token compression.
- Team billing or multi-user admin features.

## Architecture

High-level request path:

```text
Claude Code CLI
  -> Claude client adapter
  -> normalized request
  -> core router
  -> provider adapter
  -> upstream provider
  -> normalized response
  -> Claude client adapter
  -> Anthropic-compatible response
```

Primary internal boundaries:

- `client/claude`: HTTP handlers for Claude Code's Anthropic-compatible API.
- `router`: alias resolution, route selection, fallback, health decisions.
- `adapter/openai`: OpenAI-compatible provider request and response mapping.
- `adapter/gemini`: Gemini provider request and response mapping.
- `adapter/anthropic`: Anthropic passthrough behavior.
- `protocol`: shared normalized request and response structures plus provider wire types.
- `config`: local YAML loading, validation, defaults, and future migrations.
- `observability`: logs, request IDs, health state, and local debugging support.

Future client ingress can add packages such as `client/openai` or `client/responses` without rewriting the core router.

## Technology Stack

The core router is implemented in Go.

Reasons:

- Produces a small native binary for local CLI use.
- Handles HTTP and SSE streaming well.
- Has simpler long-term maintenance than Rust for this project.
- Avoids a Node or Bun runtime requirement for phase 1.
- Fits the existing local Go gateway experience already used in related work.

A dashboard can be added later as a TypeScript or React frontend that calls Arkrouter's local admin API. Routing logic should stay in the Go core.

## Project Shape

Initial repository path:

```text
/Users/bat/RiderProjects/arkrouter
```

Go module:

```text
bat.dev/arkrouter
```

Binary:

```text
arkrouter
```

Proposed directory structure:

```text
arkrouter/
  cmd/arkrouter/
    main.go
  internal/
    app/
    cli/
    config/
    client/
      claude/
    router/
    adapter/
      anthropic/
      openai/
      gemini/
    protocol/
      anthropic/
      openai/
      gemini/
    observability/
  docs/
  go.mod
  README.md
```

## Configuration

Default config path:

```text
~/.arkrouter/config.yaml
```

Initial format:

```yaml
version: 1

server:
  host: 127.0.0.1
  port: 20128
  client_key: env:ARKROUTER_CLIENT_KEY
  upstream_timeout_seconds: 600

clients:
  claude:
    enabled: true
    model_discovery: true

providers:
  - id: openrouter
    name: OpenRouter
    type: openai_compatible
    base_url: https://openrouter.ai/api/v1
    api_key: env:OPENROUTER_API_KEY
    headers:
      HTTP-Referer: https://localhost
      X-OpenRouter-Title: Arkrouter

  - id: gemini
    name: Gemini
    type: gemini
    base_url: https://generativelanguage.googleapis.com/v1beta
    api_key: env:GEMINI_API_KEY

models:
  - id: openrouter-sonnet
    provider_id: openrouter
    upstream_model: anthropic/claude-sonnet-4.5
    exposed_alias: sonnet
    claude_discovery_alias: claude-sonnet-4-20250514
    display_name: Claude Sonnet via OpenRouter
    capabilities:
      streaming: true
      tools: true
      tool_results: true
      vision: false
      system_messages: true
      prompt_cache: false
      context_window: 200000
      max_output_tokens: 8192
    enabled: true

  - id: gemini-pro
    provider_id: gemini
    upstream_model: gemini-2.5-pro
    exposed_alias: gemini-pro
    claude_discovery_alias: claude-gemini-pro
    display_name: Gemini Pro
    capabilities:
      streaming: true
      tools: true
      tool_results: true
      vision: false
      system_messages: true
      prompt_cache: false
      context_window: 1000000
      max_output_tokens: 8192
    enabled: true

routes:
  - alias: sonnet
    claude_discovery_alias: claude-sonnet-4-20250514
    strategy: fallback
    targets:
      - model_id: openrouter-sonnet
      - model_id: gemini-pro

profiles:
  default: sonnet
  best: sonnet
```

Rules:

- `providers` hold endpoint and authentication data.
- `models` map provider models to internal model IDs and exposed aliases.
- `routes` are the aliases clients select.
- `claude_discovery_alias` lets Claude Code discover model IDs that look like Claude models.
- API keys should use `env:NAME` references by default.
- Plaintext API keys may be supported for convenience but should not be encouraged.
- Config has `version: 1` from the start to support future migrations.

Validation requirements:

- `server.host` must default to loopback and phase 1 should reject non-loopback hosts unless an explicit unsafe override is added later.
- Provider IDs, model IDs, route aliases, and discovery aliases must be unique within their namespaces.
- Enabled models must reference enabled providers.
- Enabled routes must contain at least one enabled target.
- Route targets must reference enabled models.
- `claude_discovery_alias` values should be empty or begin with `claude` or `anthropic` so Claude Code model discovery behaves predictably.
- Profiles must reference existing route aliases or exposed model aliases.
- Secret values must be redacted in validation errors, logs, status output, and tests.

Config loading should produce an immutable runtime snapshot. Health state, request logs, and counters live outside that snapshot so future reloads can swap config safely without racing request handlers.

## Normalized Protocol Contract

The core router must not route raw Anthropic, OpenAI, or Gemini wire structs directly. Client adapters convert incoming requests into a normalized protocol first, and provider adapters convert normalized protocol into upstream wire formats.

The normalized request should represent these concepts explicitly:

- model alias requested by the client
- system messages or system text blocks
- ordered conversation messages
- content blocks: text, image metadata, tool use, and tool result
- tool definitions with JSON schemas
- tool choice
- max output tokens
- temperature and common sampling fields when present
- stream flag
- client metadata and safe pass-through headers

The normalized response should represent:

- response ID
- output role
- ordered content blocks
- stop reason
- usage if available
- provider metadata needed for logs but not exposed as client protocol fields

The normalized stream should use provider-neutral events:

- message start
- content block start
- content delta
- content block stop
- message delta
- message stop
- error

Adapters can preserve provider-specific details in metadata, but the router must make decisions from normalized capabilities and route state, not from provider wire structs.

## Capability Matrix

Every model should expose a capability record. Phase 1 should include:

- `streaming`
- `tools`
- `tool_results`
- `vision`
- `system_messages`
- `prompt_cache`
- `context_window`
- `max_output_tokens`

The router should check required capabilities before selecting a target. Examples:

- A request with tools must not be sent to a target with `tools: false`.
- A request with images must not be sent to a target with `vision: false`.
- A streaming request must prefer targets with `streaming: true`; if none exist, return a clear unsupported-capability error rather than silently switching to non-streaming.

Capability checks are part of routing, not provider adapter best-effort behavior. This prevents a weak provider from failing late after the request has already been transformed.

## Adapter Contracts

Provider adapters must implement a small stable contract:

- Validate that the normalized request can be represented by the provider.
- Build the upstream HTTP request path, method, headers, and body.
- Map non-streaming upstream responses into normalized responses.
- Map streaming upstream events into normalized stream events.
- Classify upstream failures as retryable, fatal, auth, rate-limit, or invalid-request.

Adapter instances should be stateless. Streaming mappers may hold per-request state because provider stream formats often require stateful conversion.

Provider adapter packages must not read config files directly, mutate route state, or write logs directly. They receive explicit inputs and return structured outputs. This keeps adapter tests small and makes future client protocols easier to add.

## Provider URL And Header Rules

OpenAI-compatible providers differ in base URL shape. The OpenAI-compatible adapter should normalize these safely:

- Base URL may end at host, `/v1`, or another provider-specific API prefix.
- Chat completions should resolve to exactly one `/chat/completions` path.
- Model listing should resolve to exactly one `/models` path where supported.
- Duplicate `/v1/v1` and missing `/v1` cases must be covered by tests.

Header handling:

- Client `Authorization` must never be forwarded upstream.
- Upstream auth is built from provider config only.
- Safe Claude Code headers, such as `anthropic-version` and `anthropic-beta`, may be preserved in request metadata for adapters that need them.
- OpenRouter should receive `HTTP-Referer` and `X-OpenRouter-Title` when configured.
- All configured custom headers must be redacted if they look like secrets.

## CLI UX

Phase 1 commands:

```sh
arkrouter init
arkrouter provider add openrouter
arkrouter provider add gemini
arkrouter provider add anthropic
arkrouter provider add openai-compatible
arkrouter model add
arkrouter route add sonnet
arkrouter serve
arkrouter validate
arkrouter activate claude
arkrouter status
arkrouter doctor
arkrouter test sonnet "hello"
arkrouter logs --tail
```

Claude activation output:

```sh
export ANTHROPIC_BASE_URL='http://127.0.0.1:20128'
export ANTHROPIC_AUTH_TOKEN='<local-client-key>'
export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY='1'
```

`arkrouter activate claude --write-settings` may be added if it can safely write `.claude/settings.local.json` without overwriting unrelated user settings.

## Security Model

Phase 1 is local-first:

- The HTTP server binds to loopback by default.
- The local client key protects the gateway from accidental local cross-process access.
- Upstream API keys should use environment references in generated config.
- Config files containing generated local secrets should be written with owner-only permissions where the OS supports it.
- Logs must not contain API keys, auth headers, prompt body, response body, or resolved secret values.
- Debug body logging is not part of phase 1.

The local client key is not a provider secret. It can be generated and stored in the local config to make Claude Code activation simple. Provider keys are more sensitive and should remain in environment variables by default.

## Request Flow

For `/v1/messages`:

1. Validate request method and client auth.
2. Read and decode Anthropic Messages request.
3. Resolve `model` against route aliases, Claude discovery aliases, and direct model aliases.
4. Convert the request to Arkrouter's normalized internal shape.
5. Ask the router for an ordered target list.
6. Convert the normalized request through the selected provider adapter.
7. Call upstream with timeout and forwarded safe headers.
8. Convert upstream response back to Anthropic-compatible shape.
9. Record health, latency, status, and fallback metadata.
10. Return response or stream to Claude Code.

## Fallback And Error Handling

Retry the next route target only for:

- network errors
- upstream timeout
- HTTP `408`
- HTTP `429`
- HTTP `500`
- HTTP `502`
- HTTP `503`
- HTTP `504`

Do not fallback for:

- invalid client request
- invalid API key
- invalid or missing upstream model
- unsupported tool or content feature
- provider authentication failure
- provider permission failure

Streaming behavior:

- Fallback is allowed before the first event is sent to Claude Code.
- Once the response stream has started, Arkrouter must not switch upstream providers mid-stream.

All user-facing errors returned to Claude Code should use Anthropic-compatible error shape:

```json
{
  "type": "error",
  "error": {
    "type": "api_error",
    "message": "upstream failed after 2 targets"
  }
}
```

## Observability

Every request gets a `request_id`.

Default logs include:

- request ID
- client adapter
- route alias
- selected target
- upstream provider
- upstream model
- latency
- HTTP status
- fallback reason
- retry count

Default logs must not include:

- prompt content
- response content
- API keys
- full authorization headers

Health state:

- `unknown`: no recent result.
- `ok`: last relevant request succeeded.
- `degraded`: retryable upstream failure happened.
- `unhealthy`: auth/config failure or repeated fatal failures.

`arkrouter status`, `arkrouter doctor`, and `GET /healthz` should expose enough data to debug Claude Code failures without turning on body logging.

## Runtime Model

The server runtime should be built from an immutable config snapshot:

- Request handlers read config through a snapshot pointer.
- Route resolution uses maps built at load time, not repeated linear scans on every request.
- Health state is separate from config and protected by a narrow synchronization boundary.
- Upstream HTTP clients must use explicit timeouts.
- Server shutdown should respect request contexts.

This runtime model keeps phase 1 simple while allowing future config reloads without redesigning request handling.

## Implementation Slices

Implementation should happen in thin vertical slices instead of building all adapters at once:

1. Repository scaffold, Go module, CLI shell, config load and validation.
2. Claude activation, `/healthz`, `/v1/models`, and static route/model snapshot.
3. Anthropic request decode/encode and local non-streaming test adapter.
4. OpenAI-compatible non-streaming adapter.
5. OpenAI-compatible streaming adapter and SSE conversion.
6. Tool use and tool result conversion for OpenAI-compatible providers.
7. Fallback routing, retry classification, and health state.
8. Gemini adapter.
9. Anthropic passthrough adapter.
10. Logs, status, doctor, and test commands.

Each slice should leave the binary buildable and tests passing.

## Testing Strategy

Required test areas:

- Config loading and validation.
- `env:NAME` secret resolution.
- Route alias and Claude discovery alias resolution.
- `priority` and `fallback` target selection.
- Retryability classification.
- Anthropic request decoding and response encoding.
- Anthropic-to-OpenAI-compatible request conversion.
- OpenAI-compatible-to-Anthropic response conversion.
- SSE stream conversion.
- Gemini request and response conversion.
- HTTP integration tests for `/v1/models`, `/v1/messages`, and `/v1/messages/count_tokens`.

Golden fixtures should be used for protocol mapping. This reduces regression risk when provider adapters evolve.

Acceptance criteria for phase 1:

- `arkrouter init` creates a valid local config.
- `arkrouter validate` accepts generated config and rejects invalid references.
- `arkrouter activate claude` prints working Claude Code environment exports.
- `arkrouter serve` starts on loopback with the configured client key.
- `/v1/models` returns route aliases and Claude discovery aliases.
- `/v1/messages` can complete a non-streaming OpenAI-compatible upstream request.
- `/v1/messages` can complete a streaming OpenAI-compatible upstream request.
- Tool use round-trips through an OpenAI-compatible provider fixture.
- Fallback tries the second target on timeout, `429`, and `5xx`.
- Auth/config failures do not fallback.
- Logs show routing, latency, status, and fallback reason without prompt or secret data.
- `go test ./...` passes.

## Implementation Planning Decisions

- Upstream API keys support both `env:NAME` references and plaintext values, but CLI-generated config should prefer `env:NAME`.
- `arkrouter init` should generate a local client key into `~/.arkrouter/config.yaml`; this key protects the local loopback gateway and is not an upstream provider secret.
- Local trace logs should be JSONL from day one so `arkrouter logs`, future dashboard views, and support tooling can parse them consistently.
- Gemini phase 1 should include text, streaming, and basic function tool mapping. Advanced multimodal input, prompt caching, and provider-specific Gemini tuning are later features.

## Approved Decisions

- Project name: `arkrouter`.
- Initial path: `/Users/bat/RiderProjects/arkrouter`.
- Long-term direction: multi-tool router.
- Phase 1 focus: Claude Code CLI.
- Core language: Go.
- Dashboard: not in MVP; possible TypeScript frontend later.
- Core router must be protocol-neutral internally.
