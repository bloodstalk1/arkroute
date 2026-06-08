package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	aproto "github.com/bloodstalk1/arkroute/internal/protocol/anthropic"
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

func TestModelsReturnsModelExposedAliases(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sonnet-or") {
		t.Fatalf("models response missing model exposed alias: %s", rec.Body.String())
	}
}

func TestModelsIncludesOpenAICompatibleListFields(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"object":"list"`, `"object":"model"`, `"owned_by":"arkroute"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("models response missing OpenAI-compatible field %s: %s", want, rec.Body.String())
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
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
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

func TestOpenAIChatCompletionsMountedOnGateway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object":"chat.completion"`) || !strings.Contains(rec.Body.String(), `"content":"pong"`) {
		t.Fatalf("OpenAI chat response = %s", rec.Body.String())
	}
}

func TestOpenAIResponsesMountedOnGateway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"sonnet","input":"ping"}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object":"response"`) || !strings.Contains(rec.Body.String(), `"output_text":"pong"`) {
		t.Fatalf("OpenAI responses response = %s", rec.Body.String())
	}
}

func TestMessagesDoesNotExposeOpenAICompatibleReasoningAsThinking(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","reasoning_content":"provider hidden reasoning","content":"visible answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	cfg.Models[0].UpstreamModel = "future-reasoning-model"
	cfg.Models[0].Capabilities.Reasoning = true
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, body)
	}
	if !strings.Contains(body, `"text":"visible answer"`) {
		t.Fatalf("body missing final text: %s", body)
	}
	for _, forbidden := range []string{`"type":"thinking"`, `"signature"`, "provider hidden reasoning"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("body exposed provider reasoning marker %s: %s", forbidden, body)
		}
	}
}

func TestMessagesMapsClaudeOutputEffortWhenConfigured(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"reasoning_effort":"max"`) {
			t.Fatalf("upstream body missing mapped reasoning_effort: %s", body)
		}
		if !strings.Contains(string(body), `"thinking":{"type":"enabled"}`) {
			t.Fatalf("upstream body missing enabled thinking: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	cfg.Models[0].UpstreamModel = "deepseek-v4-pro"
	cfg.Models[0].Capabilities.Reasoning = true
	cfg.Models[0].Reasoning = config.ReasoningConfig{Effort: "high", FollowClaudeEffort: testBoolPtr(true)}
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"thinking":{"type":"adaptive"},"output_config":{"effort":"xhigh"},"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
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

func TestMessagesBypassesClaudeTitleRequest(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("title bypass should not call upstream")
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":32,"messages":[{"role":"user","content":[{"type":"text","text":"Generate a concise title for this conversation. Return only the title."}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("upstream was called")
	}
	if !strings.Contains(rec.Body.String(), `"type":"text"`) {
		t.Fatalf("title bypass body = %s", rec.Body.String())
	}
}

func TestMessagesBypassesClaudeWarmupRequest(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("warmup bypass should not call upstream")
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"Warmup request. Respond only with OK."}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("upstream was called")
	}
	if !strings.Contains(rec.Body.String(), `"text":"ok"`) {
		t.Fatalf("warmup bypass body = %s", rec.Body.String())
	}
}

func TestMessagesDoesNotBypassUnknownModel(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"missing-model","max_tokens":32,"messages":[{"role":"user","content":[{"type":"text","text":"Generate a concise title for this conversation. Return only the title."}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestMessagesDoesNotBypassOrdinaryWarmupMention(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","content":"real upstream answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":64,"messages":[{"role":"user","content":[{"type":"text","text":"Explain warmup routines for a Go service."}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("upstream was not called")
	}
	if !strings.Contains(rec.Body.String(), `"text":"real upstream answer"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.MinimalValidConfig("local-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	return NewServer(Deps{State: state, ConfigPath: path})
}

func testBoolPtr(value bool) *bool {
	return &value
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
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
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

func TestAnthropicStreamEventWritesToolUse(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "content_block_start", Index: 0, Block: protocol.ContentBlock{Type: "tool_use", ID: "call_1", Name: "read_file"}}, "sonnet")
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "tool_input_delta", Index: 0, Delta: `{"path":"README.md"}`}, "sonnet")
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "message_delta", StopReason: "tool_use"}, "sonnet")

	body := rec.Body.String()
	for _, want := range []string{`"type":"tool_use"`, `"id":"call_1"`, `"name":"read_file"`, `"type":"input_json_delta"`, `"partial_json":"{\"path\":\"README.md\"}"`, `"stop_reason":"tool_use"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %s: %s", want, body)
		}
	}
}

func TestAnthropicStreamEventWritesThinking(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "content_block_start", Index: 0, Block: protocol.ContentBlock{Type: "thinking"}}, "sonnet")
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "thinking_delta", Index: 0, Delta: "I should inspect the docs."}, "sonnet")
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "signature_delta", Index: 0, Delta: "sig_1"}, "sonnet")
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "content_block_stop", Index: 0}, "sonnet")

	body := rec.Body.String()
	for _, want := range []string{`"type":"thinking"`, `"type":"thinking_delta"`, `"thinking":"I should inspect the docs."`, `"type":"signature_delta"`, `"signature":"sig_1"`, "event: content_block_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %s: %s", want, body)
		}
	}
}

func TestAnthropicStreamEventWritesError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAnthropicStreamEvent(rec, protocol.StreamEvent{Type: "error", Error: "stream parse failed"}, "sonnet")

	body := rec.Body.String()
	for _, want := range []string{"event: error", `"type":"error"`, `"type":"api_error"`, `"message":"stream parse failed"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %s: %s", want, body)
		}
	}
}

func TestMapAnthropicMessagesPreservesThinkingBlocks(t *testing.T) {
	messages := mapAnthropicMessages([]aproto.Message{{
		Role:    "assistant",
		Content: []byte(`[{"type":"thinking","thinking":"I should inspect the docs.","signature":"sig_1"},{"type":"tool_use","id":"toolu_1","name":"search_docs","input":{"query":"deepseek"}}]`),
	}})
	if len(messages) != 1 || len(messages[0].Content) != 2 {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[0].Content[0].Type != "thinking" || messages[0].Content[0].Thinking != "I should inspect the docs." || messages[0].Content[0].Signature != "sig_1" {
		t.Fatalf("thinking block = %+v", messages[0].Content[0])
	}
}

func TestMapNormalizedResponseEmitsThinkingBlocks(t *testing.T) {
	got := mapNormalizedResponse(protocol.Response{
		ID:   "msg_1",
		Role: protocol.RoleAssistant,
		Content: []protocol.ContentBlock{{
			Type:      "thinking",
			Thinking:  "I should inspect the docs.",
			Signature: "sig_1",
		}, {
			Type: "text",
			Text: "done",
		}},
	}, "sonnet")
	content, ok := got["content"].([]map[string]any)
	if !ok || len(content) != 2 {
		t.Fatalf("content = %#v", got["content"])
	}
	if content[0]["type"] != "thinking" || content[0]["thinking"] != "I should inspect the docs." || content[0]["signature"] != "sig_1" {
		t.Fatalf("thinking block = %#v", content[0])
	}
}

func TestStreamingRequestKeepsOriginalGenerationDuringReload(t *testing.T) {
	firstStreamStarted := make(chan struct{})
	releaseFirstStream := make(chan struct{})
	var releaseFirstStreamOnce sync.Once
	releaseFirstStreamSafely := func() {
		releaseFirstStreamOnce.Do(func() {
			close(releaseFirstStream)
		})
	}
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(firstStreamStarted)
		<-releaseFirstStream
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"old\"},\"index\":0}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer first.Close()
	defer releaseFirstStreamSafely()
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

	select {
	case <-firstStreamStarted:
	case <-time.After(time.Second):
		t.Fatal("stream request did not reach original upstream")
	}
	cfg.Providers[0].BaseURL = second.URL + "/v1"
	overwriteClaudeServerConfig(t, path, cfg)
	reloadDone := make(chan arkruntime.ReloadResult, 1)
	go func() {
		reloadDone <- state.Reload(context.Background(), arkruntime.ReloadSourceAdmin, "req_reload")
	}()
	var result arkruntime.ReloadResult
	select {
	case result = <-reloadDone:
	case <-time.After(time.Second):
		releaseFirstStreamSafely()
		t.Fatal("reload did not complete while stream was in flight")
	}
	if !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}
	releaseFirstStreamSafely()

	var streamBody string
	select {
	case streamBody = <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("stream request did not complete")
	}
	if !strings.Contains(streamBody, "old") {
		t.Fatalf("stream did not use original generation: %s", streamBody)
	}
	if strings.Contains(streamBody, "new") {
		t.Fatalf("stream used reloaded generation: %s", streamBody)
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
	if strings.Contains(rec.Body.String(), "old") {
		t.Fatalf("new request used original generation: %s", rec.Body.String())
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
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{State: state})
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

func TestInternalReloadRejectsGetWithoutReloading(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/internal/reload", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if status := state.Status(); status.Generation != 1 {
		t.Fatalf("GET /internal/reload changed generation to %d", status.Generation)
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

func TestInternalSetupSessionRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestInternalSetupSessionIssuesTokenForPanelOptions(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		SetupToken    string `json:"setup_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 1 || payload.SetupToken == "" {
		t.Fatalf("payload = %+v", payload)
	}
	optionsReq := httptest.NewRequest(http.MethodGet, "/internal/setup/options", nil)
	optionsReq.Header.Set("X-Arkroute-Setup-Token", payload.SetupToken)
	optionsRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("options status = %d, body = %s", optionsRec.Code, optionsRec.Body.String())
	}
}

func TestPanelProviderSaveTriggersReload(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := filepath.Join(t.TempDir(), "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{
		State:      state,
		ConfigPath: path,
	})

	// 1. Get setup token
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var sessionPayload struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}

	// 2. Call provider save with setup token
	body := strings.NewReader(`{"preset_id":"openrouter","api_key_mode":"config","api_key":"sk-secret","upstream_model":"anthropic/claude-sonnet-4.5","exposed_alias":"sonnet-or","route_alias":"sonnet"}`)
	providerReq := httptest.NewRequest(http.MethodPost, "/internal/setup/provider", body)
	providerReq.Header.Set("X-Arkroute-Setup-Token", sessionPayload.SetupToken)
	providerRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusOK {
		t.Fatalf("provider status = %d, body = %s", providerRec.Code, providerRec.Body.String())
	}

	// 3. Verify generation increased to 2
	if gen := state.Current().Number(); gen != 2 {
		t.Fatalf("expected state generation to be 2, got %d", gen)
	}
}

func TestPanelSetupLaterTriggersReload(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := filepath.Join(t.TempDir(), "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	srv := NewServer(Deps{
		State:      state,
		ConfigPath: path,
	})

	// 1. Get setup token
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var sessionPayload struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}

	// 2. Call setup later with setup token
	laterReq := httptest.NewRequest(http.MethodPost, "/internal/setup/later", nil)
	laterReq.Header.Set("X-Arkroute-Setup-Token", sessionPayload.SetupToken)
	laterRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(laterRec, laterReq)
	if laterRec.Code != http.StatusOK {
		t.Fatalf("later status = %d, body = %s", laterRec.Code, laterRec.Body.String())
	}

	// 3. Verify generation increased to 2
	if gen := state.Current().Number(); gen != 2 {
		t.Fatalf("expected state generation to be 2, got %d", gen)
	}
}

func TestCLIToolsMountedOnGatewayWithSetupSession(t *testing.T) {
	srv := testServer(t)
	sessionReq := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	sessionReq.Header.Set("Authorization", "Bearer local-key")
	sessionRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionRec.Code, sessionRec.Body.String())
	}
	var sessionPayload struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.Unmarshal(sessionRec.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-tools", nil)
	req.Header.Set("X-Arkroute-Setup-Token", sessionPayload.SetupToken)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"claude"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPolicyInspectMountedOnGatewayWithSetupSession(t *testing.T) {
	srv := testServer(t)
	sessionReq := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	sessionReq.Header.Set("Authorization", "Bearer local-key")
	sessionRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionRec.Code, sessionRec.Body.String())
	}
	var sessionPayload struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.Unmarshal(sessionRec.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=openrouter-sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", sessionPayload.SetupToken)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"model_id":"openrouter-sonnet"`, `"resolved_reasoning"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestGatewayMountsConfigExportEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := os.WriteFile(path, []byte(`version: 1
server:
  host: 127.0.0.1
  port: 2002
  client_key: local-key
  upstream_timeout_seconds: 600
clients:
  claude:
    enabled: true
    model_discovery: true
providers: []
models: []
routes: []
profiles: {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = cfg
	server := NewServer(Deps{ConfigPath: path})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodGet, "/internal/config/export?redacted=1", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "version: 1") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGatewayMountsPolicyOverrideEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := panelTestWriteConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Deps{ConfigPath: path})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodPut, "/internal/policy/override", strings.NewReader(`{"model_id":"openrouter-sonnet","replay":false}`))
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"policy_id":"model-openrouter-sonnet-compat"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func panelTestWriteConfig(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func TestGatewayMountsCLIContextEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := panelTestWriteConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Deps{ConfigPath: path})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-context?route_alias=sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"selected_alias":"sonnet"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGatewayMountsRoutePresetsEndpoint(t *testing.T) {
	server := NewServer(Deps{})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodGet, "/internal/route-presets", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deepseek-v4-pro"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestModelsIncludesRouteAndExposedAliases(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	server := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"id":"sonnet"`, `"id":"sonnet-or"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
}

