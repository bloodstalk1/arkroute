# OpenAI Platform Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Arkroute's OpenAI-compatible platform track in phases, starting with Chat Completions and Responses, then expanding through embeddings, files, batches, vector stores, multimodal APIs, realtime, compatibility docs, and routing intelligence.

**Architecture:** Add OpenAI client ingress as a separate HTTP and codec boundary that maps OpenAI wire requests into Arkroute's existing normalized runtime. Broader OpenAI Platform surfaces introduce new normalized protocol families and local persistence only after the text-generation surface is stable.

**Tech Stack:** Go 1.23+, `net/http`, `httptest`, JSON and SSE codecs, local filesystem persistence for later storage phases, existing Arkroute runtime/router/adapter packages, npm launcher tests.

---

## Source Spec

Implement from `docs/superpowers/specs/2026-06-05-openai-platform-compatibility-design.md`.

The track is intentionally split into phases. Do not start a later phase until the prior phase has passing tests and documented endpoint behavior.

## Phase Overview

1. **Phase 4A:** OpenAI Chat Core.
2. **Phase 4B:** Responses API Core.
3. **Phase 4C:** Compatibility Hardening.
4. **Phase 5A:** Embeddings And Legacy Completions.
5. **Phase 5B:** Moderations And Rerank.
6. **Phase 6A:** Files API.
7. **Phase 6B:** Batch API.
8. **Phase 6C:** Response State Store.
9. **Phase 7A:** Vector Stores And File Search.
10. **Phase 7B:** Image And Vision Platform APIs.
11. **Phase 7C:** Audio APIs.
12. **Phase 8A:** Realtime Compatibility.
13. **Phase 8B:** OpenAI SDK And CLI Compatibility Matrix.
14. **Phase 8C:** Usage, Quotas, And Routing Intelligence.

## Cross-Phase Rules

- Preserve all current Claude behavior.
- Keep OpenAI response/error shapes out of `internal/client/claude`.
- Use `Authorization: Bearer <server.client_key>` for OpenAI-compatible local auth until a per-client key phase exists.
- Never log prompt bodies, response bodies, uploaded file content, provider keys, or local client keys.
- Each endpoint must be tested as implemented, stubbed, or out of scope.
- Use TDD for each phase: failing test, failing command output, implementation, passing command output.
- Run `gofmt -w` and `go test -count=1 ./...` before phase completion.
- Run `npm test --prefix npm/arkroute` before phase completion.

## Phase 4A: OpenAI Chat Core

### File Structure

Create:

- `internal/client/openai/server.go`: OpenAI-compatible route registration and handler dependencies.
- `internal/client/openai/auth.go`: shared local bearer auth and runtime generation context for OpenAI handlers.
- `internal/client/openai/models.go`: OpenAI model-list handler.
- `internal/client/openai/chat.go`: Chat Completions handler.
- `internal/client/openai/stream.go`: Chat Completions SSE writer.
- `internal/client/openai/errors.go`: OpenAI-compatible error responses and execution-error mapping.
- `internal/client/openai/server_test.go`: OpenAI ingress HTTP integration tests.
- `internal/protocol/openai/client_codec.go`: OpenAI client request decoding into normalized protocol.
- `internal/protocol/openai/client_codec_test.go`: codec unit tests.

Modify:

- `internal/client/claude/server.go`: mount OpenAI handler routes or delegate shared route composition.
- `internal/protocol/openai/types.go`: add missing client response and stream chunk structs if useful.
- `README.md`: add OpenAI Chat setup section.

### Acceptance Criteria

- `GET /v1/models` returns OpenAI list shape.
- `POST /v1/chat/completions` supports non-streaming text.
- `POST /v1/chat/completions` supports streaming text.
- Function tools and tool results round-trip.
- OpenAI errors are not Anthropic-shaped.
- Existing Claude tests still pass.

### Task 1: OpenAI Client Codec

**Files:**

- Create: `internal/protocol/openai/client_codec.go`
- Create: `internal/protocol/openai/client_codec_test.go`
- Modify: `internal/protocol/openai/types.go`

- [ ] Write codec tests for string content, text-array content, developer role, tool calls, and tool role messages.
- [ ] Run `go test -count=1 ./internal/protocol/openai` and verify failure because the codec does not exist.
- [ ] Implement `DecodeChatRequest(body []byte) (ChatRequest, error)`.
- [ ] Implement `NormalizeChatRequest(req ChatRequest) (protocol.Request, router.Requirements, error)`.
- [ ] Treat `max_completion_tokens` as equivalent to `max_tokens` when `max_tokens` is absent.
- [ ] Map `developer` and `system` messages into normalized system text blocks.
- [ ] Map OpenAI function tools into `protocol.Tool`.
- [ ] Map assistant `tool_calls` into `tool_use` blocks.
- [ ] Map tool role messages into `tool_result` blocks.
- [ ] Run `gofmt -w internal/protocol/openai/client_codec.go internal/protocol/openai/client_codec_test.go internal/protocol/openai/types.go`.
- [ ] Run `go test -count=1 ./internal/protocol/openai`.
- [ ] Commit with `feat: add openai chat client codec`.

### Task 2: OpenAI Error Shape And Auth

**Files:**

- Create: `internal/client/openai/errors.go`
- Create: `internal/client/openai/auth.go`
- Create: `internal/client/openai/server_test.go`

- [ ] Write HTTP tests for missing bearer token, bad token, malformed JSON, missing route, and unsupported capability.
- [ ] Run `go test -count=1 ./internal/client/openai` and verify failure because the package does not exist.
- [ ] Implement `openAIError(w, status, errorType, code, param, message)`.
- [ ] Implement execution error mapping from `runtime.ExecutionError` to OpenAI status/type/code.
- [ ] Implement local bearer auth that attaches `*runtime.Generation` to request context.
- [ ] Run `gofmt -w internal/client/openai`.
- [ ] Run `go test -count=1 ./internal/client/openai`.
- [ ] Commit with `feat: add openai ingress auth and errors`.

### Task 3: Models Endpoint

**Files:**

- Create: `internal/client/openai/server.go`
- Create: `internal/client/openai/models.go`
- Modify: `internal/client/claude/server.go`
- Modify: `internal/client/openai/server_test.go`

- [ ] Write a test that `GET /v1/models` requires auth.
- [ ] Write a test that `GET /v1/models` returns `object:"list"` and enabled route/model aliases.
- [ ] Run targeted tests and verify failure.
- [ ] Implement `openai.NewServer(deps)` with routes for `/v1/models`.
- [ ] Mount OpenAI routes from the existing server composition.
- [ ] Implement model list response.
- [ ] Run `go test -count=1 ./internal/client/openai ./internal/client/claude`.
- [ ] Commit with `feat: add openai models endpoint`.

### Task 4: Non-Streaming Chat Completions

**Files:**

- Create: `internal/client/openai/chat.go`
- Modify: `internal/client/openai/server.go`
- Modify: `internal/client/openai/server_test.go`

- [ ] Write an integration test with an OpenAI-compatible fake upstream returning `chat.completion`.
- [ ] Assert Arkroute returns `object:"chat.completion"`, `choices[0].message.content`, `finish_reason`, and `usage`.
- [ ] Assert upstream receives normalized OpenAI-compatible provider request through the existing provider adapter.
- [ ] Run targeted test and verify failure.
- [ ] Implement handler decode, normalize, `gen.Execute`, and response mapping.
- [ ] Run `go test -count=1 ./internal/client/openai ./internal/client/claude`.
- [ ] Commit with `feat: add openai chat completions endpoint`.

### Task 5: Streaming Chat Completions

**Files:**

- Create: `internal/client/openai/stream.go`
- Modify: `internal/client/openai/chat.go`
- Modify: `internal/client/openai/server_test.go`

- [ ] Write a streaming integration test with fake upstream SSE.
- [ ] Assert response uses `Content-Type: text/event-stream`.
- [ ] Assert chunks include `object:"chat.completion.chunk"` and final `data: [DONE]`.
- [ ] Write direct unit tests for text deltas, tool call start/delta, and error events.
- [ ] Run targeted tests and verify failure.
- [ ] Implement `writeChatCompletionStream`.
- [ ] Run `go test -count=1 ./internal/client/openai ./internal/client/claude`.
- [ ] Commit with `feat: add openai chat streaming`.

### Task 6: Phase 4A Docs And Verification

**Files:**

- Modify: `README.md`

- [ ] Add OpenAI Chat setup docs with `OPENAI_BASE_URL` and `OPENAI_API_KEY`.
- [ ] Add supported and unsupported Chat fields.
- [ ] Run `go test -count=1 ./...`.
- [ ] Run `npm test --prefix npm/arkroute`.
- [ ] Commit with `docs: document openai chat compatibility`.

## Phase 4B: Responses API Core

### File Structure

Create:

- `internal/protocol/openai/responses_codec.go`: Responses request decode and normalize.
- `internal/protocol/openai/responses_codec_test.go`: Responses codec tests.
- `internal/client/openai/responses.go`: Responses handler.
- `internal/client/openai/responses_stream.go`: Responses SSE writer.
- `internal/client/openai/responses_test.go`: Responses HTTP integration tests.

Modify:

- `internal/client/openai/server.go`: mount `/v1/responses`.
- `README.md`: document Responses subset.

### Acceptance Criteria

- `POST /v1/responses` works for string input.
- `POST /v1/responses` works for message item input.
- Streaming text emits Responses-style events.
- Function tools map to normalized tools.
- Hosted-only features return stable unsupported errors.

### Task 1: Responses Codec

**Files:**

- Create: `internal/protocol/openai/responses_codec.go`
- Create: `internal/protocol/openai/responses_codec_test.go`

- [ ] Write tests for string input, input message arrays, `instructions`, function tools, `max_output_tokens`, and `stream`.
- [ ] Write tests that `previous_response_id`, `store:true`, and hosted tools return unsupported-feature errors.
- [ ] Run `go test -count=1 ./internal/protocol/openai` and verify failure.
- [ ] Implement `DecodeResponsesRequest`.
- [ ] Implement `NormalizeResponsesRequest`.
- [ ] Run `gofmt -w internal/protocol/openai/responses_codec.go internal/protocol/openai/responses_codec_test.go`.
- [ ] Run `go test -count=1 ./internal/protocol/openai`.
- [ ] Commit with `feat: add responses request codec`.

### Task 2: Non-Streaming Responses

**Files:**

- Create: `internal/client/openai/responses.go`
- Create: `internal/client/openai/responses_test.go`
- Modify: `internal/client/openai/server.go`

- [ ] Write integration tests for `POST /v1/responses` string input.
- [ ] Assert response includes `id`, `object:"response"`, `model`, `output`, `output_text`, and `usage`.
- [ ] Write integration test for function-call output.
- [ ] Run targeted tests and verify failure.
- [ ] Implement non-streaming Responses handler using `gen.Execute`.
- [ ] Run `go test -count=1 ./internal/client/openai ./internal/client/claude`.
- [ ] Commit with `feat: add responses endpoint`.

### Task 3: Streaming Responses

**Files:**

- Create: `internal/client/openai/responses_stream.go`
- Modify: `internal/client/openai/responses.go`
- Modify: `internal/client/openai/responses_test.go`

- [ ] Write streaming tests for text deltas and final completion.
- [ ] Write streaming tests for error event shape.
- [ ] Run targeted tests and verify failure.
- [ ] Implement Responses SSE event writer from `protocol.StreamEvent`.
- [ ] Run `go test -count=1 ./internal/client/openai ./internal/client/claude`.
- [ ] Commit with `feat: add responses streaming`.

### Task 4: Phase 4B Docs And Verification

**Files:**

- Modify: `README.md`

- [ ] Add Responses subset docs.
- [ ] Add unsupported feature table.
- [ ] Run `go test -count=1 ./...`.
- [ ] Run `npm test --prefix npm/arkroute`.
- [ ] Commit with `docs: document responses compatibility subset`.

## Phase 4C: Compatibility Hardening

### File Structure

Create:

- `internal/client/openai/fixtures_test.go`: client payload fixture tests.
- `docs/openai-compatibility.md`: endpoint and client compatibility matrix.

Modify:

- `README.md`
- `internal/app/commands.go` or `internal/cli/cli.go` only if adding an OpenAI doctor command.

### Tasks

- [ ] Add fixture tests for Cursor/OpenCode/Cline/Continue-style chat payloads.
- [ ] Add fixture tests for OpenAI SDK Chat and Responses payloads.
- [ ] Add tolerant parsing for harmless unsupported fields.
- [ ] Add explicit unsupported errors for dangerous unsupported fields.
- [ ] Add docs for generic OpenAI-compatible client setup.
- [ ] Run `go test -count=1 ./...`.
- [ ] Run `npm test --prefix npm/arkroute`.
- [ ] Commit with `test: harden openai client compatibility`.

## Phase 5A: Embeddings And Legacy Completions

### File Structure

Create:

- `internal/protocol/embedding/types.go`
- `internal/client/openai/embeddings.go`
- `internal/client/openai/completions.go`
- `internal/adapter/openai/embeddings.go`

Modify:

- `internal/adapter/adapter.go`: add optional embedding adapter interface.
- `internal/adapter/builtin/registry.go`: preserve existing chat adapter behavior.
- `internal/config/types.go`: add embedding capability fields only if required.

### Tasks

- [ ] Add normalized embedding request/response types.
- [ ] Add OpenAI-compatible embeddings provider adapter.
- [ ] Add `POST /v1/embeddings` handler.
- [ ] Add `POST /v1/completions` legacy prompt adapter.
- [ ] Add unsupported-provider tests.
- [ ] Run full verification and commit.

## Phase 5B: Moderations And Rerank

### Tasks

- [ ] Add normalized moderation request/response types.
- [ ] Add `POST /v1/moderations`.
- [ ] Add provider adapter support where available.
- [ ] Add `POST /v1/rerank` as an Arkroute extension with clear docs.
- [ ] Run full verification and commit.

## Phase 6A: Files API

### File Structure

Create:

- `internal/storage/files/store.go`
- `internal/storage/files/store_test.go`
- `internal/client/openai/files.go`
- `internal/client/openai/files_test.go`

### Tasks

- [ ] Add local file metadata model.
- [ ] Add safe filesystem storage under Arkroute data dir.
- [ ] Add upload/list/retrieve/delete/content endpoints.
- [ ] Add size/path/content safety tests.
- [ ] Run full verification and commit.

## Phase 6B: Batch API

### File Structure

Create:

- `internal/storage/batches/store.go`
- `internal/storage/batches/worker.go`
- `internal/client/openai/batches.go`

### Tasks

- [ ] Add local batch metadata and lifecycle store.
- [ ] Parse input JSONL from Files API.
- [ ] Execute supported endpoint requests with bounded concurrency.
- [ ] Write output/error files through Files API storage.
- [ ] Add cancel/list/retrieve tests.
- [ ] Run full verification and commit.

## Phase 6C: Response State Store

### Tasks

- [ ] Add local response store.
- [ ] Persist Responses input/output when `store:true`.
- [ ] Implement `GET /v1/responses/{id}`.
- [ ] Implement `DELETE /v1/responses/{id}`.
- [ ] Implement `GET /v1/responses/{id}/input_items`.
- [ ] Implement `previous_response_id` for stored local responses.
- [ ] Run full verification and commit.

## Phase 7A: Vector Stores And File Search

### Tasks

- [ ] Add local vector store metadata.
- [ ] Add file chunking.
- [ ] Use Phase 5A embeddings to index chunks.
- [ ] Implement vector store CRUD endpoints.
- [ ] Implement vector store file and file batch endpoints.
- [ ] Implement vector store search endpoint.
- [ ] Enable Responses file-search tool when vector stores are configured.
- [ ] Run full verification and commit.

## Phase 7B: Image And Vision Platform APIs

### Tasks

- [ ] Add normalized image generation/edit types.
- [ ] Add OpenAI image endpoint handlers.
- [ ] Add provider adapter interfaces for image generation/edit.
- [ ] Add tests with fake image provider.
- [ ] Add docs for provider capability requirements.
- [ ] Run full verification and commit.

## Phase 7C: Audio APIs

### Tasks

- [ ] Add normalized transcription, translation, and speech request types.
- [ ] Add multipart parsing for audio input.
- [ ] Add provider adapter interfaces for STT/TTS.
- [ ] Add OpenAI audio endpoints.
- [ ] Add tests with fake audio providers.
- [ ] Run full verification and commit.

## Phase 8A: Realtime Compatibility

### Tasks

- [ ] Define local realtime session model.
- [ ] Add realtime session endpoint subset.
- [ ] Add WebSocket handler.
- [ ] Map text-first realtime events to runtime streaming.
- [ ] Add cancellation/backpressure tests.
- [ ] Run full verification and commit.

## Phase 8B: OpenAI SDK And CLI Compatibility Matrix

### Tasks

- [ ] Create `docs/openai-compatibility.md`.
- [ ] Add endpoint coverage table.
- [ ] Add setup docs for generic SDK, Cursor, OpenCode, Cline, Continue, and Codex-style clients.
- [ ] Add fixture tests for documented clients.
- [ ] Extend `arkroute doctor` with OpenAI base URL guidance if useful.
- [ ] Run full verification and commit.

## Phase 8C: Usage, Quotas, And Routing Intelligence

### Tasks

- [ ] Add request usage counters outside trace logs.
- [ ] Add local per-client key model if needed.
- [ ] Add optional token/request budgets.
- [ ] Add circuit breaker and cooldown health policy.
- [ ] Add latency-aware and health-aware router policies.
- [ ] Add cost-aware policy after pricing metadata exists.
- [ ] Add dashboard/admin API fields for usage and policy state.
- [ ] Run full verification and commit.

## Full Track Completion Checklist

- [ ] `GET /v1/models` implemented.
- [ ] `POST /v1/chat/completions` implemented with stream and tools.
- [ ] `POST /v1/responses` implemented with stream, tools, and stored state.
- [ ] `POST /v1/embeddings` implemented for compatible providers.
- [ ] `POST /v1/completions` implemented or clearly documented as legacy adapter.
- [ ] `POST /v1/moderations` implemented where provider-backed.
- [ ] Files API implemented locally.
- [ ] Batch API implemented locally.
- [ ] Vector Stores and file search implemented locally.
- [ ] Image APIs implemented where provider-backed.
- [ ] Audio APIs implemented where provider-backed.
- [ ] Realtime subset implemented.
- [ ] Unsupported hosted-only features return stable OpenAI-style errors.
- [ ] README and compatibility matrix document endpoint status.
- [ ] `go test -count=1 ./...` passes.
- [ ] `npm test --prefix npm/arkroute` passes.
- [ ] Existing Claude Code flow remains compatible.
