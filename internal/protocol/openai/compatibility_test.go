package openai

import (
	"strings"
	"testing"
)

func TestNormalizeChatRequestAcceptsCommonSDKOptionalFields(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "sonnet",
		"messages": [
			{"role": "developer", "content": "Follow repo style."},
			{"role": "user", "content": [{"type": "text", "text": "hello"}]}
		],
		"max_completion_tokens": 128,
		"stream": false,
		"parallel_tool_calls": true,
		"stream_options": {"include_usage": true},
		"response_format": {"type": "text"},
		"metadata": {"client": "openai-sdk"},
		"user": "local-user",
		"top_p": 0.9,
		"presence_penalty": 0,
		"frequency_penalty": 0
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest() error = %v", err)
	}
	normalized, _, err := NormalizeChatRequest(req)
	if err != nil {
		t.Fatalf("NormalizeChatRequest() error = %v", err)
	}
	if normalized.Model != "sonnet" || normalized.MaxTokens != 128 {
		t.Fatalf("normalized request = %+v", normalized)
	}
	if len(normalized.System) != 1 || normalized.System[0].Text != "Follow repo style." {
		t.Fatalf("system = %+v", normalized.System)
	}
	if len(normalized.Messages) != 1 || normalized.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("messages = %+v", normalized.Messages)
	}
}

func TestNormalizeChatRequestAcceptsInputTextContentAlias(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "sonnet",
		"messages": [
			{"role": "user", "content": [{"type": "input_text", "text": "hello"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest() error = %v", err)
	}
	normalized, _, err := NormalizeChatRequest(req)
	if err != nil {
		t.Fatalf("NormalizeChatRequest() error = %v", err)
	}
	if len(normalized.Messages) != 1 || normalized.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("messages = %+v", normalized.Messages)
	}
}

func TestNormalizeChatRequestRejectsStructuredResponseFormatClearly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "json object",
			body: `{"model":"sonnet","response_format":{"type":"json_object"},"messages":[{"role":"user","content":"hello"}]}`,
			want: "json_object",
		},
		{
			name: "json schema",
			body: `{"model":"sonnet","response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object"}}},"messages":[{"role":"user","content":"hello"}]}`,
			want: "json_schema",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := DecodeChatRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("DecodeChatRequest() error = %v", err)
			}
			_, _, err = NormalizeChatRequest(req)
			if err == nil {
				t.Fatal("NormalizeChatRequest() error = nil, want unsupported response_format")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNormalizeChatRequestRejectsMultimodalContentClearly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "image_url",
			body: `{"model":"sonnet","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`,
			want: "image_url",
		},
		{
			name: "input_audio",
			body: `{"model":"sonnet","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"abc","format":"wav"}}]}]}`,
			want: "input_audio",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := DecodeChatRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("DecodeChatRequest() error = %v", err)
			}
			_, _, err = NormalizeChatRequest(req)
			if err == nil {
				t.Fatal("NormalizeChatRequest() error = nil, want unsupported content")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNormalizeChatRequestRejectsDangerousOutputOptionsClearly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "logprobs",
			body: `{"model":"sonnet","messages":[{"role":"user","content":"hello"}],"logprobs":true}`,
			want: "logprobs",
		},
		{
			name: "top logprobs",
			body: `{"model":"sonnet","messages":[{"role":"user","content":"hello"}],"top_logprobs":3}`,
			want: "top_logprobs",
		},
		{
			name: "audio modality",
			body: `{"model":"sonnet","messages":[{"role":"user","content":"hello"}],"modalities":["text","audio"],"audio":{"voice":"alloy","format":"mp3"}}`,
			want: "modalities",
		},
		{
			name: "audio config",
			body: `{"model":"sonnet","messages":[{"role":"user","content":"hello"}],"audio":{"voice":"alloy","format":"mp3"}}`,
			want: "audio",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := DecodeChatRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("DecodeChatRequest() error = %v", err)
			}
			_, _, err = NormalizeChatRequest(req)
			if err == nil {
				t.Fatal("NormalizeChatRequest() error = nil, want unsupported output option")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNormalizeResponsesRequestAcceptsCommonSDKOptionalFields(t *testing.T) {
	req, err := DecodeResponsesRequest([]byte(`{
		"model": "sonnet",
		"input": [{"role": "user", "content": [{"type": "input_text", "text": "hello"}]}],
		"instructions": "Be concise.",
		"max_output_tokens": 128,
		"parallel_tool_calls": true,
		"metadata": {"client": "openai-sdk"},
		"user": "local-user",
		"truncation": "auto"
	}`))
	if err != nil {
		t.Fatalf("DecodeResponsesRequest() error = %v", err)
	}
	normalized, _, err := NormalizeResponsesRequest(req)
	if err != nil {
		t.Fatalf("NormalizeResponsesRequest() error = %v", err)
	}
	if normalized.Model != "sonnet" || normalized.MaxTokens != 128 {
		t.Fatalf("normalized request = %+v", normalized)
	}
	if len(normalized.System) != 1 || normalized.System[0].Text != "Be concise." {
		t.Fatalf("system = %+v", normalized.System)
	}
	if len(normalized.Messages) != 1 || normalized.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("messages = %+v", normalized.Messages)
	}
}

func TestNormalizeResponsesRequestRejectsDangerousOutputOptionsClearly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "json schema text format",
			body: `{"model":"sonnet","input":"hello","text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object"}}}}`,
			want: "json_schema",
		},
		{
			name: "audio modality",
			body: `{"model":"sonnet","input":"hello","modalities":["text","audio"]}`,
			want: "modalities",
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
				t.Fatal("NormalizeResponsesRequest() error = nil, want unsupported output option")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNormalizeResponsesRequestRejectsHostedAndMultimodalContentClearly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "input_image",
			body: `{"model":"sonnet","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,abc"}]}]}`,
			want: "input_image",
		},
		{
			name: "file_search",
			body: `{"model":"sonnet","input":"hello","tools":[{"type":"file_search"}]}`,
			want: "file_search",
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
				t.Fatal("NormalizeResponsesRequest() error = nil, want unsupported content")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}
