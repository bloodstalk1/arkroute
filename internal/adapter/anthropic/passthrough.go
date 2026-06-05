package anthropic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/adapter"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

type Adapter struct{}

func (a Adapter) BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (adapter.UpstreamRequest, error) {
	endpoint, err := messagesURL(provider.BaseURL)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	body := map[string]any{
		"model":      model.UpstreamModel,
		"max_tokens": req.MaxTokens,
		"messages":   mapMessages(req.Messages),
		"stream":     req.Stream,
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if len(req.System) > 0 {
		body["system"] = mapSystem(req.System)
	}
	if len(req.ToolChoice) > 0 {
		var tc any
		if err := json.Unmarshal(req.ToolChoice, &tc); err == nil {
			body["tool_choice"] = tc
		}
	}
	if len(req.Tools) > 0 {
		body["tools"] = mapTools(req.Tools)
	}
	if req.Thinking.Type != "" {
		thinking := map[string]any{"type": req.Thinking.Type}
		if req.Thinking.BudgetTokens > 0 {
			thinking["budget_tokens"] = req.Thinking.BudgetTokens
		}
		body["thinking"] = thinking
	}
	if req.ReasoningEffort != "" {
		body["output_config"] = map[string]any{"effort": req.ReasoningEffort}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("x-api-key", provider.APIKey)
	headers.Set("anthropic-version", "2023-06-01")
	headers.Set("Content-Type", "application/json")
	for key, value := range provider.Headers {
		headers.Set(key, value)
	}
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: endpoint, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Role    string `json:"role"`
		Content []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			Thinking  string          `json:"thinking"`
			Signature string          `json:"signature"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return protocol.Response{}, err
	}
	resp := protocol.Response{ID: decoded.ID, Model: decoded.Model, Role: protocol.RoleAssistant, StopReason: decoded.StopReason}
	for _, block := range decoded.Content {
		resp.Content = append(resp.Content, protocol.ContentBlock{
			Type:      block.Type,
			Text:      block.Text,
			ID:        block.ID,
			Name:      block.Name,
			Input:     block.Input,
			ToolUseID: block.ToolUseID,
			Content:   block.Content,
			Thinking:  block.Thinking,
			Signature: block.Signature,
		})
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens}
	return resp, nil
}

func messagesURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/messages"):
	case strings.HasSuffix(path, "/v1"):
		path += "/messages"
	default:
		path += "/v1/messages"
	}
	parsed.Path = path
	return parsed.String(), nil
}

func mapMessages(messages []protocol.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		content := mapContentBlocks(msg.Content)
		out = append(out, map[string]any{"role": string(msg.Role), "content": content})
	}
	return out
}

func mapContentBlocks(blocks []protocol.ContentBlock) []map[string]any {
	content := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				content = append(content, map[string]any{"type": "text", "text": block.Text})
			}
		case "thinking":
			thinking := block.Thinking
			if thinking == "" {
				thinking = block.Text
			}
			if thinking == "" {
				continue
			}
			mapped := map[string]any{"type": "thinking", "thinking": thinking}
			if block.Signature != "" {
				mapped["signature"] = block.Signature
			}
			content = append(content, mapped)
		case "tool_use":
			mapped := map[string]any{
				"type":  "tool_use",
				"id":    block.ID,
				"name":  block.Name,
				"input": decodeRawJSON(block.Input, map[string]any{}),
			}
			content = append(content, mapped)
		case "tool_result":
			mapped := map[string]any{
				"type":        "tool_result",
				"tool_use_id": block.ToolUseID,
				"content":     decodeRawJSON(block.Content, ""),
			}
			content = append(content, mapped)
		}
	}
	return content
}

func decodeRawJSON(raw json.RawMessage, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return decoded
	}
	return string(raw)
}

func mapTools(tools []protocol.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
	}
	return out
}

func mapSystem(system []protocol.ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(system))
	for _, block := range system {
		if block.Type == "text" && block.Text != "" {
			out = append(out, map[string]any{"type": "text", "text": block.Text})
		}
	}
	return out
}

func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return NewStreamMapper(), true
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
