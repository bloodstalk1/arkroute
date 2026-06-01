package openai

import (
	"encoding/json"
	"net/http"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
	oai "bat.dev/arkrouter/internal/protocol/openai"
)

type Adapter struct{}

func (a Adapter) BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (adapter.UpstreamRequest, error) {
	url, err := ChatCompletionsURL(provider.BaseURL)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	body := oai.ChatRequest{
		Model:       model.UpstreamModel,
		Messages:    mapMessages(req.Messages),
		Tools:       mapTools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+provider.APIKey)
	headers.Set("Content-Type", "application/json")
	for key, value := range provider.Headers {
		headers.Set(key, value)
	}
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: url, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded oai.ChatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return protocol.Response{}, err
	}
	resp := protocol.Response{ID: decoded.ID, Role: protocol.RoleAssistant}
	if len(decoded.Choices) > 0 {
		choice := decoded.Choices[0]
		resp.StopReason = choice.FinishReason
		if choice.Message.Content != "" {
			resp.Content = append(resp.Content, protocol.ContentBlock{Type: "text", Text: choice.Message.Content})
		}
		for _, call := range choice.Message.ToolCalls {
			resp.Content = append(resp.Content, protocol.ContentBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: json.RawMessage(call.Function.Arguments)})
		}
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens}
	return resp, nil
}

func mapMessages(messages []protocol.Message) []oai.Message {
	out := make([]oai.Message, 0, len(messages))
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == "text" {
				out = append(out, oai.Message{Role: string(msg.Role), Content: block.Text})
			}
			if block.Type == "tool_result" {
				out = append(out, oai.Message{Role: "tool", ToolCallID: block.ToolUseID, Content: string(block.Content)})
			}
		}
	}
	return out
}

func mapTools(tools []protocol.Tool) []oai.Tool {
	out := make([]oai.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, oai.Tool{Type: "function", Function: oai.FunctionDef{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema}})
	}
	return out
}

func (a Adapter) NewStreamMapper() (adapter.StreamMapper, bool) {
	return NewStreamMapper(), true
}

func (a Adapter) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ClassifyStatus(status)
}
