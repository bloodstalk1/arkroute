package openai

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/protocol"
)

func TestNormalizeChatRequestMapsCommonOpenAIChatShapes(t *testing.T) {
	body := []byte(`{
		"model": "sonnet",
		"max_completion_tokens": 256,
		"temperature": 0.2,
		"reasoning_effort": "high",
		"messages": [
			{"role": "developer", "content": "Use terse output."},
			{"role": "system", "content": [{"type": "text", "text": "System note"}]},
			{"role": "user", "content": [{"type": "text", "text": "hello"}]},
			{
				"role": "assistant",
				"content": "I will call a tool",
				"tool_calls": [{
					"id": "call_1",
					"type": "function",
					"function": {"name": "read_file", "arguments": "{\"path\":\"README.md\"}"}
				}]
			},
			{"role": "tool", "tool_call_id": "call_1", "content": "file contents"}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "read_file",
				"description": "Read a file",
				"parameters": {"type": "object"}
			}
		}],
		"tool_choice": {"type": "function", "function": {"name": "read_file"}}
	}`)

	chatReq, err := DecodeChatRequest(body)
	if err != nil {
		t.Fatalf("DecodeChatRequest() error = %v", err)
	}
	normalized, requirements, err := NormalizeChatRequest(chatReq)
	if err != nil {
		t.Fatalf("NormalizeChatRequest() error = %v", err)
	}

	if normalized.Model != "sonnet" {
		t.Fatalf("model = %q, want sonnet", normalized.Model)
	}
	if normalized.MaxTokens != 256 {
		t.Fatalf("max tokens = %d, want 256", normalized.MaxTokens)
	}
	if normalized.Temperature == nil || *normalized.Temperature != 0.2 {
		t.Fatalf("temperature = %v, want 0.2", normalized.Temperature)
	}
	if normalized.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q, want high", normalized.ReasoningEffort)
	}
	if len(normalized.System) != 2 || normalized.System[0].Text != "Use terse output." || normalized.System[1].Text != "System note" {
		t.Fatalf("system blocks = %+v", normalized.System)
	}
	if len(normalized.Tools) != 1 || normalized.Tools[0].Name != "read_file" || normalized.Tools[0].Description != "Read a file" {
		t.Fatalf("tools = %+v", normalized.Tools)
	}
	if len(normalized.ToolChoice) == 0 {
		t.Fatal("tool choice was not preserved")
	}
	if !requirements.Tools {
		t.Fatal("requirements.Tools = false, want true")
	}
	if len(normalized.Messages) != 3 {
		t.Fatalf("messages = %+v, want 3 normalized conversation messages", normalized.Messages)
	}
	user := normalized.Messages[0]
	if user.Role != protocol.RoleUser || len(user.Content) != 1 || user.Content[0].Text != "hello" {
		t.Fatalf("user message = %+v", user)
	}
	assistant := normalized.Messages[1]
	if assistant.Role != protocol.RoleAssistant || len(assistant.Content) != 2 {
		t.Fatalf("assistant message = %+v", assistant)
	}
	if assistant.Content[0].Type != "text" || assistant.Content[0].Text != "I will call a tool" {
		t.Fatalf("assistant text block = %+v", assistant.Content[0])
	}
	if assistant.Content[1].Type != "tool_use" || assistant.Content[1].ID != "call_1" || assistant.Content[1].Name != "read_file" || string(assistant.Content[1].Input) != `{"path":"README.md"}` {
		t.Fatalf("assistant tool block = %+v", assistant.Content[1])
	}
	toolResult := normalized.Messages[2]
	if toolResult.Role != protocol.RoleUser || len(toolResult.Content) != 1 || toolResult.Content[0].Type != "tool_result" || toolResult.Content[0].ToolUseID != "call_1" {
		t.Fatalf("tool result message = %+v", toolResult)
	}
	if string(toolResult.Content[0].Content) != `"file contents"` {
		t.Fatalf("tool result content = %s", toolResult.Content[0].Content)
	}
}

func TestNormalizeChatRequestUsesMaxTokensWhenPresent(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{"model":"sonnet","max_tokens":64,"max_completion_tokens":256,"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest() error = %v", err)
	}
	normalized, _, err := NormalizeChatRequest(req)
	if err != nil {
		t.Fatalf("NormalizeChatRequest() error = %v", err)
	}
	if normalized.MaxTokens != 64 {
		t.Fatalf("max tokens = %d, want explicit max_tokens 64", normalized.MaxTokens)
	}
}

func TestNormalizeChatRequestRejectsUnsupportedMultipleChoices(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{"model":"sonnet","n":2,"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest() error = %v", err)
	}
	_, _, err = NormalizeChatRequest(req)
	if err == nil {
		t.Fatal("NormalizeChatRequest() error = nil, want unsupported n")
	}
	if !strings.Contains(err.Error(), "n > 1") {
		t.Fatalf("error = %q, want n > 1", err.Error())
	}
}
