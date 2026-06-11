package gemini

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
	endpoint, err := geminiURL(provider.BaseURL, model.UpstreamModel, req.Stream)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	genConfig := map[string]any{"maxOutputTokens": req.MaxTokens}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	body := map[string]any{
		"contents":         mapMessages(req.Messages),
		"generationConfig": genConfig,
	}
	if len(req.System) > 0 {
		body["systemInstruction"] = mapSystemInstruction(req.System)
	}
	if len(req.Tools) > 0 {
		body["tools"] = []any{map[string]any{"functionDeclarations": mapTools(req.Tools)}}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("x-goog-api-key", provider.APIKey)
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: endpoint, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		Usage struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return protocol.Response{}, err
	}
	resp := protocol.Response{Role: protocol.RoleAssistant}
	if len(decoded.Candidates) > 0 {
		candidate := decoded.Candidates[0]
		resp.StopReason = strings.ToLower(candidate.FinishReason)
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				resp.Content = append(resp.Content, protocol.ContentBlock{Type: "text", Text: part.Text})
			}
		}
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.PromptTokenCount, OutputTokens: decoded.Usage.CandidatesTokenCount}
	return resp, nil
}

func geminiURL(baseURL string, model string, stream bool) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	method := "generateContent"
	if stream {
		method = "streamGenerateContent"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/models/" + url.PathEscape(model) + ":" + method
	return parsed.String(), nil
}

func mapMessages(messages []protocol.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := "user"
		if msg.Role == protocol.RoleAssistant {
			role = "model"
		}
		parts := []map[string]string{}
		for _, block := range msg.Content {
			if block.Type == "text" && block.Text != "" {
				parts = append(parts, map[string]string{"text": block.Text})
			}
		}
		out = append(out, map[string]any{"role": role, "parts": parts})
	}
	return out
}

func mapTools(tools []protocol.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		var parameters any = map[string]any{"type": "object"}
		if len(tool.InputSchema) > 0 {
			_ = json.Unmarshal(tool.InputSchema, &parameters)
		}
		out = append(out, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  parameters,
		})
	}
	return out
}

func mapSystemInstruction(system []protocol.ContentBlock) map[string]any {
	parts := make([]map[string]string, 0, len(system))
	for _, block := range system {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, map[string]string{"text": block.Text})
		}
	}
	return map[string]any{"parts": parts}
}

func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return NewStreamMapper(), true
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
