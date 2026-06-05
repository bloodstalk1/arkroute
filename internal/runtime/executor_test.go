package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/adapter/builtin"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/router"
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
			Model:    "sonnet",
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

func TestExecutorIncludesUpstreamErrorBodyMessage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"ModelError","message":"Model qwen3.7-max is not supported for format oa-compat"}}`))
	}))
	defer upstream.Close()

	executor := testExecutor(t, upstream.URL)
	_, err := executor.Execute(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Request: protocol.Request{Model: "sonnet"}})
	if err == nil {
		t.Fatal("Execute() error = nil, want upstream auth error")
	}
	var execErr *ExecutionError
	if !AsExecutionError(err, &execErr) {
		t.Fatalf("error = %T %v, want ExecutionError", err, err)
	}
	if execErr.Class != ErrorUpstreamAuth {
		t.Fatalf("class = %q, want upstream auth", execErr.Class)
	}
	if execErr.Message != "upstream returned 401: Model qwen3.7-max is not supported for format oa-compat" {
		t.Fatalf("message = %q", execErr.Message)
	}
	if len(execErr.Attempts) != 1 || execErr.Attempts[0].ErrorMessage != execErr.Message {
		t.Fatalf("attempts = %+v, want propagated error message", execErr.Attempts)
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
	cfg.Models[1].ExposedAlias = "second-or"
	cfg.Models[1].ClaudeDiscoveryAlias = ""
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "first-model", Enabled: true}, {ModelID: "second-model", Enabled: true}}
	return cfg
}

type memorySink struct{ events []observability.TraceEvent }

func newMemorySink() *memorySink                          { return &memorySink{} }
func (s *memorySink) Emit(event observability.TraceEvent) { s.events = append(s.events, event) }
func (s *memorySink) Stats() observability.Stats {
	return observability.Stats{Emitted: int64(len(s.events))}
}
