# Arkroute OpenAI Platform Compatibility Design

Status: Draft for implementation planning

Date: 2026-06-05

## Goal

Build Arkroute into a local OpenAI-compatible platform surface while preserving the current Claude Code gateway. When all phases in this track are complete, a client that expects the OpenAI API base URL can point at Arkroute and use the supported OpenAI REST surfaces through Arkroute's local routing, provider adapters, auth, tracing, health, and reload boundaries.

This does not mean Arkroute becomes OpenAI's hosted platform. Features that rely on OpenAI-hosted storage, built-in tools, realtime infrastructure, or managed training must be implemented as local Arkroute equivalents, provider-backed adapters, or explicit compatibility stubs with clear errors until an equivalent exists.

## Reference Review

This design uses the current Arkroute codebase and these external references:

- OpenAI Responses API: `POST /v1/responses`, response objects, state, tools, and streaming.
- OpenAI Chat Completions API: `POST /v1/chat/completions`, `GET /v1/models`, and chat streaming.
- OpenAI Batch API: batches can target `/v1/responses`, `/v1/chat/completions`, `/v1/embeddings`, `/v1/completions`, and `/v1/moderations`.
- OpenAI Files and Vector Stores APIs: storage-backed APIs used by batches, file search, and assistant-like workflows.
- OmniRoute API reference and OpenAPI docs: a broad OpenAI-compatible router surface including chat, responses, embeddings, images, files, batches, search, audio, rerank, moderation, dashboard, usage, quotas, and management endpoints.
- OmniRoute CLI tools guide: emphasizes a single `OPENAI_BASE_URL=http://localhost:20128/v1` and `OPENAI_API_KEY=...` setup for many coding tools.
- OmniRoute auto-combo docs: health, quota, cost, latency, task fit, and self-healing routing are separate from the base OpenAI protocol surface.

What Arkroute should copy from OmniRoute conceptually:

- Treat OpenAI compatibility as a product surface, not one endpoint.
- Ship a small reliable core first, then broaden to storage, batches, multimodal, and management.
- Keep CLI tool compatibility docs as first-class acceptance criteria.
- Keep routing intelligence separate from request wire compatibility.

What Arkroute should not copy in this track:

- Large cloud/dashboard/account system before local protocol compatibility works.
- Remote telemetry or cloud request path.
- A broad provider catalog unless each entry has tests and clear adapter behavior.
- Token compression, MCP, A2A, or memory as part of the OpenAI protocol track.

## OmniRoute Patterns Worth Adopting

OmniRoute is much larger than Arkroute should become in this track, but several patterns are worth borrowing.

### 1. Compatibility API And Management API Are Separate Surfaces

OmniRoute documents compatibility endpoints under `/v1/*` and management/dashboard endpoints under `/api/*`. Arkroute should keep the same conceptual split:

- `/v1/*`: OpenAI/Anthropic/Gemini-compatible client APIs.
- `/internal/*`: authenticated local runtime/admin APIs.
- panel routes: browser UI and setup control plane.

This avoids mixing client wire compatibility with operator controls.

### 2. One Public Base URL For Tools

OmniRoute optimizes onboarding around one OpenAI-compatible base URL:

```sh
OPENAI_BASE_URL=http://localhost:20128/v1
OPENAI_API_KEY=...
```

Arkroute should make the same workflow first-class:

```sh
OPENAI_BASE_URL=http://127.0.0.1:2002/v1
OPENAI_API_KEY=<server.client_key>
```

Every OpenAI-compatible phase should include a CLI/tool setup check, not only endpoint tests.

### 3. Endpoint Families Are Product Modules

OmniRoute groups API families clearly: chat, messages, responses, embeddings, images, files, batches, search, audio, moderation, dashboard, usage, quota, resilience, and management. Arkroute should copy the module boundary, not the implementation size.

Recommended Arkroute module split:

- `internal/client/openai`: OpenAI-compatible HTTP ingress.
- `internal/protocol/openai`: OpenAI wire decode/encode and compatibility errors.
- `internal/storage/files`: local file metadata/content store.
- `internal/storage/batches`: batch lifecycle and worker state.
- `internal/storage/responses`: local Responses state.
- `internal/storage/vector`: local vector stores and file search.
- `internal/usage`: request/token/cost counters.

### 4. Provider Execution Is A Shared Core

OmniRoute routes many endpoint formats through a shared translation/execution core. Arkroute already has this pattern with `runtime.Executor`; the OpenAI work should strengthen it:

- OpenAI Chat and Responses should call the same normalized runtime as Claude.
- Embeddings/images/audio should add new normalized protocol families rather than special-case provider calls inside handlers.
- Retry, fallback, trace, and health should remain runtime responsibilities.

### 5. Resilience Is Layered

OmniRoute treats resilience as layered provider health, account cooldown, model lockout, and combo fallback. Arkroute currently has basic health and fallback. The useful lesson is the layering:

- provider-level circuit breaker
- provider+model lockout
- credential/account cooldown
- route/fallback ordering
- probe-based recovery

These should be a later routing-intelligence phase, not part of initial OpenAI wire compatibility.

### 6. Routing Intelligence Uses Multiple Signals

OmniRoute's auto routing scores health, quota, cost, latency, task fit, context window, and recent success. Arkroute should eventually model these as policy inputs:

- health/error rate from runtime attempts
- p50/p95 latency by target
- token/cost estimate by provider/model
- remaining quota or cooldown state
- request requirements such as tools, streaming, vision, context window, and task type

This should plug into `router.Policy` without changing client handlers.

### 7. Compatibility Requires Sanitization

OmniRoute calls out response sanitization and role normalization for OpenAI SDK compatibility. Arkroute needs the same discipline:

- normalize `developer` role safely
- accept string content and array content
- emit stable IDs, `object`, `created`, `model`, and `usage`
- keep Anthropic-specific thinking blocks out of OpenAI responses unless a specific OpenAI-compatible field exists
- return OpenAI-style errors from OpenAI endpoints only

### 8. State Belongs In Local Stores, Not Handlers

OmniRoute uses local persistence for providers, keys, usage, fallback state, budgets, and logs. Arkroute should add local stores only when endpoint families require them:

- Files API requires a file store.
- Batch API requires a batch store and worker state.
- Responses `previous_response_id` requires a response store.
- Vector Stores require chunk/vector storage.

Handlers should orchestrate stores and runtime calls, not own durable state.

### 9. Compatibility Matrix Is A Deliverable

OmniRoute treats CLI tool compatibility as a first-class doc surface. Arkroute should maintain a matrix with:

- endpoint coverage
- supported request fields
- unsupported fields
- tested clients
- required environment variables
- provider capability requirements

This prevents "OpenAI-compatible" from becoming an ambiguous claim.

### 10. Do Not Import The Whole Product Shape Too Early

OmniRoute includes dashboard, OAuth, key management, cloud sync, compression, MCP/A2A, memory, evals, proxies, and many media endpoints. Those are valuable product ideas, but Arkroute's current advantage is a smaller Go runtime with clear local-first boundaries. Borrow the decomposition and compatibility discipline; defer the breadth until the core OpenAI platform surfaces are stable.

## Existing Arkroute Foundations

Arkroute already has the right internal boundaries for the first phases:

- `internal/client/claude` owns Anthropic-compatible HTTP ingress.
- `internal/runtime.State` owns reloadable generations.
- `internal/runtime.Executor` executes normalized requests and streams against routed targets.
- `internal/protocol` contains provider-neutral request, response, stream, capabilities, and usage types.
- `internal/protocol/openai` already contains partial Chat Completions wire structs for the OpenAI-compatible upstream adapter.
- `internal/adapter/openai`, `internal/adapter/anthropic`, and `internal/adapter/gemini` map normalized requests to upstream providers.
- `internal/router` supports route/model alias resolution, capability filtering, fallback policies, and health state.
- `/internal/*` admin APIs already expose generation, config, routes, and health for future dashboard and CLI reuse.

The new work should add OpenAI client ingress packages rather than mixing OpenAI response shapes into the Claude handler.

## Compatibility Definition

Arkroute OpenAI Platform compatibility has three levels:

1. **Wire-compatible**: Arkroute accepts the request shape and returns the expected OpenAI-style response shape.
2. **Runtime-compatible**: Arkroute can fulfill the request through existing provider adapters and routes.
3. **Platform-compatible**: Arkroute provides local equivalents for OpenAI platform storage, state, batch lifecycle, files, vector stores, usage, and management where applicable.

Every endpoint must be classified as one of:

- `implemented`: request can run successfully when provider capabilities exist.
- `stubbed`: request shape is recognized and returns a stable OpenAI-style error explaining the unsupported local feature.
- `out_of_scope`: not part of the current phase and not mounted.

The final track is complete when all targeted OpenAI Platform surfaces are either implemented or intentionally unsupported with stable OpenAI-style errors, documented behavior, and tests.

## Non-Goals

- Do not change provider routing behavior while building the initial OpenAI ingress.
- Do not add cloud sync, user accounts, billing, or remote telemetry.
- Do not implement token compression in this track.
- Do not make OpenAI-specific provider assumptions leak into normalized runtime types.
- Do not store prompt or response bodies in trace logs.
- Do not expose provider API keys or the local client key to browser JavaScript.

## Architecture

Add a separate OpenAI client ingress boundary:

```text
OpenAI-compatible client
  -> /v1/models, /v1/chat/completions, /v1/responses, ...
  -> internal/client/openai.Server
  -> internal/protocol/openai client codecs
  -> protocol.Request / router.Requirements
  -> runtime.Generation.Execute or Stream
  -> provider adapters
  -> protocol.Response / protocol.StreamEvent
  -> OpenAI-compatible response codecs
```

The `claude.Server` can either mount the new OpenAI routes directly or the top-level server can compose multiple client handlers. The implementation should avoid duplicating auth and generation handling by extracting shared local auth helpers if needed.

## Auth

OpenAI-compatible endpoints use:

```text
Authorization: Bearer <server.client_key>
```

This deliberately matches existing local client auth. It lets users configure:

```sh
export OPENAI_BASE_URL="http://127.0.0.1:2002/v1"
export OPENAI_API_KEY="<server.client_key>"
```

The first phase should not add per-client API keys. Per-client key scopes and budgets can come later with usage/quota tracking.

## Model Listing

`GET /v1/models` should return an OpenAI-compatible list object:

```json
{
  "object": "list",
  "data": [
    {
      "id": "sonnet",
      "object": "model",
      "created": 0,
      "owned_by": "arkroute"
    }
  ]
}
```

It should include route aliases and enabled exposed model aliases. Claude discovery aliases remain Claude-specific and should not be emitted unless they are also route aliases or exposed aliases.

## Error Shape

OpenAI ingress must not return Anthropic error objects. Use a stable OpenAI-style shape:

```json
{
  "error": {
    "message": "model or route \"missing\" not found",
    "type": "invalid_request_error",
    "param": "model",
    "code": "route_not_found"
  }
}
```

Recommended status mapping:

- auth failure: `401 authentication_error`
- malformed JSON: `400 invalid_request_error`
- unsupported parameter/feature: `400 unsupported_feature`
- route not found: `404 route_not_found`
- unsupported capability: `400 unsupported_capability`
- upstream auth: `403 upstream_auth`
- upstream rate limit: `429 upstream_rate_limit`
- retryable/fatal upstream failure: `502 upstream_error`

## Streaming

OpenAI Chat Completions streaming should emit OpenAI-compatible SSE lines:

```text
data: {"id":"chatcmpl_...","object":"chat.completion.chunk","choices":[...]}

data: [DONE]
```

Responses streaming should emit Responses-style event names and payloads for the implemented subset. Events can be derived from `protocol.StreamEvent`, but the OpenAI ingress owns the wire taxonomy.

## Tool Mapping

The core tool subset is function calling:

- Chat request `tools[].type == "function"` maps to `protocol.Tool`.
- Chat assistant `tool_calls[]` maps to assistant `tool_use` blocks.
- Chat `tool` role messages map to `tool_result` blocks.
- Responses `tools` supports only function tools in the first Responses phase.
- OpenAI hosted tools such as web search, file search, computer use, code interpreter, vector store search, and image generation tools return clear unsupported errors until the relevant platform phase exists.

## Phase Breakdown

### Phase 4A: OpenAI Chat Core

Ship the smallest reliable OpenAI-compatible coding-client surface.

Endpoints:

- `GET /v1/models`
- `POST /v1/chat/completions`

Supported:

- non-streaming text chat
- streaming text chat
- function tools and tool results
- `system`, `developer`, `user`, `assistant`, and `tool` roles
- `content` as string or text block arrays
- `max_tokens`, `max_completion_tokens`, `temperature`, `stream`
- `reasoning_effort` pass-through into normalized request
- local auth and generation snapshot consistency
- fallback, health, trace, and reload compatibility through existing runtime

Not supported yet:

- `n > 1`
- `logprobs`
- audio output
- `store`
- OpenAI-specific hosted tools

Completion criteria:

- OpenAI-compatible clients can set `OPENAI_BASE_URL=http://127.0.0.1:2002/v1` and run text/tool chat through Arkroute.
- Existing Claude endpoints continue to pass all tests.

### Phase 4B: Responses API Core

Add the modern OpenAI text-generation surface as a local compatibility subset.

Endpoints:

- `POST /v1/responses`

Supported:

- `input` as string
- `input` as message item array
- `instructions`
- `model`
- `max_output_tokens`
- `temperature`
- `stream`
- function tools
- text output
- function-call output items
- usage summary

Stubbed with clear errors:

- `previous_response_id`
- `store: true`
- `include` fields that require hosted state
- built-in tools
- file search, web search, computer use
- background mode
- encrypted reasoning/state carryover

Completion criteria:

- Clients that use simple `responses.create({ model, input })` can run against Arkroute.
- Streamed Responses text works for the implemented subset.
- Unsupported OpenAI hosted features fail predictably instead of silently misbehaving.

### Phase 4C: Compatibility Hardening

Make the text-generation surface resilient with real-world client shapes.

Targets:

- Cursor-style Chat Completions requests
- Cline/OpenCode/Continue-style Chat Completions requests
- Codex-style OpenAI base URL configuration where applicable
- OpenAI SDK request/response parsing for Chat and Responses

Work:

- tolerant parsing for content arrays and multimodal placeholders
- stable IDs and `created` timestamps
- response headers that common SDKs expect
- `HEAD`/method rejection behavior where needed
- client-specific README examples
- fixture-driven tests for common client payloads

Completion criteria:

- Common OpenAI-compatible coding clients have documented setup examples.
- Fixture tests cover request shapes that Arkroute accepts.

### Phase 5A: Embeddings And Legacy Completions

Add non-chat text inference APIs that many libraries expect.

Endpoints:

- `POST /v1/embeddings`
- `POST /v1/completions`

Requirements:

- Add normalized embedding request/response types.
- Add provider adapter support for OpenAI-compatible embeddings first.
- Return stable unsupported errors for providers without embeddings.
- Implement legacy completions by adapting prompt strings into normalized chat requests or explicit provider completions where available.

Completion criteria:

- Embedding-capable OpenAI-compatible providers can serve embedding requests.
- Batch phase can target embeddings later.

### Phase 5B: Moderations And Rerank

Add safety/classification and ranking surfaces.

Endpoints:

- `POST /v1/moderations`
- `POST /v1/rerank` as an OpenAI-compatible extension, not official OpenAI core

Requirements:

- Add capability records for moderation/rerank.
- Add provider adapters only where real provider APIs exist.
- Return explicit unsupported errors otherwise.

Completion criteria:

- Moderation requests are wire-compatible.
- Rerank is documented as an Arkroute extension inspired by router ecosystems, not OpenAI core.

### Phase 6A: Files API

Create local file storage needed by batches and future vector stores.

Endpoints:

- `POST /v1/files`
- `GET /v1/files`
- `GET /v1/files/{file_id}`
- `DELETE /v1/files/{file_id}`
- `GET /v1/files/{file_id}/content`

Requirements:

- Store files under Arkroute local data directory.
- Scope files to local client key identity until per-client keys exist.
- Enforce file size limits and path safety.
- Track purpose, filename, bytes, created_at, and status metadata.

Completion criteria:

- Uploaded JSONL files can be used by local batch execution.
- File content is never logged.

### Phase 6B: Batch API

Add local asynchronous batch execution.

Endpoints:

- `POST /v1/batches`
- `GET /v1/batches`
- `GET /v1/batches/{batch_id}`
- `POST /v1/batches/{batch_id}/cancel`

Supported batch targets:

- `/v1/responses`
- `/v1/chat/completions`
- `/v1/embeddings`
- `/v1/completions`
- `/v1/moderations`

Requirements:

- Use uploaded JSONL files as input.
- Execute with bounded local concurrency.
- Write output and error JSONL files into local file storage.
- Persist batch status enough to survive process restart.

Completion criteria:

- Batch lifecycle mirrors OpenAI shape for local supported endpoints.

### Phase 6C: Response State Store

Implement local state for Responses API.

Endpoints:

- `GET /v1/responses/{response_id}`
- `DELETE /v1/responses/{response_id}`
- `GET /v1/responses/{response_id}/input_items`

Requirements:

- Store request metadata, response output items, and input items locally when `store` is true.
- Support `previous_response_id` by reconstructing normalized context from stored input/output.
- Add retention controls.

Completion criteria:

- Simple stateful Responses workflows can run locally.

### Phase 7A: Vector Stores And File Search

Add local vector-store equivalents for OpenAI file search workflows.

Endpoints:

- `POST /v1/vector_stores`
- `GET /v1/vector_stores`
- `GET /v1/vector_stores/{id}`
- `DELETE /v1/vector_stores/{id}`
- `POST /v1/vector_stores/{id}/files`
- `GET /v1/vector_stores/{id}/files`
- `DELETE /v1/vector_stores/{id}/files/{file_id}`
- `POST /v1/vector_stores/{id}/file_batches`
- `GET /v1/vector_stores/{id}/file_batches/{batch_id}`
- `POST /v1/vector_stores/{id}/search`

Requirements:

- Use local embeddings provider support from Phase 5A.
- Store chunks and vectors locally.
- Expose file search as a Responses built-in tool after vector stores exist.

Completion criteria:

- A local Responses request can use file search against an Arkroute vector store.

### Phase 7B: Image And Vision Platform APIs

Add multimodal generation and edit surfaces.

Endpoints:

- `POST /v1/images/generations`
- `POST /v1/images/edits`
- optionally `POST /v1/images/variations` if provider-backed

Requirements:

- Add normalized image generation/edit types.
- Add provider adapters for available image providers.
- Return OpenAI-compatible image response objects.
- Keep vision input in Chat/Responses separate from image generation output.

Completion criteria:

- Provider-backed image generation works through OpenAI-compatible request shapes.

### Phase 7C: Audio APIs

Add speech and transcription surfaces.

Endpoints:

- `POST /v1/audio/transcriptions`
- `POST /v1/audio/translations`
- `POST /v1/audio/speech`

Requirements:

- Add multipart parsing for audio input.
- Add provider adapters for STT/TTS providers.
- Return raw audio bodies for speech where requested.

Completion criteria:

- OpenAI-compatible STT and TTS clients can route through Arkroute when configured providers support them.

### Phase 8A: Realtime Compatibility

Add realtime APIs after core REST surfaces are stable.

Endpoints:

- Realtime session creation endpoints as needed by OpenAI clients.
- WebSocket endpoint for realtime events.

Requirements:

- Define local event protocol mapping.
- Support text-first realtime before audio.
- Add cancellation and backpressure handling.

Completion criteria:

- Basic realtime text session can run locally.

### Phase 8B: OpenAI SDK And CLI Compatibility Matrix

Treat compatibility as a testable product.

Work:

- Add fixtures for official OpenAI SDKs where practical.
- Add docs for Cursor, OpenCode, Cline, Continue, Codex-style OpenAI clients, and generic SDK usage.
- Add `arkroute openai doctor` or extend `arkroute doctor` to print OpenAI base URL setup.
- Add endpoint coverage table to README.

Completion criteria:

- Compatibility status is visible and repeatable.

### Phase 8C: Usage, Quotas, And Routing Intelligence

Build the router intelligence that OmniRoute treats as a separate layer.

Work:

- Track request counts, token usage, latency, and error rates per route/provider/model/client.
- Add optional budgets per local API key after per-client keys exist.
- Add health-aware, latency-aware, cost-aware, and quota-aware policies.
- Add circuit breaker and cooldown behavior.

Completion criteria:

- Arkroute can choose targets based on health and local policy, not only static order.

## Testing Strategy

Every phase must include:

- targeted unit tests for OpenAI request decode and response encode
- HTTP integration tests using `httptest` fake upstreams
- auth failure tests
- route-not-found and unsupported-feature tests
- streaming tests for SSE shape
- reload generation tests where endpoints execute through runtime
- regression tests proving Claude endpoints still pass

Before each phase is accepted:

```sh
gofmt -w <changed-go-files>
go test -count=1 ./...
npm test --prefix npm/arkroute
```

When frontend assets change:

```sh
npm run build --prefix web-ui
make build-frontend
```

## Documentation Strategy

The README should grow a dedicated OpenAI-compatible section:

```sh
export OPENAI_BASE_URL="http://127.0.0.1:2002/v1"
export OPENAI_API_KEY="$(arkroute config show --field server.client_key)"
```

If exposing the client key through CLI output is not desired, document how to copy it from redacted-safe local setup output or add a purpose-built command that prints only local activation exports.

Each endpoint group should have:

- supported request fields
- unsupported request fields
- example curl request
- compatible tools tested
- provider capability requirements

## Risks

- The phrase "OpenAI Platform complete" can become unbounded. The track must define endpoint coverage explicitly.
- Responses API state, files, vector stores, and batches require local persistence. That should not be mixed into Phase 4.
- Multimodal APIs require new normalized protocol families and provider adapters.
- Some OpenAI hosted tools cannot be faithfully emulated without local equivalents.
- Client compatibility often fails on small schema differences; fixture tests are mandatory.

## Acceptance Criteria For The Full Track

The OpenAI Platform compatibility track is complete when:

- `GET /v1/models` returns OpenAI-compatible model list data.
- Chat Completions supports non-streaming, streaming, function tools, and common coding-client payloads.
- Responses supports stateless and stored-state text/function workflows.
- Embeddings works for embedding-capable providers.
- Files and Batches work locally for supported endpoints.
- Vector Stores and file search work locally when embeddings are configured.
- Image and audio endpoints work when compatible providers are configured.
- Unsupported hosted-only features return stable OpenAI-style errors.
- README documents endpoint coverage and client setup.
- `go test -count=1 ./...` and npm launcher tests pass.
- Existing Claude Code functionality remains compatible.
