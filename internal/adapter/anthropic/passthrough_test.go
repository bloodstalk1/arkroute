package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
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

func TestMessagesURLAcceptsBaseEndingAtV1(t *testing.T) {
	got, err := messagesURL("https://opencode.ai/zen/go/v1")
	if err != nil {
		t.Fatalf("messagesURL() error = %v", err)
	}
	if got != "https://opencode.ai/zen/go/v1/messages" {
		t.Fatalf("messagesURL() = %q, want opencode anthropic messages endpoint", got)
	}
}

func TestBuildRequestPreservesClaudeBlocksAndReasoningControls(t *testing.T) {
	adapter := Adapter{}
	provider := config.ProviderConfig{
		BaseURL: "https://opencode.ai/zen/go",
		APIKey:  "sk-ant-test",
		Headers: map[string]string{"x-test-header": "present"},
	}
	model := config.ModelConfig{UpstreamModel: "qwen3.7-max"}
	req := protocol.Request{
		MaxTokens:       128,
		Thinking:        protocol.ThinkingConfig{Type: "enabled", BudgetTokens: 1024},
		ReasoningEffort: "high",
		Messages: []protocol.Message{
			{
				Role: protocol.RoleAssistant,
				Content: []protocol.ContentBlock{
					{Type: "thinking", Thinking: "I should inspect the file.", Signature: "sig_1"},
					{Type: "tool_use", ID: "toolu_1", Name: "Read", Input: json.RawMessage(`{"file_path":"README.md"}`)},
				},
			},
			{
				Role: protocol.RoleUser,
				Content: []protocol.ContentBlock{{
					Type:      "tool_result",
					ToolUseID: "toolu_1",
					Content:   json.RawMessage(`"file contents"`),
				}},
			},
		},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.HasSuffix(out.URL, "/v1/messages") {
		t.Fatalf("url = %s", out.URL)
	}
	if out.Headers.Get("x-test-header") != "present" {
		t.Fatalf("custom header = %q", out.Headers.Get("x-test-header"))
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("body JSON error = %v; body = %s", err, out.Body)
	}
	if thinking, ok := body["thinking"].(map[string]any); !ok || thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(1024) {
		t.Fatalf("thinking = %#v", body["thinking"])
	}
	if output, ok := body["output_config"].(map[string]any); !ok || output["effort"] != "high" {
		t.Fatalf("output_config = %#v", body["output_config"])
	}
	messages := body["messages"].([]any)
	assistantContent := messages[0].(map[string]any)["content"].([]any)
	if assistantContent[0].(map[string]any)["type"] != "thinking" || assistantContent[0].(map[string]any)["signature"] != "sig_1" {
		t.Fatalf("assistant thinking block = %#v", assistantContent[0])
	}
	if assistantContent[1].(map[string]any)["type"] != "tool_use" || assistantContent[1].(map[string]any)["name"] != "Read" {
		t.Fatalf("assistant tool_use block = %#v", assistantContent[1])
	}
	userContent := messages[1].(map[string]any)["content"].([]any)
	if userContent[0].(map[string]any)["type"] != "tool_result" || userContent[0].(map[string]any)["tool_use_id"] != "toolu_1" {
		t.Fatalf("user tool_result block = %#v", userContent[0])
	}
}

func TestAnthropicAdapterSupportsStreamMapper(t *testing.T) {
	adapter := Adapter{}
	mapper, ok := adapter.NewStreamMapper()
	if !ok {
		t.Fatal("Anthropic adapter should support streaming for anthropic-messages providers")
	}
	events, err := mapper.MapLine([]byte(`data: {"type":"message_stop"}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "message_stop" {
		t.Fatalf("events = %+v, want message_stop", events)
	}
}

func TestAnthropicStreamMapperMapsThinkingToolUseAndStopReason(t *testing.T) {
	mapper, ok := Adapter{}.NewStreamMapper()
	if !ok {
		t.Fatal("Anthropic adapter should support stream mapper")
	}
	lines := [][]byte{
		[]byte(`event: message_start`),
		[]byte(`data: {"type":"message_start","message":{"id":"msg_1","model":"qwen3.7-max","usage":{"input_tokens":1,"output_tokens":0}}}`),
		[]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`),
		[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"I should inspect first."}}`),
		[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_1"}}`),
		[]byte(`data: {"type":"content_block_stop","index":0}`),
		[]byte(`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}}`),
		[]byte(`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\":\"README.md\"}"}}`),
		[]byte(`data: {"type":"content_block_stop","index":1}`),
		[]byte(`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":7}}`),
		[]byte(`data: {"type":"message_stop"}`),
	}
	var all []protocol.StreamEvent
	for _, line := range lines {
		events, err := mapper.MapLine(line)
		if err != nil {
			t.Fatalf("MapLine(%s) error = %v", line, err)
		}
		all = append(all, events...)
	}
	var sawStart, sawThinking, sawSignature, sawToolStart, sawToolInput, sawToolStop, sawMessageDelta, sawMessageStop bool
	for _, event := range all {
		switch event.Type {
		case "message_start":
			sawStart = true
		case "thinking_delta":
			sawThinking = event.Index == 0 && event.Delta == "I should inspect first."
		case "signature_delta":
			sawSignature = event.Index == 0 && event.Delta == "sig_1"
		case "content_block_start":
			if event.Index == 1 && event.Block.Type == "tool_use" && event.Block.ID == "toolu_1" && event.Block.Name == "Read" {
				sawToolStart = true
			}
		case "tool_input_delta":
			sawToolInput = event.Index == 1 && event.Delta == `{"file_path":"README.md"}`
		case "content_block_stop":
			if event.Index == 1 {
				sawToolStop = true
			}
		case "message_delta":
			sawMessageDelta = event.StopReason == "tool_use" && event.Usage.OutputTokens == 7
		case "message_stop":
			sawMessageStop = true
		}
	}
	if !sawStart || !sawThinking || !sawSignature || !sawToolStart || !sawToolInput || !sawToolStop || !sawMessageDelta || !sawMessageStop {
		t.Fatalf("events missing expected mapped pieces: %+v", all)
	}
}

func TestAnthropicStreamMapperMapsErrorPayload(t *testing.T) {
	mapper, ok := Adapter{}.NewStreamMapper()
	if !ok {
		t.Fatal("Anthropic adapter should support stream mapper")
	}
	events, err := mapper.MapLine([]byte(`data: {"type":"error","error":{"type":"model_error","message":"qwen unavailable"}}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "error" || !strings.Contains(events[0].Error, "qwen unavailable") {
		t.Fatalf("events = %+v, want error event with upstream message", events)
	}
}
