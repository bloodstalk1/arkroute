package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestResponsesNonStreamingStringInput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"sonnet","input":"ping","max_output_tokens":128}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"object":"response"`, `"model":"sonnet"`, `"status":"completed"`, `"output_text":"pong"`, `"type":"message"`, `"type":"output_text"`, `"input_tokens":4`, `"output_tokens":2`, `"total_tokens":6`} {
		if !strings.Contains(body, want) {
			t.Fatalf("Responses body missing %s: %s", want, body)
		}
	}
}

func TestResponsesRejectsPreviousResponseIDWithOpenAIErrorShape(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"sonnet","input":"ping","previous_response_id":"resp_old"}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"error":`, `"type":"invalid_request_error"`, `"code":"unsupported_feature"`, "previous_response_id"} {
		if !strings.Contains(body, want) {
			t.Fatalf("error response missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"type":"error"`) {
		t.Fatalf("body used Anthropic error shape: %s", body)
	}
}

func TestResponsesStreamingText(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"sonnet","input":"ping","stream":true}`))
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
	for _, want := range []string{"event: response.created", `"type":"response.created"`, "event: response.output_text.delta", `"delta":"hi"`, "event: response.output_item.done", `"content":[{"type":"output_text","text":"hi","annotations":[]}]`, "event: response.completed", `"status":"completed"`, `"output_text":"hi"`, `"output":[{"id":"msg_`} {
		if !strings.Contains(body, want) {
			t.Fatalf("Responses stream missing %s: %s", want, body)
		}
	}
}
