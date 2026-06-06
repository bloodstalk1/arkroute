package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
	"gopkg.in/yaml.v3"
)

func TestModelsRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"authentication_error"`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, `"type":"error"`) {
		t.Fatalf("body used Anthropic error shape: %s", body)
	}
}

func TestModelsReturnsOpenAIModelList(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"object":"list"`, `"id":"sonnet"`, `"id":"sonnet-or"`, `"owned_by":"arkroute"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("models response missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "claude-sonnet-4-20250514") {
		t.Fatalf("OpenAI model list leaked Claude discovery alias: %s", body)
	}
}

func TestChatCompletionsNonStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("upstream path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("upstream auth = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		for _, want := range []string{`"model":"anthropic/claude-sonnet-4.5"`, `"content":"ping"`} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("upstream body missing %s: %s", want, body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_upstream","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	srv := testServerWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"object":"chat.completion"`, `"model":"sonnet"`, `"role":"assistant"`, `"content":"pong"`, `"prompt_tokens":4`, `"completion_tokens":2`} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat response missing %s: %s", want, body)
		}
	}
}

func TestChatCompletionsStreaming(t *testing.T) {
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
	srv := testServerWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"sonnet","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", contentType)
	}
	body := rec.Body.String()
	for _, want := range []string{`"object":"chat.completion.chunk"`, `"delta":{"role":"assistant"`, `"content":"hi"`, `data: [DONE]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream response missing %s: %s", want, body)
		}
	}
}

func TestChatCompletionsMissingModelUsesOpenAIErrorShape(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"missing","messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"error":`, `"type":"invalid_request_error"`, `"code":"route_not_found"`, `"param":"model"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("error response missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"type":"error"`) {
		t.Fatalf("body used Anthropic error shape: %s", body)
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return testServerWithConfig(t, config.MinimalValidConfig("local-key"))
}

func testServerWithConfig(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	path := writeOpenAIServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	return NewServer(Deps{State: state})
}

func writeOpenAIServerConfig(t *testing.T, cfg config.Config) string {
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

func testStateFromPath(t *testing.T, path, host string, port int) *arkruntime.State {
	t.Helper()
	state, err := arkruntime.NewState(arkruntime.StateDeps{
		ConfigPath:    path,
		ListenerHost:  host,
		ListenerPort:  port,
		Health:        router.NewHealthStore(),
		Trace:         observability.NewNoopSink(),
		NewHTTPClient: func(config.Config) *http.Client { return &http.Client{Timeout: time.Second} },
	})
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	return state
}
