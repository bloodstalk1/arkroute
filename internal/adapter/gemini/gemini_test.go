package gemini

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

func TestBuildRequest(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:     "gemini-pro",
		MaxTokens: 512,
		Messages:  []protocol.Message{{Role: protocol.RoleUser, Content: []protocol.ContentBlock{{Type: "text", Text: "hello"}}}},
	}
	provider := config.ProviderConfig{BaseURL: "https://generativelanguage.googleapis.com/v1beta", APIKey: "AIza-test"}
	model := config.ModelConfig{UpstreamModel: "gemini-2.5-pro"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(out.URL, "/models/gemini-2.5-pro:generateContent") {
		t.Fatalf("url = %s", out.URL)
	}
	if strings.Contains(out.URL, "key=") {
		t.Fatalf("api key must not be in URL: %s", out.URL)
	}
	if got := out.Headers.Get("x-goog-api-key"); got != "AIza-test" {
		t.Fatalf("x-goog-api-key = %q, want %q", got, "AIza-test")
	}
	if !strings.Contains(string(out.Body), `"text":"hello"`) {
		t.Fatalf("body = %s", out.Body)
	}
}

func TestGeminiAdapterAdvertisesStreamMapper(t *testing.T) {
	adapter := Adapter{}
	mapper, ok := adapter.NewStreamMapper()
	if !ok {
		t.Fatal("Gemini adapter should advertise streaming")
	}
	if mapper == nil {
		t.Fatal("NewStreamMapper() returned nil")
	}
}

func TestGeminiStreamMapperProducesDeltasFromAccumulatedStates(t *testing.T) {
	mapper := NewStreamMapper()

	// First chunk: "Hel" (accumulated)
	events, err := mapper.MapLine([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hel"}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("expected message_start + content_block_start, got %d events", len(events))
	}
	if events[0].Type != "message_start" {
		t.Errorf("event[0] = %q, want message_start", events[0].Type)
	}
	if events[1].Type != "content_block_start" {
		t.Errorf("event[1] = %q, want content_block_start", events[1].Type)
	}

	// Second chunk: "Hello" (accumulated) → delta should be "lo"
	events, err = mapper.MapLine([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	hasDelta := false
	for _, e := range events {
		if e.Type == "content_delta" && e.Delta == "lo" {
			hasDelta = true
		}
	}
	if !hasDelta {
		t.Errorf("expected content_delta with delta=lo, got %+v", events)
	}

	// Final chunk with finish reason and usage
	events, err = mapper.MapLine([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	hasStop := false
	hasUsage := false
	for _, e := range events {
		if e.Type == "content_block_stop" {
			hasStop = true
		}
		if e.Type == "message_delta" && e.StopReason == "end_turn" {
			hasStop = true
		}
		if e.Usage.InputTokens == 10 && e.Usage.OutputTokens == 5 {
			hasUsage = true
		}
	}
	if !hasStop {
		t.Errorf("expected content_block_stop + message_delta, got %+v", events)
	}
	if !hasUsage {
		t.Errorf("expected usage {input:10 output:5}, got %+v", events)
	}
}

func TestGeminiStreamMapperError(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`{"error":{"code":400,"message":"bad request"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "error" || events[0].Error != "bad request" {
		t.Errorf("expected error event, got %+v", events)
	}
}
