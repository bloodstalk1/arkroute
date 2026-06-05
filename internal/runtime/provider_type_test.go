package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/router"
)

func TestExecutorAutoDetectsAnthropicAdapterForOpenCodeGoQwen(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want /v1/messages", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"msg_1","model":"qwen3.7-max","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].ID = "opencode-go"
	cfg.Providers[0].Type = ""
	cfg.Providers[0].BaseURL = upstream.URL
	cfg.Providers[0].APIKey = "sk-test"
	cfg.Models[0].ProviderID = "opencode-go"
	cfg.Models[0].UpstreamModel = "qwen3.7-max"
	sink := newMemorySink()
	executor := executorFromConfig(t, cfg, sink)

	result, err := executor.Execute(context.Background(), ExecuteRequest{
		RequestID:    "req",
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
	if len(result.Response.Content) != 1 || result.Response.Content[0].Text != "ok" {
		t.Fatalf("response = %+v", result.Response)
	}
	var sawResolvedProviderType bool
	for _, event := range sink.events {
		if event.Event == observability.EventTargetSelected && event.ProviderType == "anthropic" {
			sawResolvedProviderType = true
		}
	}
	if !sawResolvedProviderType {
		t.Fatalf("trace events missing resolved anthropic provider_type: %+v", sink.events)
	}
}
