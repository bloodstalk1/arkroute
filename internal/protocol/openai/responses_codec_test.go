package openai

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/protocol"
)

func TestNormalizeResponsesRequestMapsStringInput(t *testing.T) {
	req, err := DecodeResponsesRequest([]byte(`{
		"model": "sonnet",
		"input": "hello",
		"instructions": "Use terse output.",
		"max_output_tokens": 128,
		"temperature": 0.3,
		"stream": true
	}`))
	if err != nil {
		t.Fatalf("DecodeResponsesRequest() error = %v", err)
	}
	normalized, requirements, err := NormalizeResponsesRequest(req)
	if err != nil {
		t.Fatalf("NormalizeResponsesRequest() error = %v", err)
	}
	if normalized.Model != "sonnet" {
		t.Fatalf("model = %q, want sonnet", normalized.Model)
	}
	if normalized.MaxTokens != 128 {
		t.Fatalf("max tokens = %d, want 128", normalized.MaxTokens)
	}
	if normalized.Temperature == nil || *normalized.Temperature != 0.3 {
		t.Fatalf("temperature = %v, want 0.3", normalized.Temperature)
	}
	if !requirements.Streaming || normalized.Stream != true {
		t.Fatalf("stream requirement/request = %v/%v, want true/true", requirements.Streaming, normalized.Stream)
	}
	if len(normalized.System) != 1 || normalized.System[0].Text != "Use terse output." {
		t.Fatalf("system = %+v", normalized.System)
	}
	if len(normalized.Messages) != 1 || normalized.Messages[0].Role != protocol.RoleUser || normalized.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("messages = %+v", normalized.Messages)
	}
}

func TestNormalizeResponsesRequestMapsMessageItemsAndFunctionTools(t *testing.T) {
	req, err := DecodeResponsesRequest([]byte(`{
		"model": "sonnet",
		"input": [
			{"role": "developer", "content": "Follow project style."},
			{"role": "user", "content": [{"type": "input_text", "text": "read file"}]},
			{"type": "function_call_output", "call_id": "call_1", "output": "file contents"}
		],
		"tools": [{
			"type": "function",
			"name": "read_file",
			"description": "Read a file",
			"parameters": {"type": "object"}
		}]
	}`))
	if err != nil {
		t.Fatalf("DecodeResponsesRequest() error = %v", err)
	}
	normalized, requirements, err := NormalizeResponsesRequest(req)
	if err != nil {
		t.Fatalf("NormalizeResponsesRequest() error = %v", err)
	}
	if len(normalized.System) != 1 || normalized.System[0].Text != "Follow project style." {
		t.Fatalf("system = %+v", normalized.System)
	}
	if len(normalized.Tools) != 1 || normalized.Tools[0].Name != "read_file" || normalized.Tools[0].Description != "Read a file" {
		t.Fatalf("tools = %+v", normalized.Tools)
	}
	if !requirements.Tools {
		t.Fatal("requirements.Tools = false, want true")
	}
	if len(normalized.Messages) != 2 {
		t.Fatalf("messages = %+v, want 2", normalized.Messages)
	}
	if normalized.Messages[0].Content[0].Text != "read file" {
		t.Fatalf("user message = %+v", normalized.Messages[0])
	}
	if normalized.Messages[1].Content[0].Type != "tool_result" || normalized.Messages[1].Content[0].ToolUseID != "call_1" {
		t.Fatalf("function output message = %+v", normalized.Messages[1])
	}
}

func TestNormalizeResponsesRequestRejectsHostedStateAndTools(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "previous response",
			body: `{"model":"sonnet","input":"hello","previous_response_id":"resp_old"}`,
			want: "previous_response_id",
		},
		{
			name: "stored response",
			body: `{"model":"sonnet","input":"hello","store":true}`,
			want: "store",
		},
		{
			name: "web search",
			body: `{"model":"sonnet","input":"hello","tools":[{"type":"web_search_preview"}]}`,
			want: "web_search_preview",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := DecodeResponsesRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("DecodeResponsesRequest() error = %v", err)
			}
			_, _, err = NormalizeResponsesRequest(req)
			if err == nil {
				t.Fatal("NormalizeResponsesRequest() error = nil, want unsupported feature")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}
