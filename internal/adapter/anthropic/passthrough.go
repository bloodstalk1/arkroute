package anthropic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
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
	if len(req.Tools) > 0 {
		body["tools"] = mapTools(req.Tools)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("x-api-key", provider.APIKey)
	headers.Set("anthropic-version", "2023-06-01")
	headers.Set("Content-Type", "application/json")
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: endpoint, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Role    string `json:"role"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
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
		resp.Content = append(resp.Content, protocol.ContentBlock{Type: block.Type, Text: block.Text, ID: block.ID, Name: block.Name, Input: block.Input})
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
	if !strings.HasSuffix(path, "/v1/messages") {
		path += "/v1/messages"
	}
	parsed.Path = path
	return parsed.String(), nil
}

func mapMessages(messages []protocol.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		content := make([]map[string]any, 0, len(msg.Content))
		for _, block := range msg.Content {
			if block.Type == "text" {
				content = append(content, map[string]any{"type": "text", "text": block.Text})
			}
		}
		out = append(out, map[string]any{"role": string(msg.Role), "content": content})
	}
	return out
}

func mapTools(tools []protocol.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
	}
	return out
}

func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return nil, false
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
