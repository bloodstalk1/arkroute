package anthropic

import (
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

func TestBuildRequest(t *testing.T) {
	adapter := Adapter{}
	provider := config.ProviderConfig{BaseURL: "https://api.anthropic.com", APIKey: "sk-ant-test"}
	model := config.ModelConfig{UpstreamModel: "claude-sonnet-4-20250514"}
	out, err := adapter.BuildRequest(protocol.Request{Model: "sonnet", MaxTokens: 128, Messages: []protocol.Message{{Role: protocol.RoleUser, Content: []protocol.ContentBlock{{Type: "text", Text: "hi"}}}}}, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.HasSuffix(out.URL, "/v1/messages") {
		t.Fatalf("url = %s", out.URL)
	}
	if out.Headers.Get("x-api-key") != "sk-ant-test" {
		t.Fatalf("x-api-key = %q", out.Headers.Get("x-api-key"))
	}
	if !strings.Contains(string(out.Body), `"model":"claude-sonnet-4-20250514"`) {
		t.Fatalf("body = %s", out.Body)
	}
}
