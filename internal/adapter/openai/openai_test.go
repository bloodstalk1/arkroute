package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

func TestChatCompletionsURL(t *testing.T) {
	for _, baseURL := range []string{
		"https://openrouter.ai/api/v1",
		"https://example.test",
		"https://example.test/v1/",
	} {
		got, err := ChatCompletionsURL(baseURL)
		if err != nil {
			t.Fatalf("ChatCompletionsURL(%q) error = %v", baseURL, err)
		}
		if strings.Contains(got, "/v1/v1/") {
			t.Fatalf("url duplicated v1: %s", got)
		}
		if !strings.HasSuffix(got, "/chat/completions") {
			t.Fatalf("url = %s, want chat completions suffix", got)
		}
	}
}

func TestBuildRequestMapsTextAndTools(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:     "sonnet",
		MaxTokens: 512,
		Tools: []protocol.Tool{{
			Name:        "read_file",
			Description: "Read file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "hello"}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "anthropic/claude-sonnet-4.5"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if out.Method != "POST" || !strings.HasSuffix(out.URL, "/chat/completions") {
		t.Fatalf("upstream request = %+v", out)
	}
	if out.Headers.Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", out.Headers.Get("Authorization"))
	}
	if !strings.Contains(string(out.Body), `"model":"anthropic/claude-sonnet-4.5"`) {
		t.Fatalf("body = %s", out.Body)
	}
	if !strings.Contains(string(out.Body), `"tools"`) {
		t.Fatalf("body missing tools = %s", out.Body)
	}
}

func TestMapResponse(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if resp.Content[0].Text != "hello" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("response = %+v", resp)
	}
}
