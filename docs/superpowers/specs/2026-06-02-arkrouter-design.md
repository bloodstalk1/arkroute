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
    supports_streaming: true
    enabled: true

  - id: gemini-pro
    provider_id: gemini
    upstream_model: gemini-2.5-pro
    exposed_alias: gemini-pro
    claude_discovery_alias: claude-gemini-pro
    display_name: Gemini Pro
    supports_streaming: true
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
  cheap: deepseek
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
