package modelfetcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestFetchOpenAICompatibleSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test" {
			t.Errorf("Authorization = %q, want Bearer sk-test", auth)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "gpt-4o", "object": "model", "owned_by": "openai"},
				{"id": "claude-3.5", "object": "model", "owned_by": "anthropic"},
				{"id": "my-custom-fine-tune", "object": "model", "owned_by": "user"},
			},
		})
	}))
	defer server.Close()

	got, err := Fetch(newCtx(t), Request{
		Provider: "openai",
		BaseURL:  server.URL + "/v1",
		APIKey:   "sk-test",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Source != "openai_compatible" {
		t.Errorf("Source = %q, want openai_compatible", got.Source)
	}
	if len(got.Models) != 3 {
		t.Fatalf("got %d models, want 3", len(got.Models))
	}
	if got.Models[0].ID != "gpt-4o" {
		t.Errorf("Models[0].ID = %q, want gpt-4o", got.Models[0].ID)
	}
	if got.Models[1].Label != "claude-3.5 (anthropic)" {
		t.Errorf("Models[1].Label = %q, want claude-3.5 (anthropic)", got.Models[1].Label)
	}
}

func TestFetchAnthropicUsesXApiKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Errorf("x-api-key = %q, want sk-ant-test", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("anthropic-version header missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-4-5", "display_name": "Claude Sonnet 4.5", "type": "model"},
				{"id": "claude-opus-4-1", "display_name": "Claude Opus 4.1", "type": "model"},
			},
		})
	}))
	defer server.Close()

	got, err := Fetch(newCtx(t), Request{
		Provider: "anthropic",
		BaseURL:  server.URL,
		APIKey:   "sk-ant-test",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Source != "anthropic" {
		t.Errorf("Source = %q, want anthropic", got.Source)
	}
	if len(got.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(got.Models))
	}
	if got.Models[0].Label != "Claude Sonnet 4.5 (claude-sonnet-4-5)" {
		t.Errorf("Models[0].Label = %q, want display + id", got.Models[0].Label)
	}
}

func TestFetchGeminiStripsModelsPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1beta/models") {
			t.Errorf("path = %q, want /v1beta/models", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "AIza-test" {
			t.Errorf("x-goog-api-key = %q, want AIza-test", r.Header.Get("x-goog-api-key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "models/gemini-2.5-pro", "displayName": "Gemini 2.5 Pro"},
				{"name": "models/gemini-2.5-flash", "displayName": "Gemini 2.5 Flash"},
			},
		})
	}))
	defer server.Close()

	got, err := Fetch(newCtx(t), Request{
		Provider: "gemini",
		BaseURL:  server.URL + "/v1beta",
		APIKey:   "AIza-test",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Source != "gemini" {
		t.Errorf("Source = %q, want gemini", got.Source)
	}
	if got.Models[0].ID != "gemini-2.5-pro" {
		t.Errorf("Models[0].ID = %q, want gemini-2.5-pro (stripped models/ prefix)", got.Models[0].ID)
	}
}

func TestFetchAuthFailureReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	_, err := Fetch(newCtx(t), Request{Provider: "openai", BaseURL: server.URL + "/v1", APIKey: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrAuthRequired) {
		t.Errorf("err = %v, want ErrAuthRequired", err)
	}
}

func TestFetchMissingBaseURL(t *testing.T) {
	_, err := Fetch(newCtx(t), Request{Provider: "openai"})
	if err == nil {
		t.Fatal("expected error for empty base_url")
	}
}

func TestFetchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()
	_, err := Fetch(newCtx(t), Request{Provider: "openai", BaseURL: server.URL, APIKey: "k"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestJoinURL(t *testing.T) {
	tests := []struct {
		base, want string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com/v1/models"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1/models"},
		{"https://api.openai.com", "https://api.openai.com/models"},
		{"http://localhost:1234/v1/", "http://localhost:1234/v1/models"},
	}
	for _, tc := range tests {
		got, err := joinURL(tc.base, "models")
		if err != nil {
			t.Errorf("joinURL(%q): %v", tc.base, err)
			continue
		}
		if got != tc.want {
			t.Errorf("joinURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestIsOpencodeGo(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://opencode.ai/zen/go", true},
		{"https://opencode.ai/zen/go/", true},
		{"http://opencode.ai/zen/go", true},
		{"https://opencode.ai/zen/v1", false},
		{"https://openai.com/zen/go", false},
	}
	for _, tc := range tests {
		got := isOpencodeGo(tc.url)
		if got != tc.want {
			t.Errorf("isOpencodeGo(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestFetchOpenAICompatibleURLRewrite(t *testing.T) {
	baseURL := "https://opencode.ai/zen/go"
	if isOpencodeGo(baseURL) {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}
	got, err := joinURL(baseURL, "models")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://opencode.ai/zen/go/v1/models"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
