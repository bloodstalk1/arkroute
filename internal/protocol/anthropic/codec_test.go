package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeMessageRequestTextAndTools(t *testing.T) {
	body := []byte(`{
	  "model":"sonnet",
	  "max_tokens":1024,
	  "stream":true,
	  "system":"You are concise.",
	  "tools":[{"name":"read_file","description":"Read file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}],
	  "messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]
	}`)
	req, err := DecodeMessageRequest(body)
	if err != nil {
		t.Fatalf("DecodeMessageRequest() error = %v", err)
	}
	if req.Model != "sonnet" || !req.Stream || req.MaxTokens != 1024 {
		t.Fatalf("decoded request = %+v", req)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "read_file" {
		t.Fatalf("tools = %+v", req.Tools)
	}
}

func TestEncodeErrorShape(t *testing.T) {
	body, err := EncodeError("api_error", "upstream failed")
	if err != nil {
		t.Fatalf("EncodeError() error = %v", err)
	}
	if !strings.Contains(string(body), `"type":"error"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestModelsResponse(t *testing.T) {
	resp := ModelsResponseFor([]Model{{ID: "claude-sonnet-4-20250514", DisplayName: "Sonnet"}})
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), "claude-sonnet-4-20250514") {
		t.Fatalf("models response = %s", data)
	}
}
