package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestChatCompletionsAcceptsOpenAISDKStylePayload(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fixture","choices":[{"message":{"role":"assistant","content":"fixture ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	srv := testServerWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "sonnet",
		"messages": [
			{"role": "developer", "content": "Use repo style."},
			{"role": "user", "content": [{"type": "input_text", "text": "hello"}]}
		],
		"max_completion_tokens": 128,
		"parallel_tool_calls": true,
		"stream_options": {"include_usage": true},
		"response_format": {"type": "text"},
		"metadata": {"client": "openai-sdk"}
	}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"content":"fixture ok"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChatCompletionsAcceptsDocumentedClientPayloads(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fixture","choices":[{"message":{"role":"assistant","content":"client fixture ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	srv := testServerWithConfig(t, cfg)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "cursor style",
			body: `{
				"model": "sonnet",
				"messages": [
					{"role": "system", "content": "You are editing code."},
					{"role": "user", "content": [{"type": "text", "text": "Refactor this."}]}
				],
				"max_tokens": 256,
				"temperature": 0.2,
				"top_p": 0.95,
				"user": "cursor-local"
			}`,
		},
		{
			name: "opencode style",
			body: `{
				"model": "sonnet",
				"messages": [
					{"role": "developer", "content": "Follow the repository conventions."},
					{"role": "user", "content": [{"type": "input_text", "text": "Implement the change."}]}
				],
				"max_completion_tokens": 512,
				"reasoning_effort": "medium",
				"parallel_tool_calls": true
			}`,
		},
		{
			name: "cline style tools",
			body: `{
				"model": "sonnet",
				"messages": [{"role": "user", "content": "Use a tool if needed."}],
				"tools": [{
					"type": "function",
					"function": {
						"name": "read_file",
						"description": "Read a workspace file",
						"parameters": {
							"type": "object",
							"properties": {"path": {"type": "string"}},
							"required": ["path"]
						}
					}
				}],
				"tool_choice": "auto",
				"parallel_tool_calls": true
			}`,
		},
		{
			name: "continue style",
			body: `{
				"model": "sonnet",
				"messages": [
					{"role": "system", "content": "Answer with concise code guidance."},
					{"role": "user", "content": "Explain this diagnostic."}
				],
				"max_tokens": 300,
				"response_format": {"type": "text"},
				"stream_options": {"include_usage": true},
				"metadata": {"client": "continue"}
			}`,
		},
		{
			name: "codex cli style",
			body: `{
				"model": "sonnet",
				"messages": [
					{"role": "developer", "content": "You are working inside a local repository."},
					{"role": "user", "content": "Inspect the failing test and propose a fix."}
				],
				"max_completion_tokens": 1024,
				"reasoning_effort": "medium",
				"parallel_tool_calls": true,
				"metadata": {"client": "codex-cli"}
			}`,
		},
		{
			name: "droidrun openailike style",
			body: `{
				"model": "sonnet",
				"messages": [
					{"role": "system", "content": "Control Android through concise steps."},
					{"role": "user", "content": "Open settings and report the current screen."}
				],
				"max_tokens": 512,
				"temperature": 0.2,
				"top_p": 0.95,
				"user": "droidrun-local"
			}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer local-key")
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"content":"client fixture ok"`) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}

func TestChatCompletionsRejectsStructuredOutputWithOpenAIError(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "sonnet",
		"messages": [{"role": "user", "content": "hello"}],
		"response_format": {"type": "json_schema", "json_schema": {"name": "answer", "schema": {"type": "object"}}}
	}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"error":`, `"type":"invalid_request_error"`, `"code":"unsupported_feature"`, "json_schema"} {
		if !strings.Contains(body, want) {
			t.Fatalf("error body missing %s: %s", want, body)
		}
	}
}

func TestResponsesAcceptsOpenAISDKStylePayload(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fixture","choices":[{"message":{"role":"assistant","content":"response fixture ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	srv := testServerWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model": "sonnet",
		"instructions": "Use repo style.",
		"input": [{"role": "user", "content": [{"type": "input_text", "text": "hello"}]}],
		"max_output_tokens": 128,
		"parallel_tool_calls": true,
		"metadata": {"client": "openai-sdk"},
		"truncation": "auto"
	}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"response fixture ok"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChatCompletionsAcceptsCodexAndDroidMetadataShapes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fixture","choices":[{"message":{"role":"assistant","content":"fixture ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	handler := testServerWithConfig(t, cfg).Routes()

	for _, body := range []string{
		`{"model":"sonnet","messages":[{"role":"user","content":"hello"}],"metadata":{"client":"codex-cli"}}`,
		`{"model":"sonnet","messages":[{"role":"system","content":"Control Android."},{"role":"user","content":"Open settings"}],"user":"droidrun-local"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer local-key")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}
}
