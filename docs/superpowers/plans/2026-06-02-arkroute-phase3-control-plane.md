# Arkroute Phase 3 Control Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add runtime config generations, hot reload, admin reload, CLI reload, and SIGHUP reload so Arkroute can run as a durable local Claude Code gateway without restart for config changes.

**Architecture:** Introduce `runtime.State` as the only owner of the current config generation. Claude handlers authenticate once against the current generation and use that same generation through request context; reload builds a fresh executor and atomically publishes it while keeping process-level health and trace state shared.

**Tech Stack:** Go 1.23+, standard `net/http`, `httptest`, `sync.RWMutex`, JSONL trace events, YAML config via `gopkg.in/yaml.v3`.

---

## Source Spec

Implement from `docs/superpowers/specs/2026-06-02-arkroute-phase3-control-plane-design.md`.

Do not add OpenAI ingress, dashboard, file watching, config editing endpoints, SQLite, remote admin, CORS, weighted routing, health-aware routing, or schema version 2.

## File Structure

Create or modify these focused units:

- `internal/failure/errors.go`: add control-plane error classes.
- `internal/failure/errors_test.go`: regression tests for control-plane classes.
- `internal/observability/trace.go`: add reload events and reload metadata fields.
- `internal/observability/trace_test.go`: trace event tests for reload metadata.
- `internal/runtime/state.go`: new runtime generation state, reload, metadata, and listener-change guard.
- `internal/runtime/state_test.go`: generation/reload/failure/shared-state tests.
- `internal/client/claude/server.go`: replace fixed snapshot/executor deps with runtime state.
- `internal/client/claude/auth.go`: authenticate once, attach generation to request context.
- `internal/client/claude/models.go`: read generation from request context for `/healthz` and `/v1/models`.
- `internal/client/claude/messages.go`: execute using request generation.
- `internal/client/claude/admin.go`: generation-aware admin status/config/routes/health and reload endpoint.
- `internal/client/claude/server_test.go`: admin reload and generation behavior tests.
- `internal/app/serve.go`: construct runtime state and wire reload signal handling.
- `internal/app/signals_unix.go`: Unix reload signal helper.
- `internal/app/signals_windows.go`: Windows reload signal helper.
- `internal/app/commands.go`: add `Reload`, update `PrintStatus`, update `Doctor`.
- `internal/app/commands_test.go`: app-level reload and status tests.
- `internal/cli/cli.go`: add `arkroute reload` and flags.
- `internal/cli/cli_test.go`: CLI reload dispatch tests.
- `README.md`: document `arkroute reload`, `--addr`, `--client-key`, and SIGHUP.

## Implementation Rules

- Use TDD for behavior changes: write failing tests first, run the targeted test and confirm the expected failure, then implement.
- Keep `runtime.Executor` focused on request execution. Put current-generation ownership in `runtime.State`.
- Do not let `POST /internal/reload` accept a config path from the request.
- Do not hold a state write lock during upstream execution or streaming.
- Do not reset process-level `HealthStore` or `TraceSink` on reload.
- Failed reload must keep the previous generation active and must not increment generation.
- Host/port changes on reload must fail with `listener_change_requires_restart`.
- Auth middleware must acquire one generation, authenticate against that generation, and pass the same generation to the handler.
- Redact secrets in admin responses, CLI output, trace events, and reload errors.
- Commit after each task.

---

### Task 1: Control-Plane Error Classes And Reload Trace Metadata

**Files:**
- Modify: `internal/failure/errors.go`
- Modify: `internal/failure/errors_test.go`
- Modify: `internal/observability/trace.go`
- Modify: `internal/observability/trace_test.go`

- [ ] **Step 1: Add failing tests for control-plane error classes**

Append to `internal/failure/errors_test.go`:

```go
func TestControlPlaneErrorClassesAreNotRetryable(t *testing.T) {
	tests := []ErrorClass{
		ErrorConfigReloadFailed,
		ErrorConfigValidationFailed,
		ErrorConfigReadFailed,
		ErrorListenerChangeRequiresRestart,
		ErrorAdminAuthFailed,
		ErrorAdminMalformedResponse,
		ErrorServerUnreachable,
	}
	for _, class := range tests {
		if class.Retryable() {
			t.Fatalf("%s should not be retryable", class)
		}
	}
}
```

- [ ] **Step 2: Add failing tests for reload trace metadata**

Append to `internal/observability/trace_test.go`:

```go
func TestTraceReloadEventIncludesGenerationMetadata(t *testing.T) {
	var buf bytes.Buffer
	sink := NewJSONLSink(&buf)
	sink.Emit(TraceEvent{
		Time:                     time.Unix(0, 0).UTC(),
		Event:                    EventConfigReloadSucceeded,
		Client:                   "admin",
		ConfigGeneration:         2,
		PreviousConfigGeneration: 1,
		NextConfigGeneration:     2,
		ConfigPath:               "/Users/bat/.arkroute/config.yaml",
	})

	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &decoded); err != nil {
		t.Fatalf("trace is not json: %v", err)
	}
	if decoded["event"] != string(EventConfigReloadSucceeded) {
		t.Fatalf("event = %v", decoded["event"])
	}
	if decoded["config_generation"].(float64) != 2 {
		t.Fatalf("config_generation = %v, want 2", decoded["config_generation"])
	}
	if decoded["previous_config_generation"].(float64) != 1 {
		t.Fatalf("previous_config_generation = %v, want 1", decoded["previous_config_generation"])
	}
	if decoded["next_config_generation"].(float64) != 2 {
		t.Fatalf("next_config_generation = %v, want 2", decoded["next_config_generation"])
	}
	if decoded["config_path"] != "/Users/bat/.arkroute/config.yaml" {
		t.Fatalf("config_path = %v", decoded["config_path"])
	}
}
```

- [ ] **Step 3: Run targeted tests and verify failure**

Run:

```sh
go test -count=1 ./internal/failure ./internal/observability
```

Expected: fails because `ErrorConfigReloadFailed`, `EventConfigReloadSucceeded`, and trace metadata fields do not exist.

- [ ] **Step 4: Implement control-plane error classes**

Add constants to `internal/failure/errors.go`:

```go
const (
	ErrorConfigReloadFailed            ErrorClass = "config_reload_failed"
	ErrorConfigValidationFailed        ErrorClass = "config_validation_failed"
	ErrorConfigReadFailed              ErrorClass = "config_read_failed"
	ErrorListenerChangeRequiresRestart ErrorClass = "listener_change_requires_restart"
	ErrorAdminAuthFailed               ErrorClass = "admin_auth_failed"
	ErrorAdminMalformedResponse        ErrorClass = "admin_malformed_response"
	ErrorServerUnreachable             ErrorClass = "server_unreachable"
)
```

Leave `Retryable()` unchanged except for gofmt. These new classes intentionally fall through to `false`.

- [ ] **Step 5: Implement reload trace events and fields**

Add constants and fields to `internal/observability/trace.go`:

```go
const (
	EventConfigReloadStarted   EventName = "config_reload_started"
	EventConfigReloadSucceeded EventName = "config_reload_succeeded"
	EventConfigReloadFailed    EventName = "config_reload_failed"
)

type TraceEvent struct {
	SchemaVersion              int               `json:"schema_version"`
	Time                       time.Time         `json:"time"`
	Event                      EventName         `json:"event"`
	RequestID                  string            `json:"request_id"`
	Client                     string            `json:"client,omitempty"`
	Route                      string            `json:"route,omitempty"`
	Strategy                   string            `json:"strategy,omitempty"`
	Provider                   string            `json:"provider,omitempty"`
	ProviderType               string            `json:"provider_type,omitempty"`
	Model                      string            `json:"model,omitempty"`
	UpstreamModel              string            `json:"upstream_model,omitempty"`
	Status                     int               `json:"status,omitempty"`
	LatencyMS                  int64             `json:"latency_ms,omitempty"`
	Retryable                  bool              `json:"retryable,omitempty"`
	Reason                     string            `json:"reason,omitempty"`
	ErrorClass                 string            `json:"error_class,omitempty"`
	ConfigGeneration           uint64            `json:"config_generation,omitempty"`
	PreviousConfigGeneration   uint64            `json:"previous_config_generation,omitempty"`
	NextConfigGeneration       uint64            `json:"next_config_generation,omitempty"`
	ConfigPath                 string            `json:"config_path,omitempty"`
	Headers                    map[string]string `json:"headers,omitempty"`
}
```

- [ ] **Step 6: Run targeted tests and verify pass**

Run:

```sh
gofmt -w internal/failure/errors.go internal/failure/errors_test.go internal/observability/trace.go internal/observability/trace_test.go
go test -count=1 ./internal/failure ./internal/observability
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```sh
git add internal/failure/errors.go internal/failure/errors_test.go internal/observability/trace.go internal/observability/trace_test.go
git commit -m "feat: add reload error and trace metadata"
```

---

### Task 2: Runtime State And Atomic Reload

**Files:**
- Create: `internal/runtime/state.go`
- Create: `internal/runtime/state_test.go`

- [ ] **Step 1: Add failing runtime state tests**

Create `internal/runtime/state_test.go`:

```go
package runtime

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
	"gopkg.in/yaml.v3"
)

func TestStateStartsAtGenerationOne(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", 20128)

	current := state.Current()
	if current.Number != 1 {
		t.Fatalf("generation = %d, want 1", current.Number)
	}
	if current.Executor == nil || current.Router == nil {
		t.Fatalf("current generation missing executor/router: %+v", current)
	}
	if state.Status().ConfigPath != path {
		t.Fatalf("config path = %q, want %q", state.Status().ConfigPath, path)
	}
}

func TestStateReloadSuccessSwapsGenerationAndKeepsSharedState(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	health := router.NewHealthStore()
	sink := newRuntimeStateMemorySink()
	state := newRuntimeStateForTestWithShared(t, path, "127.0.0.1", 20128, health, sink)

	cfg.Routes = append(cfg.Routes, config.RouteConfig{
		Alias:    "opus",
		Strategy: "priority",
		Targets:  []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}},
		Enabled:  true,
	})
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}
	if result.Generation != 2 {
		t.Fatalf("generation = %d, want 2", result.Generation)
	}
	if _, ok := state.Current().Snapshot.RoutesByAlias["opus"]; !ok {
		t.Fatalf("new route not visible after reload")
	}
	if state.Health() != health {
		t.Fatalf("health store was replaced")
	}
	if state.Trace() != sink {
		t.Fatalf("trace sink was replaced")
	}
	if sink.count(EventNameStringConfigReloadSucceeded) == 0 {
		t.Fatalf("reload success trace was not emitted")
	}
}

func TestStateReloadFailureKeepsCurrentGeneration(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", 20128)

	cfg.Routes[0].Targets = nil
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.Generation != 1 {
		t.Fatalf("active generation = %d, want 1", result.Generation)
	}
	if state.Current().Number != 1 {
		t.Fatalf("current generation = %d, want 1", state.Current().Number)
	}
	status := state.Status()
	if status.FailedReloadCount != 1 {
		t.Fatalf("failed reload count = %d, want 1", status.FailedReloadCount)
	}
	if status.LastReloadErrorClass != string(failure.ErrorConfigValidationFailed) {
		t.Fatalf("last reload error class = %q", status.LastReloadErrorClass)
	}
	if !strings.Contains(status.LastReloadError, "routes[0].targets") {
		t.Fatalf("last reload error = %q", status.LastReloadError)
	}
}

func TestStateReloadRejectsListenerChange(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", 20128)

	cfg.Server.Port = 20129
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.ErrorClass != failure.ErrorListenerChangeRequiresRestart {
		t.Fatalf("error class = %s, want listener_change_requires_restart", result.ErrorClass)
	}
	if state.Current().Number != 1 {
		t.Fatalf("generation changed to %d", state.Current().Number)
	}
}

func TestStateSerializesConcurrentReloads(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", 20128)

	const reloads = 5
	var wg sync.WaitGroup
	results := make(chan ReloadResult, reloads)
	for i := 0; i < reloads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- state.Reload(context.Background(), ReloadSourceAdmin, fmt.Sprintf("req_reload_%d", i))
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for result := range results {
		if !result.Success {
			t.Fatalf("reload failed: %+v", result)
		}
		successes++
	}
	if successes != reloads {
		t.Fatalf("successes = %d, want %d", successes, reloads)
	}
	if state.Current().Number != 1+reloads {
		t.Fatalf("generation = %d, want %d", state.Current().Number, 1+reloads)
	}
}

func writeRuntimeStateConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	overwriteRuntimeStateConfig(t, path, cfg)
	return path
}

func overwriteRuntimeStateConfig(t *testing.T, path string, cfg config.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newRuntimeStateForTest(t *testing.T, path, host string, port int) *State {
	t.Helper()
	return newRuntimeStateForTestWithShared(t, path, host, port, router.NewHealthStore(), observability.NewNoopSink())
}

func newRuntimeStateForTestWithShared(t *testing.T, path, host string, port int, health *router.HealthStore, trace observability.TraceSink) *State {
	t.Helper()
	state, err := NewState(StateDeps{
		ConfigPath:   path,
		ListenerHost: host,
		ListenerPort: port,
		Health:       health,
		Trace:        trace,
		NewHTTPClient: func(cfg config.Config) *http.Client {
			return &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second}
		},
	})
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	return state
}
```

Also add the memory sink helper in the same file:

```go
type runtimeStateMemorySink struct {
	events []observability.TraceEvent
}

const (
	EventNameStringConfigReloadSucceeded = "config_reload_succeeded"
)

func newRuntimeStateMemorySink() *runtimeStateMemorySink {
	return &runtimeStateMemorySink{}
}

func (s *runtimeStateMemorySink) Emit(event observability.TraceEvent) {
	s.events = append(s.events, event)
}

func (s *runtimeStateMemorySink) Stats() observability.Stats {
	return observability.Stats{Emitted: int64(len(s.events))}
}

func (s *runtimeStateMemorySink) count(event string) int {
	count := 0
	for _, item := range s.events {
		if string(item.Event) == event {
			count++
		}
	}
	return count
}
```

- [ ] **Step 2: Run runtime state tests and verify failure**

Run:

```sh
go test -count=1 ./internal/runtime -run 'TestState'
```

Expected: fails because `State`, `NewState`, `StateDeps`, and reload types do not exist.

- [ ] **Step 3: Implement runtime state types**

Create `internal/runtime/state.go` with these public types and methods:

```go
package runtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bloodstalk1/arkroute/internal/adapter"
	"github.com/bloodstalk1/arkroute/internal/adapter/builtin"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
)

type ReloadSource string

const (
	ReloadSourceAdmin  ReloadSource = "admin"
	ReloadSourceSignal ReloadSource = "signal"
)

type StateDeps struct {
	ConfigPath    string
	ListenerHost  string
	ListenerPort  int
	Adapters      adapter.Registry
	Health        *router.HealthStore
	Trace         observability.TraceSink
	NewHTTPClient func(config.Config) *http.Client
}

type State struct {
	mu            sync.RWMutex
	reloadMu      sync.Mutex
	configPath    string
	listenerHost  string
	listenerPort  int
	adapters      adapter.Registry
	health        *router.HealthStore
	trace         observability.TraceSink
	newHTTPClient func(config.Config) *http.Client
	current       *Generation
	meta          ReloadStatus
}

type Generation struct {
	Number   uint64
	LoadedAt time.Time
	Snapshot config.Snapshot
	Router   *router.Router
	Executor *Executor
}

type ReloadStatus struct {
	ConfigPath              string    `json:"config_path"`
	Generation              uint64    `json:"generation"`
	ConfigLoadedAt          time.Time `json:"config_loaded_at"`
	LastReloadAttemptAt     time.Time `json:"last_reload_attempt_at,omitempty"`
	LastSuccessfulReloadAt  time.Time `json:"last_successful_reload_at,omitempty"`
	LastFailedReloadAt      time.Time `json:"last_failed_reload_at,omitempty"`
	LastReloadErrorClass    string    `json:"last_reload_error_class,omitempty"`
	LastReloadError         string    `json:"last_reload_error,omitempty"`
	ReloadCount             uint64    `json:"reload_count"`
	FailedReloadCount       uint64    `json:"failed_reload_count"`
}

type ReloadResult struct {
	Success        bool
	Status         string
	Generation     uint64
	ConfigLoadedAt time.Time
	ErrorClass     failure.ErrorClass
	Error          string
}
```

- [ ] **Step 4: Implement state construction and accessors**

Add these functions to `internal/runtime/state.go`:

```go
func NewState(deps StateDeps) (*State, error) {
	if deps.ConfigPath == "" {
		return nil, fmt.Errorf("config path must be non-empty")
	}
	if deps.Health == nil {
		deps.Health = router.NewHealthStore()
	}
	if deps.Trace == nil {
		deps.Trace = observability.NewNoopSink()
	}
	if deps.Adapters == nil {
		deps.Adapters = builtin.DefaultRegistry()
	}
	if deps.NewHTTPClient == nil {
		deps.NewHTTPClient = func(cfg config.Config) *http.Client {
			return &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second}
		}
	}
	cfg, err := config.LoadFile(deps.ConfigPath)
	if err != nil {
		return nil, err
	}
	gen, err := buildGeneration(1, cfg, deps.Adapters, deps.Health, deps.Trace, deps.NewHTTPClient)
	if err != nil {
		return nil, err
	}
	state := &State{
		configPath:    deps.ConfigPath,
		listenerHost:  deps.ListenerHost,
		listenerPort:  deps.ListenerPort,
		adapters:      deps.Adapters,
		health:        deps.Health,
		trace:         deps.Trace,
		newHTTPClient: deps.NewHTTPClient,
		current:       gen,
	}
	state.meta = ReloadStatus{
		ConfigPath:     deps.ConfigPath,
		Generation:     gen.Number,
		ConfigLoadedAt: gen.LoadedAt,
	}
	return state, nil
}

func (s *State) Current() *Generation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *State) Status() ReloadStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

func (s *State) Health() *router.HealthStore {
	return s.health
}

func (s *State) Trace() observability.TraceSink {
	return s.trace
}

func buildGeneration(number uint64, cfg config.Config, adapters adapter.Registry, health *router.HealthStore, trace observability.TraceSink, newHTTPClient func(config.Config) *http.Client) (*Generation, error) {
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		return nil, err
	}
	rt := router.New(snapshot, health)
	executor := NewExecutor(Deps{
		Snapshot: snapshot,
		Router:   rt,
		Adapters: adapters,
		Health:   health,
		Trace:    trace,
		Client:   newHTTPClient(cfg),
	})
	return &Generation{Number: number, LoadedAt: snapshot.LoadedAt, Snapshot: snapshot, Router: rt, Executor: executor}, nil
}
```

- [ ] **Step 5: Implement reload with all-or-nothing semantics**

Add reload helpers to `internal/runtime/state.go`:

```go
func (s *State) Reload(ctx context.Context, source ReloadSource, requestID string) ReloadResult {
	_ = ctx
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	start := time.Now()
	before := s.Current()
	s.trace.Emit(observability.TraceEvent{
		Event:            observability.EventConfigReloadStarted,
		RequestID:        requestID,
		Client:           string(source),
		ConfigGeneration: before.Number,
		ConfigPath:       s.configPath,
	})

	s.mu.Lock()
	s.meta.LastReloadAttemptAt = time.Now().UTC()
	s.mu.Unlock()

	cfg, err := config.LoadFile(s.configPath)
	if err != nil {
		return s.recordReloadFailure(source, requestID, before, failure.ErrorConfigReadFailed, err, start)
	}
	if cfg.Server.Host != s.listenerHost || cfg.Server.Port != s.listenerPort {
		err := fmt.Errorf("server.host/server.port change requires restart: running %s:%d, config %s:%d", s.listenerHost, s.listenerPort, cfg.Server.Host, cfg.Server.Port)
		return s.recordReloadFailure(source, requestID, before, failure.ErrorListenerChangeRequiresRestart, err, start)
	}
	next, err := buildGeneration(before.Number+1, cfg, s.adapters, s.health, s.trace, s.newHTTPClient)
	if err != nil {
		return s.recordReloadFailure(source, requestID, before, classifyReloadBuildError(err), err, start)
	}

	s.mu.Lock()
	s.current = next
	s.meta.Generation = next.Number
	s.meta.ConfigLoadedAt = next.LoadedAt
	s.meta.LastSuccessfulReloadAt = time.Now().UTC()
	s.meta.LastReloadErrorClass = ""
	s.meta.LastReloadError = ""
	s.meta.ReloadCount++
	s.mu.Unlock()

	s.trace.Emit(observability.TraceEvent{
		Event:                    observability.EventConfigReloadSucceeded,
		RequestID:                requestID,
		Client:                   string(source),
		ConfigGeneration:         next.Number,
		PreviousConfigGeneration: before.Number,
		NextConfigGeneration:     next.Number,
		ConfigPath:               s.configPath,
		LatencyMS:                time.Since(start).Milliseconds(),
	})
	return ReloadResult{Success: true, Status: "reloaded", Generation: next.Number, ConfigLoadedAt: next.LoadedAt}
}

func (s *State) recordReloadFailure(source ReloadSource, requestID string, before *Generation, class failure.ErrorClass, err error, start time.Time) ReloadResult {
	message := sanitizeReloadError(err)
	s.mu.Lock()
	s.meta.LastFailedReloadAt = time.Now().UTC()
	s.meta.LastReloadErrorClass = string(class)
	s.meta.LastReloadError = message
	s.meta.FailedReloadCount++
	active := s.current
	s.mu.Unlock()

	s.trace.Emit(observability.TraceEvent{
		Event:            observability.EventConfigReloadFailed,
		RequestID:        requestID,
		Client:           string(source),
		ConfigGeneration: active.Number,
		ConfigPath:       s.configPath,
		LatencyMS:        time.Since(start).Milliseconds(),
		ErrorClass:       string(class),
		Reason:           message,
	})
	return ReloadResult{Success: false, Status: "failed", Generation: before.Number, ConfigLoadedAt: before.LoadedAt, ErrorClass: class, Error: message}
}

func classifyReloadBuildError(err error) failure.ErrorClass {
	if strings.Contains(err.Error(), "config validation failed") {
		return failure.ErrorConfigValidationFailed
	}
	return failure.ErrorConfigReloadFailed
}

func sanitizeReloadError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}
```

Keep `_ = ctx` in Phase 3. The public signature is intentionally context-aware for later reload work, while this phase performs local file reload synchronously.

- [ ] **Step 6: Run targeted tests and verify pass**

Run:

```sh
gofmt -w internal/runtime/state.go internal/runtime/state_test.go
go test -count=1 ./internal/runtime -run 'TestState'
```

Expected: all `TestState...` tests pass.

- [ ] **Step 7: Commit**

```sh
git add internal/runtime/state.go internal/runtime/state_test.go
git commit -m "feat: add runtime config generations"
```

---

### Task 3: Claude Server Uses Runtime Generation Context

**Files:**
- Modify: `internal/client/claude/server.go`
- Modify: `internal/client/claude/auth.go`
- Modify: `internal/client/claude/models.go`
- Modify: `internal/client/claude/messages.go`
- Modify: `internal/client/claude/server_test.go`

- [ ] **Step 1: Add failing tests for generation-aware server behavior**

Append to `internal/client/claude/server_test.go`:

```go
func TestModelsUsesReloadedGeneration(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})

	cfg.Routes = append(cfg.Routes, config.RouteConfig{
		Alias:    "opus",
		Strategy: "priority",
		Targets:  []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}},
		Enabled:  true,
	})
	overwriteClaudeServerConfig(t, path, cfg)
	if result := state.Reload(context.Background(), arkruntime.ReloadSourceAdmin, "req_reload"); !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "opus") {
		t.Fatalf("models response did not use reloaded generation: %s", rec.Body.String())
	}
}

func TestAuthUsesReloadedClientKey(t *testing.T) {
	cfg := config.MinimalValidConfig("old-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})

	cfg.Server.ClientKey = "new-key"
	overwriteClaudeServerConfig(t, path, cfg)
	if result := state.Reload(context.Background(), arkruntime.ReloadSourceAdmin, "req_reload"); !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer old-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old key status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer new-key")
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("new key status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
```

Add helper functions to `internal/client/claude/server_test.go`:

```go
func writeClaudeServerConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	overwriteClaudeServerConfig(t, path, cfg)
	return path
}

func overwriteClaudeServerConfig(t *testing.T, path string, cfg config.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testStateFromPath(t *testing.T, path, host string, port int) *arkruntime.State {
	t.Helper()
	state, err := arkruntime.NewState(arkruntime.StateDeps{
		ConfigPath:   path,
		ListenerHost: host,
		ListenerPort: port,
		Health:       router.NewHealthStore(),
		Trace:        observability.NewNoopSink(),
		NewHTTPClient: func(config.Config) *http.Client {
			return &http.Client{Timeout: time.Second}
		},
	})
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	return state
}
```

- [ ] **Step 2: Run Claude server tests and verify failure**

Run:

```sh
go test -count=1 ./internal/client/claude -run 'TestModelsUsesReloadedGeneration|TestAuthUsesReloadedClientKey'
```

Expected: fails because `Deps{State: ...}` is not supported and handlers still use fixed snapshot.

- [ ] **Step 3: Change server dependencies to runtime state**

Modify `internal/client/claude/server.go`:

```go
type Deps struct {
	State *arkruntime.State
}

type Server struct {
	deps Deps
}

func NewServer(deps Deps) *Server {
	return &Server{deps: deps}
}
```

Add route registration for reload:

```go
mux.HandleFunc("/internal/reload", s.withAuth(s.handleInternalReload))
```

- [ ] **Step 4: Authenticate once and store generation in request context**

Replace `internal/client/claude/auth.go` with:

```go
package claude

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type generationContextKey struct{}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gen := s.deps.State.Current()
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(gen.Snapshot.Config.Server.ClientKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"type": "error",
				"error": map[string]string{
					"type":    "authentication_error",
					"message": "invalid local client key",
				},
			})
			return
		}
		ctx := context.WithValue(r.Context(), generationContextKey{}, gen)
		next(w, r.WithContext(ctx))
	}
}

func generationFromRequest(r *http.Request) *arkruntime.Generation {
	if gen, ok := r.Context().Value(generationContextKey{}).(*arkruntime.Generation); ok && gen != nil {
		return gen
	}
	return nil
}
```

- [ ] **Step 5: Update models and health handlers**

Modify `internal/client/claude/models.go`:

```go
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	gen := s.deps.State.Current()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"loaded_at":  gen.LoadedAt,
		"generation": gen.Number,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	snapshot := gen.Snapshot
	entries := map[string]aproto.Model{}
	for _, route := range snapshot.RoutesByAlias {
		display := route.Alias
		context := 0
		if len(route.Targets) > 0 {
			if model, ok := snapshot.ModelsByID[route.Targets[0].ModelID]; ok {
				display = model.DisplayName
				context = model.Capabilities.ContextWindow
			}
		}
		entries[route.Alias] = aproto.Model{ID: route.Alias, DisplayName: display, ContextWindow: context}
		if route.ClaudeDiscoveryAlias != "" {
			entries[route.ClaudeDiscoveryAlias] = aproto.Model{ID: route.ClaudeDiscoveryAlias, DisplayName: display, ContextWindow: context}
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	models := make([]aproto.Model, 0, len(keys))
	for _, key := range keys {
		models = append(models, entries[key])
	}
	writeJSON(w, http.StatusOK, aproto.ModelsResponseFor(models))
}
```

- [ ] **Step 6: Update message handlers to use the request generation**

In `internal/client/claude/messages.go`, get `gen := generationFromRequest(r)` after decode. Replace `s.deps.Executor.Stream` with `gen.Executor.Stream`, `s.deps.Executor.Execute` with `gen.Executor.Execute`, and `s.deps.Executor.Router.Plan` with `gen.Router.Plan`.

Use this guard near the start of authenticated handlers:

```go
gen := generationFromRequest(r)
if gen == nil {
	writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
	return
}
```

- [ ] **Step 7: Update existing test helpers**

Modify `testServer(t)` and existing direct server construction in `server_test.go` so they call `testStateFromPath` and `NewServer(Deps{State: state})`.

For tests that customize an upstream URL, write the config to a temp file and construct state from that path. Keep each existing upstream `httptest.Server` setup unchanged.

- [ ] **Step 8: Run Claude server tests and verify pass**

Run:

```sh
gofmt -w internal/client/claude/server.go internal/client/claude/auth.go internal/client/claude/models.go internal/client/claude/messages.go internal/client/claude/server_test.go
go test -count=1 ./internal/client/claude
```

Expected: all Claude server tests pass.

- [ ] **Step 9: Commit**

```sh
git add internal/client/claude/server.go internal/client/claude/auth.go internal/client/claude/models.go internal/client/claude/messages.go internal/client/claude/server_test.go
git commit -m "feat: route Claude handlers through runtime generations"
```

---

### Task 4: Admin Reload Endpoint And Generation-Aware Admin Responses

**Files:**
- Modify: `internal/client/claude/admin.go`
- Modify: `internal/client/claude/server_test.go`

- [ ] **Step 1: Add failing admin reload tests**

Append to `internal/client/claude/server_test.go`:

```go
func TestInternalStatusIncludesGenerationMetadata(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"generation":1`, `"config_path":`, `"config_loaded_at":`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("internal status missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestInternalReloadRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/reload", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestInternalReloadReturnsSchemaOnFailure(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})

	cfg.Routes[0].Targets = nil
	overwriteClaudeServerConfig(t, path, cfg)

	req := httptest.NewRequest(http.MethodPost, "/internal/reload", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"status":"failed"`, `"generation":1`, `"error_class":"config_validation_failed"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("reload failure missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestInternalReloadReturnsSchemaOnSuccess(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})

	cfg.Routes = append(cfg.Routes, config.RouteConfig{
		Alias:    "fast",
		Strategy: "priority",
		Targets:  []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}},
		Enabled:  true,
	})
	overwriteClaudeServerConfig(t, path, cfg)

	req := httptest.NewRequest(http.MethodPost, "/internal/reload", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"status":"reloaded"`, `"generation":2`, `"config_loaded_at":`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("reload success missing %s: %s", want, rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Run admin tests and verify failure**

Run:

```sh
go test -count=1 ./internal/client/claude -run 'TestInternalStatusIncludesGenerationMetadata|TestInternalReload'
```

Expected: fails because `/internal/reload` and generation metadata responses are not implemented.

- [ ] **Step 3: Implement generation-aware admin responses**

Replace `internal/client/claude/admin.go` response functions with generation-aware versions:

```go
func (s *Server) handleInternalStatus(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	status := s.deps.State.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":             adminSchemaVersion,
		"version":                    buildinfo.Version,
		"commit":                     buildinfo.Commit,
		"build_date":                 buildinfo.BuildDate,
		"config_path":                status.ConfigPath,
		"generation":                 gen.Number,
		"config_loaded_at":           gen.LoadedAt,
		"last_reload_attempt_at":     status.LastReloadAttemptAt,
		"last_successful_reload_at":  status.LastSuccessfulReloadAt,
		"last_failed_reload_at":      status.LastFailedReloadAt,
		"last_reload_error_class":    status.LastReloadErrorClass,
		"last_reload_error":          status.LastReloadError,
		"reload_count":               status.ReloadCount,
		"failed_reload_count":        status.FailedReloadCount,
		"provider_count":             len(gen.Snapshot.ProvidersByID),
		"model_count":                len(gen.Snapshot.ModelsByID),
		"route_count":                len(gen.Snapshot.RoutesByAlias),
		"health":                     s.deps.State.Health().Snapshot(),
		"trace":                      s.deps.State.Trace().Stats(),
	})
}

func (s *Server) handleInternalConfig(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"generation":     gen.Number,
		"config":         config.Redacted(gen.Snapshot.Config),
	})
}

func (s *Server) handleInternalRoutes(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"generation":     gen.Number,
		"routes":         gen.Snapshot.RoutesByAlias,
	})
}

func (s *Server) handleInternalHealth(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"generation":     gen.Number,
		"health":         s.deps.State.Health().Snapshot(),
	})
}
```

- [ ] **Step 4: Implement `/internal/reload`**

Add to `internal/client/claude/admin.go`:

```go
func (s *Server) handleInternalReload(w http.ResponseWriter, r *http.Request) {
	result := s.deps.State.Reload(r.Context(), arkruntime.ReloadSourceAdmin, requestID(r))
	payload := map[string]any{
		"schema_version":   adminSchemaVersion,
		"status":           result.Status,
		"generation":       result.Generation,
		"config_loaded_at": result.ConfigLoadedAt,
	}
	if result.Success {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	payload["error_class"] = string(result.ErrorClass)
	payload["error"] = result.Error
	status := http.StatusInternalServerError
	if result.ErrorClass == failure.ErrorConfigValidationFailed || result.ErrorClass == failure.ErrorListenerChangeRequiresRestart {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, payload)
}
```

Update imports for `failure` and `arkruntime`.

- [ ] **Step 5: Run admin tests and verify pass**

Run:

```sh
gofmt -w internal/client/claude/admin.go internal/client/claude/server_test.go
go test -count=1 ./internal/client/claude -run 'TestInternalStatusIncludesGenerationMetadata|TestInternalReload|TestInternalConfigRedactsSecrets'
```

Expected: targeted tests pass.

- [ ] **Step 6: Commit**

```sh
git add internal/client/claude/admin.go internal/client/claude/server_test.go
git commit -m "feat: add authenticated admin reload"
```

---

### Task 5: CLI Reload And Status/Doctor Updates

**Files:**
- Modify: `internal/app/commands.go`
- Modify: `internal/app/commands_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Add failing app tests for reload client**

Append to `internal/app/commands_test.go`:

```go
func TestReloadPostsInternalReload(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/reload" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded","generation":2,"config_loaded_at":"2026-06-02T00:00:00Z"}`))
	}))
	defer admin.Close()

	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	if err := Reload(path, "", "", &out); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !strings.Contains(out.String(), "reloaded generation 2") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestReloadUsesClientKeyOverride(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded","generation":2,"config_loaded_at":"2026-06-02T00:00:00Z"}`))
	}))
	defer admin.Close()

	path := writeAppCommandConfigForURL(t, admin.URL, "new-key")
	var out bytes.Buffer
	if err := Reload(path, "", "old-key", &out); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
}

func TestReloadUsesAddressOverride(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded","generation":2,"config_loaded_at":"2026-06-02T00:00:00Z"}`))
	}))
	defer admin.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 1
	path := writeAppCommandConfig(t, cfg)
	var out bytes.Buffer
	if err := Reload(path, admin.URL, "", &out); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
}

func TestReloadReportsFailurePayload(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"failed","generation":1,"error_class":"config_validation_failed","error":"config validation failed: routes[0].targets: must contain at least one target"}`))
	}))
	defer admin.Close()

	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	err := Reload(path, "", "", &out)
	if err == nil {
		t.Fatal("Reload() error = nil")
	}
	if !strings.Contains(err.Error(), "routes[0].targets") {
		t.Fatalf("error = %v", err)
	}
}
```

Add helpers:

```go
func writeAppCommandConfigForURL(t *testing.T, rawURL string, key string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
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
	cfg := config.MinimalValidConfig(key)
	cfg.Server.Host = host
	cfg.Server.Port = port
	return writeAppCommandConfig(t, cfg)
}

func writeAppCommandConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
```

- [ ] **Step 2: Add failing CLI tests**

Append to `internal/cli/cli_test.go`:

```go
func TestRunReloadMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "reload", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "reload failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReloadParsesFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "reload", "--config", "/path/does/not/exist", "--addr", "http://127.0.0.1:20128", "--client-key", "old-key"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "reload failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

- [ ] **Step 3: Run reload app/CLI tests and verify failure**

Run:

```sh
go test -count=1 ./internal/app ./internal/cli -run 'TestReload|TestRunReload'
```

Expected: fails because `Reload` and the CLI command do not exist.

- [ ] **Step 4: Implement app reload command**

Add to `internal/app/commands.go`:

```go
func Reload(path string, addrOverride string, clientKeyOverride string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	addr := strings.TrimRight(addrOverride, "/")
	if addr == "" {
		addr = fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	}
	key := cfg.Server.ClientKey
	if clientKeyOverride != "" {
		key = clientKeyOverride
	}
	req, err := http.NewRequest(http.MethodPost, addr+"/internal/reload", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("server_unreachable: %w", err)
	}
	defer resp.Body.Close()
	var payload struct {
		SchemaVersion  int    `json:"schema_version"`
		Status         string `json:"status"`
		Generation     uint64 `json:"generation"`
		ConfigLoadedAt string `json:"config_loaded_at"`
		ErrorClass     string `json:"error_class"`
		Error          string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("admin_malformed_response: %w", err)
	}
	if payload.SchemaVersion != 1 {
		return fmt.Errorf("admin_malformed_response: schema_version %d", payload.SchemaVersion)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || payload.Status != "reloaded" {
		if payload.Error != "" {
			return fmt.Errorf("reload failed: %s", payload.Error)
		}
		return fmt.Errorf("reload failed: status %d", resp.StatusCode)
	}
	fmt.Fprintf(w, "reloaded generation %d\nconfig_loaded_at: %s\n", payload.Generation, payload.ConfigLoadedAt)
	return nil
}
```

- [ ] **Step 5: Update status and doctor output**

Update `PrintStatus` payload struct in `internal/app/commands.go`:

```go
var payload struct {
	SchemaVersion          int    `json:"schema_version"`
	Version                string `json:"version"`
	Generation             uint64 `json:"generation"`
	LastReloadErrorClass   string `json:"last_reload_error_class"`
	LastReloadError        string `json:"last_reload_error"`
	ProviderCount          int    `json:"provider_count"`
	ModelCount             int    `json:"model_count"`
	RouteCount             int    `json:"route_count"`
}
```

Update output:

```go
fmt.Fprintf(w, "server: running\nversion: %s\ngeneration: %d\nproviders: %d\nmodels: %d\nroutes: %d\n", payload.Version, payload.Generation, payload.ProviderCount, payload.ModelCount, payload.RouteCount)
if payload.LastReloadError != "" {
	fmt.Fprintf(w, "last_reload_error: %s %s\n", payload.LastReloadErrorClass, payload.LastReloadError)
}
```

In `Doctor`, after `server: reachable`, add:

```go
if reloadEndpointReachable(cfg) {
	fmt.Fprintln(w, "reload: reachable")
} else {
	fmt.Fprintln(w, "reload: unreachable")
}
```

Add helper:

```go
func reloadEndpointReachable(cfg config.Config) bool {
	url := fmt.Sprintf("http://%s:%d/internal/status", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
```

- [ ] **Step 6: Wire CLI command**

In `internal/cli/cli.go`, add case:

```go
case "reload":
	if err := app.Reload(flagValue(args[2:], "--config"), flagValue(args[2:], "--addr"), flagValue(args[2:], "--client-key"), stdout); err != nil {
		fmt.Fprintf(stderr, "reload failed: %v\n", err)
		return 1
	}
	return 0
```

Add help line:

```go
fmt.Fprintln(w, "  reload            Reload running server config")
```

- [ ] **Step 7: Run app and CLI tests**

Run:

```sh
gofmt -w internal/app/commands.go internal/app/commands_test.go internal/cli/cli.go internal/cli/cli_test.go
go test -count=1 ./internal/app ./internal/cli
```

Expected: all app and CLI tests pass.

- [ ] **Step 8: Commit**

```sh
git add internal/app/commands.go internal/app/commands_test.go internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: add reload CLI command"
```

---

### Task 6: Serve Runtime State And SIGHUP Reload

**Files:**
- Modify: `internal/app/serve.go`
- Create: `internal/app/signals_unix.go`
- Create: `internal/app/signals_windows.go`

- [ ] **Step 1: Add platform signal helpers**

Create `internal/app/signals_unix.go`:

```go
//go:build !windows

package app

import (
	"os"
	"syscall"
)

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func reloadSignals() []os.Signal {
	return []os.Signal{syscall.SIGHUP}
}
```

Create `internal/app/signals_windows.go`:

```go
//go:build windows

package app

import (
	"os"
	"syscall"
)

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func reloadSignals() []os.Signal {
	return nil
}
```

- [ ] **Step 2: Modify serve to construct runtime state**

In `internal/app/serve.go`, replace manual executor construction with:

```go
state, err := arkruntime.NewState(arkruntime.StateDeps{
	ConfigPath:   path,
	ListenerHost: cfg.Server.Host,
	ListenerPort: cfg.Server.Port,
	Adapters:     builtin.DefaultRegistry(),
	Health:       router.NewHealthStore(),
	Trace:        trace,
	NewHTTPClient: func(cfg config.Config) *http.Client {
		return &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second}
	},
})
if err != nil {
	return err
}
server := claude.NewServer(claude.Deps{State: state})
```

Keep the initial `config.LoadFile(path)` call in `Serve` so the listener address is known before `NewState`.

- [ ] **Step 3: Modify serve signal loop**

Replace the single `stop` channel with shutdown and reload channels:

```go
shutdown := make(chan os.Signal, 1)
signal.Notify(shutdown, shutdownSignals()...)
defer signal.Stop(shutdown)

reload := make(chan os.Signal, 1)
if signals := reloadSignals(); len(signals) > 0 {
	signal.Notify(reload, signals...)
	defer signal.Stop(reload)
}

for {
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server startup failed: %w", err)
		}
		return nil
	case <-reload:
		result := state.Reload(context.Background(), arkruntime.ReloadSourceSignal, "signal_sighup")
		if result.Success {
			fmt.Printf("arkroute reloaded generation %d\n", result.Generation)
		} else {
			fmt.Fprintf(os.Stderr, "arkroute reload failed: %s\n", result.Error)
		}
	case <-shutdown:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}
		return nil
	}
}
```

- [ ] **Step 4: Run app tests and full compile**

Run:

```sh
gofmt -w internal/app/serve.go internal/app/signals_unix.go internal/app/signals_windows.go
go test -count=1 ./internal/app
go test -count=1 ./cmd/arkroute
```

Expected: app tests pass and command package compiles.

- [ ] **Step 5: Commit**

```sh
git add internal/app/serve.go internal/app/signals_unix.go internal/app/signals_windows.go
git commit -m "feat: reload config on SIGHUP"
```

---

### Task 7: Streaming Generation Stability Regression

**Files:**
- Modify: `internal/client/claude/server_test.go`

- [ ] **Step 1: Add failing streaming generation test**

Append to `internal/client/claude/server_test.go`:

```go
func TestStreamingRequestKeepsOriginalGenerationDuringReload(t *testing.T) {
	releaseFirstStream := make(chan struct{})
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-releaseFirstStream
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"old\"},\"index\":0}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"new"},"finish_reason":"stop"}]}`))
	}))
	defer second.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = first.URL + "/v1"
	cfg.Providers[0].APIKey = "sk-old"
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})

	streamDone := make(chan string, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
		req.Header.Set("Authorization", "Bearer local-key")
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		streamDone <- rec.Body.String()
	}()

	cfg.Providers[0].BaseURL = second.URL + "/v1"
	overwriteClaudeServerConfig(t, path, cfg)
	if result := state.Reload(context.Background(), arkruntime.ReloadSourceAdmin, "req_reload"); !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}
	close(releaseFirstStream)

	streamBody := <-streamDone
	if !strings.Contains(streamBody, "old") {
		t.Fatalf("stream did not use original generation: %s", streamBody)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "new") {
		t.Fatalf("new request did not use reloaded generation: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run streaming test**

Run:

```sh
go test -count=1 ./internal/client/claude -run TestStreamingRequestKeepsOriginalGenerationDuringReload
```

Expected: passes if Task 3 correctly uses request-local generation. If it fails, fix handler code so stream execution uses `gen.Executor.Stream` captured at request start.

- [ ] **Step 3: Commit**

```sh
git add internal/client/claude/server_test.go
git commit -m "test: cover streaming generation stability"
```

---

### Task 8: README And Final Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README operator commands**

Add `reload` to the operator command list in `README.md`:

```sh
arkroute reload
arkroute reload --addr http://127.0.0.1:20128
arkroute reload --client-key <current-running-key>
```

- [ ] **Step 2: Add reload troubleshooting note**

Add this text to `README.md` troubleshooting:

```text
`arkroute reload` asks the running server to reload the config path it started with. If you edit `server.host` or `server.port`, the running listener cannot move without restart. Use `arkroute reload --addr http://127.0.0.1:20128` to contact the old listener and get the explicit restart-required error, then restart `arkroute serve`.

If you changed `server.client_key`, use `arkroute reload --client-key <current-running-key>` once so the running server can authenticate the reload request. After reload, new requests must use the new key.

On Unix, sending SIGHUP to the `arkroute serve` process triggers the same reload path as `arkroute reload`.
```

- [ ] **Step 3: Run full verification**

Run:

```sh
gofmt -w internal
go test -count=1 ./...
git diff --check
```

Expected: all tests pass and `git diff --check` reports no whitespace errors.

- [ ] **Step 4: Commit**

```sh
git add README.md
git commit -m "docs: document config reload"
```

---

## Final Review Checklist

After all tasks:

- [ ] `client/claude.Server` depends on `runtime.State`, not fixed `config.Snapshot` plus fixed `runtime.Executor`.
- [ ] `withAuth` captures one generation and handlers use the same generation from context.
- [ ] `/internal/reload` cannot receive or override a config path.
- [ ] Reload success increments generation by one.
- [ ] Reload failure keeps old generation and records sanitized metadata.
- [ ] Host/port change returns `listener_change_requires_restart`.
- [ ] Health and trace sink object identity survives reload.
- [ ] `arkroute reload --addr` and `--client-key` are implemented.
- [ ] SIGHUP code compiles on Unix and Windows.
- [ ] `go test -count=1 ./...` passes.
