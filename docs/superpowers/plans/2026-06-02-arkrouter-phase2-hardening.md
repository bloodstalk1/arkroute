# Arkrouter Phase 2 Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor Arkrouter Phase 1 into a durable runtime foundation with route plans, policy boundaries, adapter registry, trace lifecycle events, read-only admin APIs, improved operator CLI, graceful server lifecycle, and release metadata.

**Architecture:** Keep Claude Code as the only client protocol in Phase 2, but move provider execution out of `client/claude` into `internal/runtime`. Router builds `RoutePlan`, policies order targets, adapters map provider protocols, runtime owns upstream HTTP calls/fallback/health/trace, and admin/CLI consume read-only runtime state.

**Tech Stack:** Go 1.23+, standard `net/http`, `httptest`, JSONL logs, `gopkg.in/yaml.v3`, Makefile build metadata via `-ldflags`.

---

## Source Spec

Implement from `docs/superpowers/specs/2026-06-02-arkrouter-phase2-hardening-design.md`.

Do not add OpenAI-compatible ingress, dashboard, token compression, cloud sync, team mode, OAuth, account pooling, SQLite, or advanced routing strategies in Phase 2.

## File Structure

Create or modify these focused units:

- `internal/runtime/executor.go`: normalized request execution, non-streaming and streaming entrypoints.
- `internal/runtime/attempt.go`: execution attempt/result types.
- `internal/failure/errors.go`: structured error classes shared by runtime, adapters, CLI, and tests.
- `internal/runtime/executor_test.go`: non-streaming runtime success, fallback, no-fallback, trace, health.
- `internal/adapter/adapter.go`: registry, stream mapper, error classification interfaces.
- `internal/adapter/builtin/registry.go`: default provider registry without creating adapter import cycles.
- `internal/adapter/builtin/registry_test.go`: registry behavior.
- `internal/adapter/openai/stream.go`: conform OpenAI stream mapper to shared interface.
- `internal/adapter/openai/mapper.go`: implement provider error classification.
- `internal/adapter/gemini/mapper.go`: implement stream mapper capability/error classification.
- `internal/adapter/anthropic/passthrough.go`: implement stream mapper capability/error classification.
- `internal/router/router.go`: add `RoutePlan` and `Plan`.
- `internal/router/policy.go`: policy interface and priority/fallback target ordering.
- `internal/router/router_test.go`: route plan and policy tests.
- `internal/router/health.go`: richer health state with sanitized error/latency.
- `internal/observability/trace.go`: schema version, event names, trace sink interface, JSONL sink, counters.
- `internal/observability/trace_test.go`: concurrency/redaction/counter tests.
- `internal/failure/errors_test.go`: error classification and retryability tests.
- `internal/client/claude/server.go`: deps switch from router/adapters to runtime executor plus admin deps.
- `internal/client/claude/messages.go`: remove upstream execution and adapter selection.
- `internal/client/claude/stream.go`: encode normalized stream events only.
- `internal/client/claude/admin.go`: read-only internal admin endpoints.
- `internal/client/claude/server_test.go`: compatibility tests plus admin endpoint tests.
- `internal/app/serve.go`: build runtime executor, shared HTTP client, trace sink, graceful shutdown.
- `internal/app/commands.go`: config/provider/model/route list, status/admin fallback, logs tail, doctor checks.
- `internal/app/store.go`: config store abstraction.
- `internal/config/migrate.go`: version 1 migration foundation.
- `internal/config/redact.go`: redacted config view.
- `internal/buildinfo/buildinfo.go`: version/commit/date/debug info.
- `internal/cli/cli.go`: subcommands for `config`, `provider`, `model`, `route`, `version --debug`.
- `internal/cli/cli_test.go`: command tests.
- `Makefile`: `test`, `build`, `install`, `clean`.
- `README.md`: troubleshooting, make commands, phase 2 operator usage.

## Implementation Rules

- Preserve all Phase 1 user-facing behavior and tests.
- Do not weaken existing test assertions to make refactors pass.
- Move behavior behind new tests before deleting old code paths.
- Client handlers must not select provider adapters or execute upstream HTTP requests after Task 6.
- Runtime must not write directly to `http.ResponseWriter`.
- Provider adapters must not mutate health or write trace logs.
- All new JSON admin responses include `schema_version: 1`.
- Redact upstream API keys and local client key.
- Run `gofmt -w .` and `go test -count=1 ./...` before each commit. If sandbox blocks local ports, rerun the same command with appropriate permission.

---

### Task 1: Error Classes, Trace Sink, And Build Info Foundations

**Files:**
- Modify: `internal/observability/trace.go`
- Modify: `internal/observability/trace_test.go`
- Create: `internal/failure/errors.go`
- Create: `internal/failure/errors_test.go`
- Create: `internal/buildinfo/buildinfo.go`
- Create: `internal/buildinfo/buildinfo_test.go`

- [ ] **Step 1: Add failing tests for trace schema/counters**

Replace `internal/observability/trace_test.go` with:

```go
package observability

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteTraceRedactsSecretsAndIncludesSchema(t *testing.T) {
	var buf bytes.Buffer
	sink := NewJSONLSink(&buf)
	sink.Emit(TraceEvent{
		Time:      time.Unix(0, 0).UTC(),
		Event:     EventRequestStarted,
		RequestID: "req_1",
		Route:     "sonnet",
		Provider:  "openrouter",
		Status:    200,
		Headers:   map[string]string{"Authorization": "Bearer secret", "X-OpenRouter-Title": "Arkrouter"},
	})

	out := buf.String()
	if strings.Contains(out, "Bearer secret") {
		t.Fatalf("trace leaked secret: %s", out)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &decoded); err != nil {
		t.Fatalf("trace is not json: %v", err)
	}
	if decoded["schema_version"].(float64) != 1 {
		t.Fatalf("schema_version = %v, want 1", decoded["schema_version"])
	}
	if decoded["event"] != string(EventRequestStarted) {
		t.Fatalf("event = %v", decoded["event"])
	}
	if sink.Stats().Emitted != 1 {
		t.Fatalf("emitted = %d, want 1", sink.Stats().Emitted)
	}
}

func TestJSONLSinkConcurrentEmit(t *testing.T) {
	var buf bytes.Buffer
	sink := NewJSONLSink(&buf)
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sink.Emit(TraceEvent{Time: time.Unix(0, 0).UTC(), Event: EventRequestStarted, RequestID: "req"})
		}()
	}
	wg.Wait()
	if sink.Stats().Emitted != 25 {
		t.Fatalf("emitted = %d, want 25", sink.Stats().Emitted)
	}
}

func TestNoopSinkCountsDropped(t *testing.T) {
	sink := NewNoopSink()
	sink.Emit(TraceEvent{Event: EventRequestStarted})
	if sink.Stats().Dropped != 1 {
		t.Fatalf("dropped = %d, want 1", sink.Stats().Dropped)
	}
}
```

- [ ] **Step 2: Add failing tests for shared error classes**

Create `internal/failure/errors_test.go`:

```go
package failure

import "testing"

func TestClassifyStatus(t *testing.T) {
	tests := map[int]ErrorClass{
		400: ErrorInvalidRequest,
		401: ErrorUpstreamAuth,
		403: ErrorUpstreamAuth,
		408: ErrorUpstreamRetryable,
		429: ErrorUpstreamRateLimit,
		500: ErrorUpstreamRetryable,
		502: ErrorUpstreamRetryable,
		503: ErrorUpstreamRetryable,
		504: ErrorUpstreamRetryable,
	}
	for status, want := range tests {
		if got := ClassifyStatus(status); got != want {
			t.Fatalf("ClassifyStatus(%d) = %s, want %s", status, got, want)
		}
	}
}

func TestErrorClassRetryable(t *testing.T) {
	for _, class := range []ErrorClass{ErrorUpstreamRateLimit, ErrorUpstreamRetryable, ErrorUpstreamTimeout} {
		if !class.Retryable() {
			t.Fatalf("%s should be retryable", class)
		}
	}
	for _, class := range []ErrorClass{ErrorUpstreamAuth, ErrorInvalidRequest, ErrorUnsupportedCapability} {
		if class.Retryable() {
			t.Fatalf("%s should not be retryable", class)
		}
	}
}
```

- [ ] **Step 3: Add failing tests for build info**

Create `internal/buildinfo/buildinfo_test.go`:

```go
package buildinfo

import "strings"
import "testing"

func TestSummary(t *testing.T) {
	got := Summary()
	if !strings.Contains(got, "arkrouter") {
		t.Fatalf("Summary() = %q", got)
	}
}

func TestDebug(t *testing.T) {
	got := Debug()
	for _, want := range []string{"version:", "commit:", "build_date:", "go:", "os_arch:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Debug() missing %s: %q", want, got)
		}
	}
}
```

- [ ] **Step 4: Run tests and verify failure**

Run:

```sh
go test ./internal/observability ./internal/failure ./internal/buildinfo
```

Expected: fails because the new APIs do not exist.

- [ ] **Step 5: Implement trace sink with schema/counters**

Replace `internal/observability/trace.go` with:

```go
package observability

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"bat.dev/arkrouter/internal/security"
)

const SchemaVersion = 1

type EventName string

const (
	EventRequestStarted        EventName = "request_started"
	EventRoutePlanned          EventName = "route_planned"
	EventTargetSelected        EventName = "target_selected"
	EventUpstreamRequestStarted EventName = "upstream_request_started"
	EventUpstreamResponse      EventName = "upstream_response"
	EventFallback              EventName = "fallback"
	EventStreamStarted         EventName = "stream_started"
	EventRequestFinished       EventName = "request_finished"
	EventRequestFailed         EventName = "request_failed"
)

type TraceEvent struct {
	SchemaVersion int               `json:"schema_version"`
	Time          time.Time         `json:"time"`
	Event         EventName         `json:"event"`
	RequestID     string            `json:"request_id"`
	Client        string            `json:"client,omitempty"`
	Route         string            `json:"route,omitempty"`
	Strategy      string            `json:"strategy,omitempty"`
	Provider      string            `json:"provider,omitempty"`
	ProviderType  string            `json:"provider_type,omitempty"`
	Model         string            `json:"model,omitempty"`
	UpstreamModel string            `json:"upstream_model,omitempty"`
	Status        int               `json:"status,omitempty"`
	LatencyMS     int64             `json:"latency_ms,omitempty"`
	Retryable     bool              `json:"retryable,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	ErrorClass    string            `json:"error_class,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type Stats struct {
	Emitted     int64 `json:"emitted"`
	FailedWrites int64 `json:"failed_writes"`
	Dropped     int64 `json:"dropped"`
}

type TraceSink interface {
	Emit(event TraceEvent)
	Stats() Stats
}

type JSONLSink struct {
	mu    sync.Mutex
	w     io.Writer
	stats Stats
}

func NewJSONLSink(w io.Writer) *JSONLSink {
	return &JSONLSink{w: w}
}

func (s *JSONLSink) Emit(event TraceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.SchemaVersion = SchemaVersion
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.Headers != nil {
		event.Headers = security.RedactMap(event.Headers)
	}
	data, err := json.Marshal(event)
	if err != nil {
		s.stats.FailedWrites++
		return
	}
	if _, err := s.w.Write(append(data, '\n')); err != nil {
		s.stats.FailedWrites++
		return
	}
	s.stats.Emitted++
}

func (s *JSONLSink) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

type NoopSink struct {
	mu    sync.Mutex
	stats Stats
}

func NewNoopSink() *NoopSink {
	return &NoopSink{}
}

func (s *NoopSink) Emit(TraceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Dropped++
}

func (s *NoopSink) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}
```

- [ ] **Step 6: Implement shared error classes**

Create `internal/failure/errors.go`:

```go
package failure

type ErrorClass string

const (
	ErrorInvalidRequest         ErrorClass = "invalid_request"
	ErrorRouteNotFound         ErrorClass = "route_not_found"
	ErrorUnsupportedCapability ErrorClass = "unsupported_capability"
	ErrorGatewayAuth           ErrorClass = "gateway_auth"
	ErrorUpstreamAuth          ErrorClass = "upstream_auth"
	ErrorUpstreamRateLimit     ErrorClass = "upstream_rate_limit"
	ErrorUpstreamTimeout       ErrorClass = "upstream_timeout"
	ErrorUpstreamRetryable     ErrorClass = "upstream_retryable"
	ErrorUpstreamFatal         ErrorClass = "upstream_fatal"
	ErrorStream                ErrorClass = "stream_error"
)

func ClassifyStatus(status int) ErrorClass {
	switch status {
	case 400:
		return ErrorInvalidRequest
	case 401, 403:
		return ErrorUpstreamAuth
	case 408, 500, 502, 503, 504:
		return ErrorUpstreamRetryable
	case 429:
		return ErrorUpstreamRateLimit
	default:
		if status >= 500 {
			return ErrorUpstreamRetryable
		}
		return ErrorUpstreamFatal
	}
}

func (c ErrorClass) Retryable() bool {
	switch c {
	case ErrorUpstreamRateLimit, ErrorUpstreamRetryable, ErrorUpstreamTimeout:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 7: Implement build info**

Create `internal/buildinfo/buildinfo.go`:

```go
package buildinfo

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func Summary() string {
	return "arkrouter " + Version
}

func Debug() string {
	return fmt.Sprintf("version: %s\ncommit: %s\nbuild_date: %s\ngo: %s\nos_arch: %s/%s\n", Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
```

- [ ] **Step 8: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/observability internal/failure internal/buildinfo
git commit -m "feat: add phase 2 trace and error foundations"
```

---

### Task 2: Adapter Registry, Stream Mapper Contract, And Error Classification

**Files:**
- Modify: `internal/adapter/adapter.go`
- Create: `internal/adapter/builtin/registry.go`
- Create: `internal/adapter/builtin/registry_test.go`
- Modify: `internal/adapter/openai/mapper.go`
- Modify: `internal/adapter/openai/stream.go`
- Modify: `internal/adapter/openai/openai_test.go`
- Modify: `internal/adapter/gemini/mapper.go`
- Modify: `internal/adapter/gemini/gemini_test.go`
- Modify: `internal/adapter/anthropic/passthrough.go`
- Modify: `internal/adapter/anthropic/passthrough_test.go`

- [ ] **Step 1: Add failing registry tests**

Create `internal/adapter/builtin/registry_test.go`:

```go
package builtin

import (
	"testing"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
)

func TestDefaultRegistryHasBuiltIns(t *testing.T) {
	registry := DefaultRegistry()
	for _, providerType := range []string{"openai_compatible", "gemini", "anthropic"} {
		if _, ok := registry.Get(providerType); !ok {
			t.Fatalf("DefaultRegistry missing %s", providerType)
		}
	}
}

func TestMapRegistry(t *testing.T) {
	registry := adapter.MapRegistry{"fake": fakeAdapterForTest{}}
	if _, ok := registry.Get("fake"); !ok {
		t.Fatal("fake adapter missing")
	}
	if _, ok := registry.Get("missing"); ok {
		t.Fatal("missing adapter unexpectedly present")
	}
}

type fakeAdapterForTest struct{}

func (fakeAdapterForTest) BuildRequest(protocol.Request, config.ProviderConfig, config.ModelConfig) (adapter.UpstreamRequest, error) {
	return adapter.UpstreamRequest{}, nil
}
func (fakeAdapterForTest) MapResponse([]byte) (protocol.Response, error) { return protocol.Response{}, nil }
func (fakeAdapterForTest) NewStreamMapper() (adapter.StreamMapper, bool) { return nil, false }
func (fakeAdapterForTest) ClassifyError(status int, body []byte) failure.ErrorClass { return failure.ErrorUpstreamFatal }
```

- [ ] **Step 2: Add failing adapter contract tests**

Append to `internal/adapter/openai/openai_test.go`:

```go
func TestOpenAIAdapterSupportsStreamMapper(t *testing.T) {
	adapter := Adapter{}
	mapper, ok := adapter.NewStreamMapper()
	if !ok {
		t.Fatal("OpenAI adapter should support stream mapper")
	}
	events, err := mapper.MapLine([]byte("data: [DONE]"))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "message_stop" {
		t.Fatalf("events = %+v", events)
	}
}

func TestOpenAIClassifyError(t *testing.T) {
	adapter := Adapter{}
	if got := adapter.ClassifyError(429, nil); got != "upstream_rate_limit" {
		t.Fatalf("ClassifyError(429) = %s", got)
	}
	if got := adapter.ClassifyError(401, nil); got != "upstream_auth" {
		t.Fatalf("ClassifyError(401) = %s", got)
	}
}
```

Append to `internal/adapter/gemini/gemini_test.go`:

```go
func TestGeminiAdapterDoesNotAdvertiseUnimplementedStreamMapper(t *testing.T) {
	adapter := Adapter{}
	if _, ok := adapter.NewStreamMapper(); ok {
		t.Fatal("Gemini adapter should not advertise streaming until its parser maps deltas")
	}
}
```

Append to `internal/adapter/anthropic/passthrough_test.go`:

```go
func TestAnthropicAdapterDoesNotAdvertiseUnimplementedStreamMapper(t *testing.T) {
	adapter := Adapter{}
	if _, ok := adapter.NewStreamMapper(); ok {
		t.Fatal("Anthropic adapter should not advertise streaming until its parser maps deltas")
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```sh
go test ./internal/adapter/...
```

Expected: fails because registry, `NewStreamMapper`, and `ClassifyError` are missing.

- [ ] **Step 4: Extend adapter interfaces**

Modify `internal/adapter/adapter.go`:

```go
package adapter

import (
	"net/http"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
)

type UpstreamRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

type StreamMapper interface {
	MapLine(line []byte) ([]protocol.StreamEvent, error)
}

type ProviderAdapter interface {
	BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (UpstreamRequest, error)
	MapResponse(body []byte) (protocol.Response, error)
	NewStreamMapper() (StreamMapper, bool)
	ClassifyError(status int, body []byte) failure.ErrorClass
}

type Registry interface {
	Get(providerType string) (ProviderAdapter, bool)
}

type MapRegistry map[string]ProviderAdapter

func (r MapRegistry) Get(providerType string) (ProviderAdapter, bool) {
	providerAdapter, ok := r[providerType]
	return providerAdapter, ok
}
```

- [ ] **Step 5: Add registry implementation**

Create `internal/adapter/builtin/registry.go`:

```go
package builtin

import (
	"bat.dev/arkrouter/internal/adapter"
	anthropicadapter "bat.dev/arkrouter/internal/adapter/anthropic"
	geminiadapter "bat.dev/arkrouter/internal/adapter/gemini"
	openaiadapter "bat.dev/arkrouter/internal/adapter/openai"
)

func DefaultRegistry() adapter.Registry {
	return adapter.MapRegistry{
		"openai_compatible": openaiadapter.Adapter{},
		"gemini":            geminiadapter.Adapter{},
		"anthropic":         anthropicadapter.Adapter{},
	}
}
```

- [ ] **Step 6: Implement adapter contract methods**

Add to `internal/adapter/openai/mapper.go`:

```go
func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return NewStreamMapper(), true
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
```

Add import alias:

```go
"bat.dev/arkrouter/internal/failure"
```

Add to `internal/adapter/gemini/mapper.go`:

```go
func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return nil, false
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
```

Ensure `internal/adapter/gemini/mapper.go` imports `bat.dev/arkrouter/internal/failure`.

Add to `internal/adapter/anthropic/passthrough.go`:

```go
func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return nil, false
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
```

Ensure `internal/adapter/anthropic/passthrough.go` imports `bat.dev/arkrouter/internal/failure`.

- [ ] **Step 7: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/adapter
git commit -m "feat: add adapter registry and stream contract"
```

---

### Task 3: RoutePlan, Policy Interface, And Richer Health

**Files:**
- Modify: `internal/router/router.go`
- Create: `internal/router/policy.go`
- Modify: `internal/router/health.go`
- Modify: `internal/router/router_test.go`

- [ ] **Step 1: Add failing route plan and policy tests**

Append to `internal/router/router_test.go`:

```go
func TestPlanReturnsRoutePlan(t *testing.T) {
	snapshot, err := config.BuildSnapshot(config.MinimalValidConfig("local-key"))
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	plan, err := New(snapshot, NewHealthStore()).Plan("sonnet", Requirements{Streaming: true, Tools: true})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Alias != "sonnet" || plan.Strategy != "fallback" || len(plan.Targets) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestFallbackPolicyReturnsAllTargets(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers = append(cfg.Providers, cfg.Providers[0])
	cfg.Providers[1].ID = "backup"
	cfg.Models = append(cfg.Models, cfg.Models[0])
	cfg.Models[1].ID = "backup-model"
	cfg.Models[1].ProviderID = "backup"
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}, {ModelID: "backup-model", Enabled: true}}
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	plan, err := New(snapshot, NewHealthStore()).Plan("sonnet", Requirements{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	targets, reason := FallbackPolicy{}.Select(plan, NewHealthStore().Snapshot())
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	if reason == "" {
		t.Fatal("reason is empty")
	}
}

func TestHealthStoreStoresSanitizedDetails(t *testing.T) {
	store := NewHealthStore()
	store.Update(Update{
		ProviderID:   "openrouter",
		UpstreamModel: "model",
		Status:       "degraded",
		StatusCode:   429,
		ErrorClass:   "upstream_rate_limit",
		ErrorMessage: strings.Repeat("x", 300),
		Latency:      time.Second,
	})
	health := store.Snapshot()["openrouter"]
	if health.Status != "degraded" || health.LastStatusCode != 429 {
		t.Fatalf("health = %+v", health)
	}
	if len(health.LastErrorMessage) > 160 {
		t.Fatalf("error message not limited: %d", len(health.LastErrorMessage))
	}
}
```

Add imports to `internal/router/router_test.go`:

```go
import (
	"strings"
	"time"
)
```

- [ ] **Step 2: Run router tests and verify failure**

Run:

```sh
go test ./internal/router
```

Expected: fails because `Plan`, `FallbackPolicy`, `Update`, and richer health fields are missing.

- [ ] **Step 3: Add RoutePlan and Plan**

Modify `internal/router/router.go`:

```go
type RoutePlan struct {
	Alias        string
	Strategy     string
	Requirements Requirements
	Targets      []Target
}

func (r *Router) Plan(alias string, req Requirements) (RoutePlan, error) {
	targets, err := r.Resolve(alias, req)
	if err != nil {
		return RoutePlan{}, err
	}
	strategy := "priority"
	resolvedAlias := alias
	if len(targets) > 0 && targets[0].Route.Alias != "" {
		strategy = targets[0].Route.Strategy
		resolvedAlias = targets[0].Route.Alias
	}
	return RoutePlan{Alias: resolvedAlias, Strategy: strategy, Requirements: req, Targets: targets}, nil
}
```

Keep `Resolve` for compatibility during migration. Later runtime should call `Plan`.

- [ ] **Step 4: Add policy interface**

Create `internal/router/policy.go`:

```go
package router

type Policy interface {
	Select(plan RoutePlan, health map[string]Health) ([]Target, string)
}

type PriorityPolicy struct{}

func (PriorityPolicy) Select(plan RoutePlan, health map[string]Health) ([]Target, string) {
	if len(plan.Targets) == 0 {
		return nil, "no_targets"
	}
	return plan.Targets[:1], "priority_first"
}

type FallbackPolicy struct{}

func (FallbackPolicy) Select(plan RoutePlan, health map[string]Health) ([]Target, string) {
	return append([]Target(nil), plan.Targets...), "fallback_order"
}

func PolicyFor(strategy string) Policy {
	if strategy == "fallback" {
		return FallbackPolicy{}
	}
	return PriorityPolicy{}
}
```

- [ ] **Step 5: Enrich health store**

Replace `internal/router/health.go`:

```go
package router

import (
	"strings"
	"sync"
	"time"
)

type Health struct {
	Status           string        `json:"status"`
	UpstreamModel    string        `json:"upstream_model,omitempty"`
	LastStatusCode   int           `json:"last_status_code,omitempty"`
	LastErrorClass   string        `json:"last_error_class,omitempty"`
	LastErrorMessage string        `json:"last_error_message,omitempty"`
	LastLatency      time.Duration `json:"last_latency,omitempty"`
	LastUpdated      time.Time     `json:"last_updated,omitempty"`
}

type Update struct {
	ProviderID    string
	UpstreamModel string
	Status        string
	StatusCode    int
	ErrorClass    string
	ErrorMessage  string
	Latency       time.Duration
}

type HealthStore struct {
	mu        sync.RWMutex
	upstreams map[string]Health
}

func NewHealthStore() *HealthStore {
	return &HealthStore{upstreams: map[string]Health{}}
}

func (s *HealthStore) Set(id string, status string) {
	s.Update(Update{ProviderID: id, Status: status})
}

func (s *HealthStore) Update(update Update) {
	if update.ProviderID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	message := sanitizeMessage(update.ErrorMessage)
	s.upstreams[update.ProviderID] = Health{
		Status:           update.Status,
		UpstreamModel:    update.UpstreamModel,
		LastStatusCode:   update.StatusCode,
		LastErrorClass:   update.ErrorClass,
		LastErrorMessage: message,
		LastLatency:      update.Latency,
		LastUpdated:      time.Now().UTC(),
	}
}

func (s *HealthStore) Snapshot() map[string]Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Health, len(s.upstreams))
	for id, health := range s.upstreams {
		out[id] = health
	}
	return out
}

func sanitizeMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 160 {
		return message[:160]
	}
	return message
}
```

- [ ] **Step 6: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/router
git commit -m "feat: add route plans policies and richer health"
```

---

### Task 4: Runtime Executor Non-Streaming

**Files:**
- Create: `internal/runtime/attempt.go`
- Create: `internal/runtime/executor.go`
- Create: `internal/runtime/executor_test.go`

- [ ] **Step 1: Write runtime executor tests**

Create `internal/runtime/executor_test.go`:

```go
package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bat.dev/arkrouter/internal/adapter/builtin"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/observability"
	"bat.dev/arkrouter/internal/protocol"
	"bat.dev/arkrouter/internal/router"
)

func TestExecutorNonStreamingSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	executor := testExecutor(t, upstream.URL)
	result, err := executor.Execute(context.Background(), ExecuteRequest{
		RequestID:    "req_1",
		Client:       "claude",
		Model:        "sonnet",
		Requirements: router.Requirements{},
		Request: protocol.Request{
			Model: "sonnet",
			Messages: []protocol.Message{{Role: protocol.RoleUser, Content: []protocol.ContentBlock{{Type: "text", Text: "hi"}}}},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Response.Content[0].Text != "hello" {
		t.Fatalf("response = %+v", result.Response)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].StatusCode != 200 {
		t.Fatalf("attempts = %+v", result.Attempts)
	}
}

func TestExecutorFallbackOnRetryableStatus(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"fallback"},"finish_reason":"stop"}]}`))
	}))
	defer second.Close()

	cfg := twoTargetConfig(first.URL, second.URL)
	executor := executorFromConfig(t, cfg, observability.NewNoopSink())
	result, err := executor.Execute(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Request: protocol.Request{Model: "sonnet"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Response.Content[0].Text != "fallback" {
		t.Fatalf("response = %+v", result.Response)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(result.Attempts))
	}
}

func TestExecutorDoesNotFallbackOnAuth(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("second target should not be called")
	}))
	defer second.Close()

	cfg := twoTargetConfig(first.URL, second.URL)
	executor := executorFromConfig(t, cfg, observability.NewNoopSink())
	_, err := executor.Execute(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Request: protocol.Request{Model: "sonnet"}})
	if err == nil {
		t.Fatal("Execute() error = nil, want auth error")
	}
	var execErr *ExecutionError
	if !AsExecutionError(err, &execErr) || execErr.Class != ErrorUpstreamAuth {
		t.Fatalf("error = %v, want upstream auth", err)
	}
}

func TestExecutorEmitsTraceAndHealth(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()
	sink := newMemorySink()
	executor := testExecutorWithSink(t, upstream.URL, sink)
	_, err := executor.Execute(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Request: protocol.Request{Model: "sonnet"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(sink.events) == 0 {
		t.Fatal("no trace events emitted")
	}
	health := executor.Health.Snapshot()["openrouter"]
	if health.Status != "ok" {
		t.Fatalf("health = %+v", health)
	}
}

func testExecutor(t *testing.T, upstreamURL string) *Executor {
	t.Helper()
	return testExecutorWithSink(t, upstreamURL, observability.NewNoopSink())
}

func testExecutorWithSink(t *testing.T, upstreamURL string, sink observability.TraceSink) *Executor {
	t.Helper()
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstreamURL + "/v1"
	cfg.Providers[0].APIKey = "sk-test"
	return executorFromConfig(t, cfg, sink)
}

func executorFromConfig(t *testing.T, cfg config.Config, sink observability.TraceSink) *Executor {
	t.Helper()
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	health := router.NewHealthStore()
	return NewExecutor(Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, health),
		Adapters: builtin.DefaultRegistry(),
		Health:   health,
		Trace:    sink,
		Client:   &http.Client{Timeout: time.Second},
	})
}

func twoTargetConfig(firstURL, secondURL string) config.Config {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers = append(cfg.Providers, cfg.Providers[0])
	cfg.Providers[0].ID = "first"
	cfg.Providers[0].BaseURL = firstURL + "/v1"
	cfg.Providers[0].APIKey = "sk-first"
	cfg.Providers[1].ID = "second"
	cfg.Providers[1].BaseURL = secondURL + "/v1"
	cfg.Providers[1].APIKey = "sk-second"
	cfg.Models = append(cfg.Models, cfg.Models[0])
	cfg.Models[0].ID = "first-model"
	cfg.Models[0].ProviderID = "first"
	cfg.Models[1].ID = "second-model"
	cfg.Models[1].ProviderID = "second"
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "first-model", Enabled: true}, {ModelID: "second-model", Enabled: true}}
	return cfg
}

type memorySink struct{ events []observability.TraceEvent }

func newMemorySink() *memorySink { return &memorySink{} }
func (s *memorySink) Emit(event observability.TraceEvent) { s.events = append(s.events, event) }
func (s *memorySink) Stats() observability.Stats { return observability.Stats{Emitted: int64(len(s.events))} }
```

- [ ] **Step 2: Run runtime tests and verify failure**

Run:

```sh
go test ./internal/runtime
```

Expected: fails because `Executor`, `Execute`, and helpers do not exist.

- [ ] **Step 3: Add attempt/result types**

Create `internal/runtime/attempt.go`:

```go
package runtime

import (
	"errors"
	"time"

	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
	"bat.dev/arkrouter/internal/router"
)

type ErrorClass = failure.ErrorClass

const (
	ErrorInvalidRequest         = failure.ErrorInvalidRequest
	ErrorRouteNotFound         = failure.ErrorRouteNotFound
	ErrorUnsupportedCapability = failure.ErrorUnsupportedCapability
	ErrorGatewayAuth           = failure.ErrorGatewayAuth
	ErrorUpstreamAuth          = failure.ErrorUpstreamAuth
	ErrorUpstreamRateLimit     = failure.ErrorUpstreamRateLimit
	ErrorUpstreamTimeout       = failure.ErrorUpstreamTimeout
	ErrorUpstreamRetryable     = failure.ErrorUpstreamRetryable
	ErrorUpstreamFatal         = failure.ErrorUpstreamFatal
	ErrorStream                = failure.ErrorStream
)

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
	ErrorClass   ErrorClass
	ErrorMessage string
}

type ExecutionError struct {
	Class    ErrorClass
	Message  string
	Attempts []Attempt
}

func (e *ExecutionError) Error() string {
	return e.Message
}

func AsExecutionError(err error, target **ExecutionError) bool {
	var execErr *ExecutionError
	if errors.As(err, &execErr) {
		*target = execErr
		return true
	}
	return false
}
```

- [ ] **Step 4: Implement non-streaming executor**

Create `internal/runtime/executor.go`:

```go
package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/adapter/builtin"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/observability"
	"bat.dev/arkrouter/internal/protocol"
	"bat.dev/arkrouter/internal/router"
)

type Deps struct {
	Snapshot config.Snapshot
	Router   *router.Router
	Adapters adapter.Registry
	Health   *router.HealthStore
	Trace    observability.TraceSink
	Client   *http.Client
}

type Executor struct {
	Snapshot config.Snapshot
	Router   *router.Router
	Adapters adapter.Registry
	Health   *router.HealthStore
	Trace    observability.TraceSink
	Client   *http.Client
}

func NewExecutor(deps Deps) *Executor {
	if deps.Health == nil {
		deps.Health = router.NewHealthStore()
	}
	if deps.Trace == nil {
		deps.Trace = observability.NewNoopSink()
	}
	if deps.Adapters == nil {
		deps.Adapters = builtin.DefaultRegistry()
	}
	if deps.Client == nil {
		deps.Client = &http.Client{Timeout: time.Duration(deps.Snapshot.Config.Server.UpstreamTimeoutSeconds) * time.Second}
	}
	if deps.Router == nil {
		deps.Router = router.New(deps.Snapshot, deps.Health)
	}
	return &Executor{Snapshot: deps.Snapshot, Router: deps.Router, Adapters: deps.Adapters, Health: deps.Health, Trace: deps.Trace, Client: deps.Client}
}

func (e *Executor) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	e.emit(req, observability.EventRequestStarted, observability.TraceEvent{})
	plan, err := e.Router.Plan(req.Model, req.Requirements)
	if err != nil {
		execErr := &ExecutionError{Class: ErrorRouteNotFound, Message: err.Error()}
		e.emit(req, observability.EventRequestFailed, observability.TraceEvent{ErrorClass: string(execErr.Class), Reason: execErr.Message})
		return ExecuteResult{}, execErr
	}
	e.emit(req, observability.EventRoutePlanned, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy})
	targets, reason := router.PolicyFor(plan.Strategy).Select(plan, e.Health.Snapshot())
	attempts := []Attempt{}
	for i, target := range targets {
		e.emit(req, observability.EventTargetSelected, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Reason: reason}))
		resp, attempt, err := e.executeTarget(ctx, req, target)
		attempts = append(attempts, attempt)
		if err == nil {
			e.Health.Update(router.Update{ProviderID: target.Provider.ID, UpstreamModel: target.Model.UpstreamModel, Status: "ok", StatusCode: attempt.StatusCode, Latency: attempt.Latency})
			e.emit(req, observability.EventRequestFinished, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: attempt.StatusCode, LatencyMS: attempt.Latency.Milliseconds()}))
			return ExecuteResult{Response: resp, Target: target, Attempts: attempts}, nil
		}
		e.Health.Update(router.Update{ProviderID: target.Provider.ID, UpstreamModel: target.Model.UpstreamModel, Status: statusForAttempt(attempt), StatusCode: attempt.StatusCode, ErrorClass: string(attempt.ErrorClass), ErrorMessage: attempt.ErrorMessage, Latency: attempt.Latency})
		if !attempt.Retryable || i == len(targets)-1 {
			execErr := &ExecutionError{Class: attempt.ErrorClass, Message: attempt.ErrorMessage, Attempts: attempts}
			e.emit(req, observability.EventRequestFailed, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, ErrorClass: string(execErr.Class), Reason: execErr.Message}))
			return ExecuteResult{}, execErr
		}
		e.emit(req, observability.EventFallback, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: attempt.StatusCode, Retryable: true, ErrorClass: string(attempt.ErrorClass), Reason: attempt.ErrorMessage}))
	}
	return ExecuteResult{}, &ExecutionError{Class: ErrorRouteNotFound, Message: "route has no targets"}
}

func (e *Executor) executeTarget(ctx context.Context, req ExecuteRequest, target router.Target) (protocol.Response, Attempt, error) {
	start := time.Now()
	providerAdapter, ok := e.Adapters.Get(target.Provider.Type)
	if !ok {
		attempt := Attempt{Target: target, ErrorClass: ErrorUpstreamFatal, ErrorMessage: "unsupported provider type " + target.Provider.Type}
		return protocol.Response{}, attempt, errors.New(attempt.ErrorMessage)
	}
	upstreamReq, err := providerAdapter.BuildRequest(req.Request, target.Provider, target.Model)
	if err != nil {
		attempt := Attempt{Target: target, ErrorClass: ErrorInvalidRequest, ErrorMessage: err.Error(), Latency: time.Since(start)}
		return protocol.Response{}, attempt, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
	if err != nil {
		attempt := Attempt{Target: target, ErrorClass: ErrorInvalidRequest, ErrorMessage: err.Error(), Latency: time.Since(start)}
		return protocol.Response{}, attempt, err
	}
	httpReq.Header = upstreamReq.Headers.Clone()
	e.emit(req, observability.EventUpstreamRequestStarted, traceForTarget(target, observability.TraceEvent{}))
	upstreamResp, err := e.Client.Do(httpReq)
	if err != nil {
		class := ErrorUpstreamFatal
		if ctx.Err() != nil {
			class = ErrorUpstreamTimeout
		}
		attempt := Attempt{Target: target, ErrorClass: class, ErrorMessage: err.Error(), Retryable: class.Retryable(), Latency: time.Since(start)}
		return protocol.Response{}, attempt, err
	}
	defer upstreamResp.Body.Close()
	body, _ := io.ReadAll(upstreamResp.Body)
	attempt := Attempt{Target: target, StatusCode: upstreamResp.StatusCode, Latency: time.Since(start)}
	if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
		class := providerAdapter.ClassifyError(upstreamResp.StatusCode, body)
		attempt.ErrorClass = class
		attempt.Retryable = class.Retryable()
		attempt.ErrorMessage = fmt.Sprintf("upstream returned %d", upstreamResp.StatusCode)
		e.emit(req, observability.EventUpstreamResponse, traceForTarget(target, observability.TraceEvent{Status: upstreamResp.StatusCode, LatencyMS: attempt.Latency.Milliseconds(), Retryable: attempt.Retryable, ErrorClass: string(class)}))
		return protocol.Response{}, attempt, errors.New(attempt.ErrorMessage)
	}
	e.emit(req, observability.EventUpstreamResponse, traceForTarget(target, observability.TraceEvent{Status: upstreamResp.StatusCode, LatencyMS: attempt.Latency.Milliseconds()}))
	resp, err := providerAdapter.MapResponse(body)
	if err != nil {
		attempt.ErrorClass = ErrorUpstreamFatal
		attempt.ErrorMessage = err.Error()
		attempt.Retryable = false
		return protocol.Response{}, attempt, err
	}
	return resp, attempt, nil
}

func (e *Executor) emit(req ExecuteRequest, event observability.EventName, base observability.TraceEvent) {
	base.Event = event
	base.RequestID = req.RequestID
	base.Client = req.Client
	if base.Model == "" {
		base.Model = req.Model
	}
	e.Trace.Emit(base)
}

func traceForTarget(target router.Target, event observability.TraceEvent) observability.TraceEvent {
	event.Provider = target.Provider.ID
	event.ProviderType = target.Provider.Type
	event.Model = target.Model.ID
	event.UpstreamModel = target.Model.UpstreamModel
	return event
}

func statusForAttempt(attempt Attempt) string {
	if attempt.Retryable {
		return "degraded"
	}
	return "unhealthy"
}
```

- [ ] **Step 5: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/runtime internal/adapter internal/router
git commit -m "feat: add runtime executor for non-streaming requests"
```

---

### Task 5: Runtime Streaming Execution

**Files:**
- Modify: `internal/runtime/attempt.go`
- Modify: `internal/runtime/executor.go`
- Create: `internal/runtime/stream_test.go`

- [ ] **Step 1: Add streaming runtime tests**

Create `internal/runtime/stream_test.go`:

```go
package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/observability"
	"bat.dev/arkrouter/internal/protocol"
	"bat.dev/arkrouter/internal/router"
)

func TestExecutorStreamSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"index\":0}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	executor := testExecutorWithSink(t, upstream.URL, observability.NewNoopSink())
	stream, err := executor.Stream(context.Background(), ExecuteRequest{
		RequestID:    "req_stream",
		Client:       "claude",
		Model:        "sonnet",
		Requirements: router.Requirements{Streaming: true},
		Request:      protocol.Request{Model: "sonnet", Stream: true},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	seenStop := false
	for event := range stream.Events {
		if event.Type == "message_stop" {
			seenStop = true
		}
	}
	if !seenStop {
		t.Fatal("stream did not emit message_stop")
	}
}

func TestExecutorStreamFallbackBeforeEvents(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer second.Close()

	cfg := twoTargetConfig(first.URL, second.URL)
	executor := executorFromConfig(t, cfg, observability.NewNoopSink())
	stream, err := executor.Stream(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Requirements: router.Requirements{Streaming: true}, Request: protocol.Request{Model: "sonnet", Stream: true}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	if len(stream.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(stream.Attempts))
	}
}

func TestExecutorStreamUnsupportedCapabilityDoesNotCallUpstream(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].Type = "gemini"
	cfg.Providers[0].BaseURL = "https://example.test"
	executor := executorFromConfig(t, cfg, observability.NewNoopSink())
	_, err := executor.Stream(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Requirements: router.Requirements{Streaming: true}, Request: protocol.Request{Model: "sonnet", Stream: true}})
	if err == nil {
		t.Fatal("Stream() error = nil, want unsupported capability")
	}
	var execErr *ExecutionError
	if !AsExecutionError(err, &execErr) || execErr.Class != ErrorUnsupportedCapability {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
}

func TestExecutorStreamClosesOnContextCancel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"index\":0}]}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	executor := testExecutorWithSink(t, upstream.URL, observability.NewNoopSink())
	stream, err := executor.Stream(ctx, ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Requirements: router.Requirements{Streaming: true}, Request: protocol.Request{Model: "sonnet", Stream: true}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-stream.Events:
			if !ok {
				if err := stream.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
				return
			}
		case <-deadline:
			t.Fatal("stream did not close after cancel")
		}
	}
}
```

- [ ] **Step 2: Run streaming tests and verify failure**

Run:

```sh
go test ./internal/runtime -run Stream -v
```

Expected: fails because `Executor.Stream` does not exist.

- [ ] **Step 3: Add StreamResult**

Modify `internal/runtime/attempt.go`:

```go
type StreamResult struct {
	Target   router.Target
	Attempts []Attempt
	Events   <-chan protocol.StreamEvent
	Close    func() error
}
```

- [ ] **Step 4: Implement runtime streaming**

Add to `internal/runtime/executor.go`:

```go
func (e *Executor) Stream(ctx context.Context, req ExecuteRequest) (StreamResult, error) {
	req.Request.Stream = true
	e.emit(req, observability.EventRequestStarted, observability.TraceEvent{})
	plan, err := e.Router.Plan(req.Model, req.Requirements)
	if err != nil {
		return StreamResult{}, &ExecutionError{Class: ErrorRouteNotFound, Message: err.Error()}
	}
	targets, reason := router.PolicyFor(plan.Strategy).Select(plan, e.Health.Snapshot())
	attempts := []Attempt{}
	for i, target := range targets {
		providerAdapter, ok := e.Adapters.Get(target.Provider.Type)
		if !ok {
			return StreamResult{}, &ExecutionError{Class: ErrorUpstreamFatal, Message: "unsupported provider type " + target.Provider.Type, Attempts: attempts}
		}
		mapper, ok := providerAdapter.NewStreamMapper()
		if !ok {
			return StreamResult{}, &ExecutionError{Class: ErrorUnsupportedCapability, Message: "provider does not support streaming", Attempts: attempts}
		}
		upstreamReq, err := providerAdapter.BuildRequest(req.Request, target.Provider, target.Model)
		if err != nil {
			return StreamResult{}, &ExecutionError{Class: ErrorInvalidRequest, Message: err.Error(), Attempts: attempts}
		}
		start := time.Now()
		streamCtx, cancel := context.WithCancel(ctx)
		httpReq, err := http.NewRequestWithContext(streamCtx, upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
		if err != nil {
			cancel()
			return StreamResult{}, &ExecutionError{Class: ErrorInvalidRequest, Message: err.Error(), Attempts: attempts}
		}
		httpReq.Header = upstreamReq.Headers.Clone()
		e.emit(req, observability.EventTargetSelected, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Reason: reason}))
		upstreamResp, err := e.Client.Do(httpReq)
		if err != nil {
			class := ErrorUpstreamFatal
			if streamCtx.Err() != nil {
				class = ErrorUpstreamTimeout
			}
			attempt := Attempt{Target: target, ErrorClass: class, ErrorMessage: err.Error(), Retryable: class.Retryable(), Latency: time.Since(start)}
			attempts = append(attempts, attempt)
			if !attempt.Retryable || i == len(targets)-1 {
				cancel()
				return StreamResult{}, &ExecutionError{Class: class, Message: err.Error(), Attempts: attempts}
			}
			cancel()
			continue
		}
		if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
			body, _ := io.ReadAll(upstreamResp.Body)
			_ = upstreamResp.Body.Close()
			cancel()
			class := providerAdapter.ClassifyError(upstreamResp.StatusCode, body)
			attempt := Attempt{Target: target, StatusCode: upstreamResp.StatusCode, ErrorClass: class, ErrorMessage: fmt.Sprintf("upstream returned %d", upstreamResp.StatusCode), Retryable: class.Retryable(), Latency: time.Since(start)}
			attempts = append(attempts, attempt)
			if !attempt.Retryable || i == len(targets)-1 {
				return StreamResult{}, &ExecutionError{Class: class, Message: attempt.ErrorMessage, Attempts: attempts}
			}
			e.emit(req, observability.EventFallback, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: attempt.StatusCode, Retryable: true, ErrorClass: string(class), Reason: attempt.ErrorMessage}))
			continue
		}
		events := make(chan protocol.StreamEvent, 16)
		done := make(chan struct{})
		var closeBody sync.Once
		closeBodyOnce := func() {
			closeBody.Do(func() {
				_ = upstreamResp.Body.Close()
			})
		}
		go func() {
			defer close(events)
			defer close(done)
			defer closeBodyOnce()
			scanner := bufio.NewScanner(upstreamResp.Body)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				mapped, err := mapper.MapLine(scanner.Bytes())
				if err != nil {
					select {
					case events <- protocol.StreamEvent{Type: "error", Error: err.Error()}:
					case <-streamCtx.Done():
					}
					return
				}
				for _, event := range mapped {
					select {
					case events <- event:
					case <-streamCtx.Done():
						return
					}
				}
				select {
				case <-streamCtx.Done():
					return
				default:
				}
			}
			if err := scanner.Err(); err != nil && streamCtx.Err() == nil {
				select {
				case events <- protocol.StreamEvent{Type: "error", Error: err.Error()}:
				case <-streamCtx.Done():
				}
			}
		}()
		closeFn := func() error {
			cancel()
			closeBodyOnce()
			<-done
			return nil
		}
		e.Health.Update(router.Update{ProviderID: target.Provider.ID, UpstreamModel: target.Model.UpstreamModel, Status: "ok", StatusCode: upstreamResp.StatusCode, Latency: time.Since(start)})
		e.emit(req, observability.EventStreamStarted, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: upstreamResp.StatusCode}))
		return StreamResult{Target: target, Attempts: append(attempts, Attempt{Target: target, StatusCode: upstreamResp.StatusCode, Latency: time.Since(start)}), Events: events, Close: closeFn}, nil
	}
	return StreamResult{}, &ExecutionError{Class: ErrorRouteNotFound, Message: "route has no targets", Attempts: attempts}
}
```

Add `bufio` and `sync` to the import block in `internal/runtime/executor.go`.

- [ ] **Step 5: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/runtime
git commit -m "feat: add runtime streaming execution"
```

---

### Task 6: Move Claude Handler To Runtime Executor

**Files:**
- Modify: `internal/client/claude/server.go`
- Modify: `internal/client/claude/messages.go`
- Modify: `internal/client/claude/server_test.go`
- Modify: `internal/app/serve.go`

- [ ] **Step 1: Add regression assertion for no direct adapter selection**

Add a test to `internal/client/claude/server_test.go`:

```go
func TestClaudeServerUsesRuntimeExecutor(t *testing.T) {
	srv := testServer(t)
	if srv.deps.Executor == nil {
		t.Fatal("Claude server missing runtime executor")
	}
}
```

- [ ] **Step 2: Run Claude tests and verify failure**

Run:

```sh
go test ./internal/client/claude -run TestClaudeServerUsesRuntimeExecutor -v
```

Expected: fails because `Deps.Executor` does not exist.

- [ ] **Step 3: Change Claude server dependencies**

Modify `internal/client/claude/server.go`:

```go
import (
	"encoding/json"
	"net/http"

	"bat.dev/arkrouter/internal/config"
	arkruntime "bat.dev/arkrouter/internal/runtime"
)

type Deps struct {
	Snapshot config.Snapshot
	Executor *arkruntime.Executor
}
```

Keep `Snapshot` for models/admin rendering. Remove `Router` and `Health` from Claude deps after tests are adjusted.

- [ ] **Step 4: Update tests to build executor**

Modify `testServer` helper in `internal/client/claude/server_test.go` to build:

```go
health := router.NewHealthStore()
executor := arkruntime.NewExecutor(arkruntime.Deps{
	Snapshot: snapshot,
	Router:   router.New(snapshot, health),
	Adapters: builtin.DefaultRegistry(),
	Health:   health,
	Trace:    observability.NewNoopSink(),
	Client:   &http.Client{Timeout: time.Second},
})
return NewServer(Deps{Snapshot: snapshot, Executor: executor})
```

Add imports for `internal/adapter/builtin`, `internal/observability`, `internal/runtime`, and `time`.

- [ ] **Step 5: Refactor `handleMessages` to call runtime**

In `internal/client/claude/messages.go`:

- Remove imports for provider adapters, `bytes`, and direct upstream `http.Client` use.
- Keep Anthropic decode and normalized mapping.
- Replace non-streaming loop with:

```go
result, err := s.deps.Executor.Execute(r.Context(), arkruntime.ExecuteRequest{
	RequestID:    requestID(r),
	Client:       "claude",
	Model:        anthropicReq.Model,
	Requirements: router.Requirements{Streaming: anthropicReq.Stream, Tools: len(anthropicReq.Tools) > 0},
	Request:      normalized,
})
if err != nil {
	writeExecutionError(w, err)
	return
}
writeJSON(w, http.StatusOK, mapNormalizedResponse(result.Response, result.Target.Model.ExposedAlias))
```

- Replace streaming direct upstream code with:

```go
stream, err := s.deps.Executor.Stream(r.Context(), arkruntime.ExecuteRequest{
	RequestID:    requestID(r),
	Client:       "claude",
	Model:        anthropicReq.Model,
	Requirements: router.Requirements{Streaming: true, Tools: len(anthropicReq.Tools) > 0},
	Request:      normalized,
})
if err != nil {
	writeExecutionError(w, err)
	return
}
defer stream.Close()
s.writeStreamingResponse(w, stream, stream.Target.Model.ExposedAlias)
return
```

Add helpers:

```go
func requestID(r *http.Request) string {
	if value := r.Header.Get("x-request-id"); value != "" {
		return value
	}
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func writeExecutionError(w http.ResponseWriter, err error) {
	var execErr *arkruntime.ExecutionError
	if arkruntime.AsExecutionError(err, &execErr) {
		status := http.StatusBadGateway
		if execErr.Class == arkruntime.ErrorRouteNotFound {
			status = http.StatusNotFound
		}
		if execErr.Class == arkruntime.ErrorInvalidRequest || execErr.Class == arkruntime.ErrorUnsupportedCapability {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, anthropicError(string(execErr.Class), execErr.Message))
		return
	}
	writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
}
```

Import aliases:

```go
arkruntime "bat.dev/arkrouter/internal/runtime"
```

Update `handleCountTokens` in the same file so it no longer reads `s.deps.Router` after the dependency migration:

```go
if _, err := s.deps.Executor.Router.Plan(req.Model, router.Requirements{Tools: len(req.Tools) > 0}); err != nil {
	writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
	return
}
```

- [ ] **Step 6: Refactor streaming writer to consume runtime stream**

Modify `internal/client/claude/stream.go` to add:

```go
func (s *Server) writeStreamingResponse(w http.ResponseWriter, stream arkruntime.StreamResult, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	for event := range stream.Events {
		writeAnthropicStreamEvent(w, event, model)
	}
}
```

Import runtime alias.

Remove `handleStreamingResponse` from `messages.go` after migration.

- [ ] **Step 7: Update `app.Serve` to build executor**

Modify `internal/app/serve.go`:

```go
health := router.NewHealthStore()
executor := arkruntime.NewExecutor(arkruntime.Deps{
	Snapshot: snapshot,
	Router:   router.New(snapshot, health),
	Adapters: builtin.DefaultRegistry(),
	Health:   health,
	Trace:    observability.NewNoopSink(),
	Client:   &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second},
})
server := claude.NewServer(claude.Deps{Snapshot: snapshot, Executor: executor})
```

Add imports for adapter/builtin, observability, runtime alias, and time.

- [ ] **Step 8: Assert old direct selection is gone**

Run:

```sh
rg -n "selectAdapter|openaiadapter.NewStreamMapper|http.Client\\{Timeout" internal/client/claude
```

Expected: no output.

- [ ] **Step 9: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/client/claude internal/app internal/runtime
git commit -m "refactor: route claude execution through runtime"
```

---

### Task 7: Admin API And CLI Status/List Commands

**Files:**
- Create: `internal/client/claude/admin.go`
- Modify: `internal/client/claude/server.go`
- Modify: `internal/client/claude/server_test.go`
- Create: `internal/config/redact.go`
- Create: `internal/config/redact_test.go`
- Modify: `internal/app/commands.go`
- Create: `internal/app/commands_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Add redacted config tests**

Create `internal/config/redact_test.go`:

```go
package config

import "testing"

func TestRedactedHidesSecrets(t *testing.T) {
	cfg := MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-test"
	redacted := Redacted(cfg)
	if redacted.Server.ClientKey != "[redacted]" {
		t.Fatalf("client key = %q", redacted.Server.ClientKey)
	}
	if redacted.Providers[0].APIKey != "[redacted]" {
		t.Fatalf("api key = %q", redacted.Providers[0].APIKey)
	}
}
```

- [ ] **Step 2: Add admin endpoint tests**

Append to `internal/client/claude/server_test.go`:

```go
func TestInternalStatusRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestInternalConfigRedactsSecrets(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/config", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"schema_version":1`) {
		t.Fatalf("missing schema version: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "local-key") {
		t.Fatalf("config leaked local key: %s", rec.Body.String())
	}
}
```

- [ ] **Step 3: Add CLI command tests**

Append to `internal/cli/cli_test.go`:

```go
func TestRunConfigPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "config", "path"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ".arkrouter") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunProviderListMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "provider", "list", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "provider list failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunStatusMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "status", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "status failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

Create `internal/app/commands_test.go`:

```go
package app

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"gopkg.in/yaml.v3"
)

func TestPrintStatusFallsBackWhenServerUnreachable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 1
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PrintStatus(path, &out); err != nil {
		t.Fatalf("PrintStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "server: unreachable") {
		t.Fatalf("status output = %q", out.String())
	}
}

func TestPrintStatusUsesAdminWhenReachable(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":1,"version":"dev","provider_count":1,"model_count":1,"route_count":1}`))
	}))
	defer admin.Close()

	parsed, err := url.Parse(admin.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Host = host
	cfg.Server.Port = port
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PrintStatus(path, &out); err != nil {
		t.Fatalf("PrintStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "server: running") || !strings.Contains(out.String(), "version: dev") {
		t.Fatalf("status output = %q", out.String())
	}
}
```

- [ ] **Step 4: Run tests and verify failure**

Run:

```sh
go test ./internal/config ./internal/client/claude ./internal/app ./internal/cli
```

Expected: fails because redaction/admin/subcommands are missing.

- [ ] **Step 5: Implement redacted config**

Create `internal/config/redact.go`:

```go
package config

func Redacted(cfg Config) Config {
	out := cfg
	out.Server.ClientKey = "[redacted]"
	out.Providers = append([]ProviderConfig(nil), cfg.Providers...)
	for i := range out.Providers {
		if out.Providers[i].APIKey != "" {
			out.Providers[i].APIKey = "[redacted]"
		}
	}
	return out
}
```

- [ ] **Step 6: Add admin handlers**

Create `internal/client/claude/admin.go`:

```go
package claude

import (
	"net/http"

	"bat.dev/arkrouter/internal/buildinfo"
	"bat.dev/arkrouter/internal/config"
)

const adminSchemaVersion = 1

func (s *Server) handleInternalStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := s.deps.Snapshot
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"version":        buildinfo.Version,
		"commit":         buildinfo.Commit,
		"build_date":     buildinfo.BuildDate,
		"config_loaded_at": snapshot.LoadedAt,
		"provider_count": len(snapshot.ProvidersByID),
		"model_count":    len(snapshot.ModelsByID),
		"route_count":    len(snapshot.RoutesByAlias),
		"health":         s.deps.Executor.Health.Snapshot(),
		"trace":          s.deps.Executor.Trace.Stats(),
	})
}

func (s *Server) handleInternalConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"config":         config.Redacted(s.deps.Snapshot.Config),
	})
}

func (s *Server) handleInternalRoutes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"routes":         s.deps.Snapshot.RoutesByAlias,
	})
}

func (s *Server) handleInternalHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"health":         s.deps.Executor.Health.Snapshot(),
	})
}
```

Modify `Routes()` in `internal/client/claude/server.go`:

```go
mux.HandleFunc("/internal/status", s.withAuth(s.handleInternalStatus))
mux.HandleFunc("/internal/config", s.withAuth(s.handleInternalConfig))
mux.HandleFunc("/internal/routes", s.withAuth(s.handleInternalRoutes))
mux.HandleFunc("/internal/health", s.withAuth(s.handleInternalHealth))
```

- [ ] **Step 7: Implement app list/show helpers**

Add to `internal/app/commands.go`:

```go
func ConfigPath(w io.Writer) error {
	_, err := fmt.Fprintln(w, DefaultConfigPath())
	return err
}

func ShowConfig(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(config.Redacted(cfg), "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func ListProviders(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "PROVIDER\tTYPE\tENABLED\tBASE_URL")
	for _, provider := range cfg.Providers {
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", provider.ID, provider.Type, provider.Enabled, provider.BaseURL)
	}
	return nil
}

func ListModels(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "MODEL\tPROVIDER\tUPSTREAM\tALIAS\tSTREAM\tTOOLS")
	for _, model := range cfg.Models {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%t\n", model.ID, model.ProviderID, model.UpstreamModel, model.ExposedAlias, model.Capabilities.Streaming, model.Capabilities.Tools)
	}
	return nil
}

func ListRoutes(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "ROUTE\tSTRATEGY\tTARGETS")
	for _, route := range cfg.Routes {
		targets := make([]string, 0, len(route.Targets))
		for _, target := range route.Targets {
			targets = append(targets, target.ModelID)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", route.Alias, route.Strategy, strings.Join(targets, ","))
	}
	return nil
}

func PrintStatus(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	adminURL := fmt.Sprintf("http://%s:%d/internal/status", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodGet, adminURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(req)
	if err != nil {
		fmt.Fprintf(w, "server: unreachable\nproviders: %d\nmodels: %d\nroutes: %d\n", len(cfg.Providers), len(cfg.Models), len(cfg.Routes))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("admin auth failed: status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("admin status failed: status %d", resp.StatusCode)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
		ProviderCount int   `json:"provider_count"`
		ModelCount    int   `json:"model_count"`
		RouteCount    int   `json:"route_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("admin status malformed: %w", err)
	}
	if payload.SchemaVersion != 1 {
		return fmt.Errorf("admin status malformed: schema_version %d", payload.SchemaVersion)
	}
	fmt.Fprintf(w, "server: running\nversion: %s\nproviders: %d\nmodels: %d\nroutes: %d\n", payload.Version, payload.ProviderCount, payload.ModelCount, payload.RouteCount)
	return nil
}
```

Add import `strings`. `internal/app/commands.go` already imports `encoding/json`, `net/http`, and `time`; keep those imports for status and route-test helpers.

- [ ] **Step 8: Wire CLI subcommands**

Modify `internal/cli/cli.go` cases, replacing the existing `status` case that calls `Doctor`:

```go
case "config":
	return runConfig(args[2:], stdout, stderr)
case "provider":
	return runProvider(args[2:], stdout, stderr)
case "model":
	return runModel(args[2:], stdout, stderr)
case "route":
	return runRoute(args[2:], stdout, stderr)
case "status":
	if err := app.PrintStatus(flagValue(args[2:], "--config"), stdout); err != nil {
		fmt.Fprintf(stderr, "status failed: %v\n", err)
		return 1
	}
	return 0
```

Add helpers:

```go
func runConfig(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: arkrouter config <path|show>")
		return 2
	}
	switch args[0] {
	case "path":
		if err := app.ConfigPath(stdout); err != nil {
			fmt.Fprintf(stderr, "config path failed: %v\n", err)
			return 1
		}
	case "show":
		if err := app.ShowConfig(flagValue(args[1:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "config show failed: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintln(stderr, "usage: arkrouter config <path|show>")
		return 2
	}
	return 0
}

func runProvider(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintln(stderr, "usage: arkrouter provider list")
		return 2
	}
	if err := app.ListProviders(flagValue(args[1:], "--config"), stdout); err != nil {
		fmt.Fprintf(stderr, "provider list failed: %v\n", err)
		return 1
	}
	return 0
}

func runModel(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintln(stderr, "usage: arkrouter model list")
		return 2
	}
	if err := app.ListModels(flagValue(args[1:], "--config"), stdout); err != nil {
		fmt.Fprintf(stderr, "model list failed: %v\n", err)
		return 1
	}
	return 0
}

func runRoute(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintln(stderr, "usage: arkrouter route list")
		return 2
	}
	if err := app.ListRoutes(flagValue(args[1:], "--config"), stdout); err != nil {
		fmt.Fprintf(stderr, "route list failed: %v\n", err)
		return 1
	}
	return 0
}
```

- [ ] **Step 9: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/client/claude internal/config internal/app internal/cli
git commit -m "feat: add admin api and operator status commands"
```

---

### Task 8: Logs Tail, Doctor, Config Store, And Migration

**Files:**
- Create: `internal/app/store.go`
- Create: `internal/app/store_test.go`
- Create: `internal/config/migrate.go`
- Create: `internal/config/migrate_test.go`
- Modify: `internal/app/commands.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Add migration tests**

Create `internal/config/migrate_test.go`:

```go
package config

import "testing"

func TestMigrateVersion1(t *testing.T) {
	cfg := MinimalValidConfig("local-key")
	migrated, err := Migrate(cfg)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if migrated.Version != 1 {
		t.Fatalf("version = %d, want 1", migrated.Version)
	}
}

func TestMigrateUnknownVersion(t *testing.T) {
	cfg := MinimalValidConfig("local-key")
	cfg.Version = 99
	_, err := Migrate(cfg)
	if err == nil {
		t.Fatal("Migrate() error = nil, want error")
	}
}
```

- [ ] **Step 2: Add store/log tests**

Create `internal/app/store_test.go`:

```go
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"gopkg.in/yaml.v3"
)

func TestFileStorePath(t *testing.T) {
	store := NewFileStore("/tmp/config.yaml")
	if store.Path() != "/tmp/config.yaml" {
		t.Fatalf("Path() = %q", store.Path())
	}
}

func TestPrintLogsTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "traces.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PrintLogsTail(path, 2, &out); err != nil {
		t.Fatalf("PrintLogsTail() error = %v", err)
	}
	if out.String() != "two\nthree\n" {
		t.Fatalf("out = %q", out.String())
	}
}

func TestDoctorMissingEnvReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 1
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Doctor(path, &out); err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !strings.Contains(out.String(), "env:OPENROUTER_API_KEY missing") {
		t.Fatalf("doctor output = %q", out.String())
	}
	if !strings.Contains(out.String(), "server: unreachable") {
		t.Fatalf("doctor output = %q", out.String())
	}
	if !strings.Contains(out.String(), "port:") {
		t.Fatalf("doctor output = %q", out.String())
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```sh
go test ./internal/config ./internal/app
```

Expected: fails because migration, store, and log tail are missing.

- [ ] **Step 4: Implement migration**

Create `internal/config/migrate.go`:

```go
package config

import "fmt"

type MigrationError struct {
	Version int
}

func (e MigrationError) Error() string {
	return fmt.Sprintf("unsupported config version %d", e.Version)
}

func Migrate(cfg Config) (Config, error) {
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Version != CurrentVersion {
		return Config{}, MigrationError{Version: cfg.Version}
	}
	return cfg, nil
}
```

Modify `internal/config/load.go` so `LoadFile` calls `Migrate(cfg)` before `ApplyDefaults`.

- [ ] **Step 5: Implement config store**

Create `internal/app/store.go`:

```go
package app

import (
	"os"
	"path/filepath"

	"bat.dev/arkrouter/internal/config"
	"gopkg.in/yaml.v3"
)

type Store interface {
	Path() string
	Load() (config.Config, error)
	Save(config.Config) error
}

type FileStore struct {
	path string
}

func NewFileStore(path string) FileStore {
	if path == "" {
		path = DefaultConfigPath()
	}
	return FileStore{path: path}
}

func (s FileStore) Path() string {
	return s.path
}

func (s FileStore) Load() (config.Config, error) {
	return config.LoadFile(s.path)
}

func (s FileStore) Save(cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
```

- [ ] **Step 6: Implement logs tail and doctor env checks**

Modify `internal/app/commands.go`:

```go
func PrintLogsTail(path string, tail int, w io.Writer) error {
	if path == "" {
		path = DefaultLogPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	compact := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			compact = append(compact, line)
		}
	}
	if tail > 0 && len(compact) > tail {
		compact = compact[len(compact)-tail:]
	}
	for _, line := range compact {
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	return nil
}
```

Update `PrintLogs` to call `PrintLogsTail(path, 0, w)`.

Enhance `Doctor` after config validation:

```go
missing := missingEnvRefs(cfg)
for _, envName := range missing {
	fmt.Fprintf(w, "env:%s missing\n", envName)
}
if portAvailable(cfg.Server.Host, cfg.Server.Port) {
	fmt.Fprintln(w, "port: available")
} else {
	fmt.Fprintln(w, "port: unavailable")
}
if serverReachable(cfg) {
	fmt.Fprintln(w, "server: reachable")
} else {
	fmt.Fprintln(w, "server: unreachable")
}
```

Add helper:

```go
func missingEnvRefs(cfg config.Config) []string {
	var missing []string
	for _, provider := range cfg.Providers {
		if strings.HasPrefix(provider.APIKey, "env:") {
			name := strings.TrimPrefix(provider.APIKey, "env:")
			if os.Getenv(name) == "" {
				missing = append(missing, name)
			}
		}
	}
	return missing
}

func portAvailable(host string, port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func serverReachable(cfg config.Config) bool {
	url := fmt.Sprintf("http://%s:%d/healthz", cfg.Server.Host, cfg.Server.Port)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}
```

Add imports to `internal/app/commands.go`: `net` and `strconv`.

- [ ] **Step 7: Wire `logs --tail`**

Modify `internal/cli/cli.go` logs case:

```go
case "logs":
	tail := intFlagValue(args[2:], "--tail", 0)
	if err := app.PrintLogsTail(flagValue(args[2:], "--file"), tail, stdout); err != nil {
		fmt.Fprintf(stderr, "logs failed: %v\n", err)
		return 1
	}
	return 0
```

Add helper:

```go
func intFlagValue(args []string, name string, fallback int) int {
	value := flagValue(args, name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
```

Import `strconv`.

- [ ] **Step 8: Verify and commit**

Run:

```sh
gofmt -w .
go test -count=1 ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/app internal/config internal/cli
git commit -m "feat: add config store migration and log tail"
```

---

### Task 9: Graceful Serve, Version Debug, Makefile, And README

**Files:**
- Modify: `internal/app/serve.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`
- Create: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Add CLI version debug test**

Append to `internal/cli/cli_test.go`:

```go
func TestRunVersionDebug(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "version", "--debug"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"version:", "commit:", "build_date:", "go:", "os_arch:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %s: %q", want, stdout.String())
		}
	}
}
```

- [ ] **Step 2: Run CLI test and verify failure**

Run:

```sh
go test ./internal/cli -run TestRunVersionDebug -v
```

Expected: fails because `version --debug` is not wired.

- [ ] **Step 3: Wire buildinfo into CLI version**

Modify `internal/cli/cli.go`:

```go
import "bat.dev/arkrouter/internal/buildinfo"
```

Replace version case:

```go
case "version":
	if hasFlag(args[2:], "--debug") {
		fmt.Fprint(stdout, buildinfo.Debug())
		return 0
	}
	fmt.Fprintln(stdout, buildinfo.Summary())
	return 0
```

- [ ] **Step 4: Implement graceful serve**

Modify `internal/app/serve.go` to open the JSONL trace file before constructing the executor:

```go
logPath := DefaultLogPath()
if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
	return fmt.Errorf("create log directory: %w", err)
}
logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
if err != nil {
	return fmt.Errorf("open trace log: %w", err)
}
defer logFile.Close()
trace := observability.NewJSONLSink(logFile)
```

In the executor construction from Task 6, replace:

```go
Trace:    observability.NewNoopSink(),
```

with:

```go
Trace:    trace,
```

Then use `http.Server` and signal shutdown:

```go
srv := &http.Server{Addr: addr, Handler: server.Routes()}
errCh := make(chan error, 1)
go func() {
	errCh <- srv.ListenAndServe()
}()
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
select {
case err := <-errCh:
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server startup failed: %w", err)
	}
	return nil
case <-stop:
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	return nil
}
```

Add imports: `context`, `os`, `os/signal`, `path/filepath`, `syscall`, and `time`.

- [ ] **Step 5: Add Makefile**

Create `Makefile`:

```makefile
BINARY := arkrouter
PREFIX ?= $(HOME)
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X bat.dev/arkrouter/internal/buildinfo.Version=$(VERSION) -X bat.dev/arkrouter/internal/buildinfo.Commit=$(COMMIT) -X bat.dev/arkrouter/internal/buildinfo.BuildDate=$(BUILD_DATE)

.PHONY: test build install clean

test:
	go test -count=1 ./...

build:
	mkdir -p dist
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) ./cmd/arkrouter

install: build
	mkdir -p $(BINDIR)
	cp dist/$(BINARY) $(BINDIR)/$(BINARY)

clean:
	rm -rf dist
```

- [ ] **Step 6: Update README**

Add sections to `README.md`:

```markdown
## Build And Install

```sh
make test
make build
make install
```

By default `make install` writes to `~/bin/arkrouter`.

```sh
make install PREFIX=/usr/local
```

## Operator Commands

```sh
arkrouter config path
arkrouter config show
arkrouter provider list
arkrouter model list
arkrouter route list
arkrouter status
arkrouter doctor
arkrouter test sonnet "hello"
arkrouter logs --tail 50
arkrouter version --debug
```

## Troubleshooting

Some tests use `httptest` and bind a local loopback port. If a restricted sandbox blocks local port binding, run the same `go test -count=1 ./...` command with permission to bind loopback.

Arkrouter redacts provider API keys and the local client key in status/config output. It does not log prompt or response bodies.
```

- [ ] **Step 7: Verify make targets and full test**

Run:

```sh
gofmt -w .
go test -count=1 ./...
make test
make build
./dist/arkrouter version --debug
```

Expected:

```text
version: dev
commit: <current short commit or unknown>
build_date: <UTC timestamp>
```

- [ ] **Step 8: Commit**

Commit:

```sh
git add Makefile README.md internal/app internal/cli internal/buildinfo
git commit -m "feat: add graceful serve and release foundation"
```

---

## Final Verification

Run from `/Users/bat/RiderProjects/arkrouter`:

```sh
gofmt -w .
go test -count=1 ./...
make test
make build
./dist/arkrouter version --debug
rg -n "selectAdapter|openaiadapter.NewStreamMapper|http.Client\\{Timeout" internal/client/claude
git status --short
```

Expected:

- Tests pass.
- Build succeeds.
- `version --debug` prints version, commit, build date, Go version, and OS/arch.
- The `rg` command prints no matches.
- `git status --short` prints no tracked or untracked files after final commit.

## Plan Self-Review

Spec coverage:

- Runtime executor, fallback, health, trace are covered by Tasks 1, 4, and 5.
- RoutePlan and policy are covered by Task 3.
- Adapter registry and stream mapper contract are covered by Task 2.
- Claude handler migration is covered by Task 6.
- Admin API, live/fallback status, and operator list commands are covered by Task 7.
- Config store, migration, log tail, doctor env/port/server checks are covered by Task 8.
- Graceful server lifecycle, JSONL trace file wiring, build metadata, Makefile, install behavior, README are covered by Task 9.

Scope check:

- No OpenAI-compatible ingress is added.
- No dashboard is added.
- No cloud/team/account/token compression work is included.
- Phase 1 behavior remains covered by existing HTTP tests and explicit final `rg` checks.

Placeholder scan:

- This plan does not use placeholder implementation language.
- Each task lists exact files, concrete tests, commands, and commit points.

Type consistency:

- Runtime package is imported as `arkruntime` in Claude package examples.
- Error class constants live in `internal/failure` and are re-exported by `internal/runtime` for handler ergonomics.
- Trace event names live in `internal/observability`.
- Route plans and policies live in `internal/router`.
