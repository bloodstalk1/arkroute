package gemini

import (
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
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
	if !strings.Contains(out.URL, "key=AIza-test") {
		t.Fatalf("url missing key: %s", out.URL)
	}
	if !strings.Contains(string(out.Body), `"text":"hello"`) {
		t.Fatalf("body = %s", out.Body)
	}
}
