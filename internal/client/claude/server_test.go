package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/adapter/builtin"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func TestModelsRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestModelsReturnsRouteAliases(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"sonnet", "claude-sonnet-4-20250514"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("models response missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestHealthz(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health body = %s", rec.Body.String())
	}
}

func TestMessagesNonStreamingOpenAICompatible(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("upstream path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("upstream auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	health := router.NewHealthStore()
	executor := arkruntime.NewExecutor(arkruntime.Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, health),
		Adapters: builtin.DefaultRegistry(),
		Health:   health,
		Trace:    observability.NewNoopSink(),
		Client:   &http.Client{Timeout: time.Second},
	})
	srv := NewServer(Deps{Snapshot: snapshot, Executor: executor})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"text":"pong"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCountTokensReturnsLocalEstimate(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"sonnet","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"input_tokens"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.MinimalValidConfig("local-key")
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
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
}

func TestMessagesStreamingOpenAICompatible(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"index\":0}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	health := router.NewHealthStore()
	executor := arkruntime.NewExecutor(arkruntime.Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, health),
		Adapters: builtin.DefaultRegistry(),
		Health:   health,
		Trace:    observability.NewNoopSink(),
		Client:   &http.Client{Timeout: time.Second},
	})
	srv := NewServer(Deps{Snapshot: snapshot, Executor: executor})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"event: message_start", "event: content_block_delta", "event: message_stop"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("stream missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestMessagesFallbackOnRetryableStatus(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate limited`))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"fallback ok"},"finish_reason":"stop"}]}`))
	}))
	defer second.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers = append(cfg.Providers, cfg.Providers[0])
	cfg.Providers[0].ID = "first"
	cfg.Providers[0].BaseURL = first.URL + "/v1"
	cfg.Providers[0].APIKey = "sk-first"
	cfg.Providers[1].ID = "second"
	cfg.Providers[1].BaseURL = second.URL + "/v1"
	cfg.Providers[1].APIKey = "sk-second"
	cfg.Models = append(cfg.Models, cfg.Models[0])
	cfg.Models[0].ID = "first-model"
	cfg.Models[0].ProviderID = "first"
	cfg.Models[0].ExposedAlias = "first-alias"
	cfg.Models[0].ClaudeDiscoveryAlias = ""
	cfg.Models[1].ID = "second-model"
	cfg.Models[1].ProviderID = "second"
	cfg.Models[1].ExposedAlias = "second-alias"
	cfg.Models[1].ClaudeDiscoveryAlias = ""
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "first-model", Enabled: true}, {ModelID: "second-model", Enabled: true}}
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	health := router.NewHealthStore()
	executor := arkruntime.NewExecutor(arkruntime.Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, health),
		Adapters: builtin.DefaultRegistry(),
		Health:   health,
		Trace:    observability.NewNoopSink(),
		Client:   &http.Client{Timeout: time.Second},
	})
	srv := NewServer(Deps{Snapshot: snapshot, Executor: executor})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fallback ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

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
