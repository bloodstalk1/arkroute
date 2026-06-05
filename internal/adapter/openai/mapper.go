package openai

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/adapter"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	oai "github.com/bloodstalk1/arkroute/internal/protocol/openai"
)

type Adapter struct{}

func (a Adapter) BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (adapter.UpstreamRequest, error) {
	url, err := ChatCompletionsURL(provider.BaseURL)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	reasoning := resolveReasoning(provider, model, req)
	body := oai.ChatRequest{
		Model:       model.UpstreamModel,
		Messages:    mapMessages(req.System, req.Messages, reasoning.Replay),
		Tools:       mapTools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}
	if reasoning.Enabled {
		body.Thinking = &oai.ThinkingConfig{Type: "enabled"}
		body.ReasoningEffort = reasoning.Effort
	} else if reasoning.DisableRequest {
		body.Thinking = &oai.ThinkingConfig{Type: "disabled"}
	}
	if !reasoning.OmitToolChoice && len(req.ToolChoice) > 0 {
		body.ToolChoice = mapToolChoice(req.ToolChoice)
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
		resp.StopReason = mapFinishReason(choice.FinishReason)
		reasoning := reasoningFromResponseMessage(choice.Message)
		content := choice.Message.Content
		if inlineThinking, rest, ok := splitInlineThinking(content); ok {
			if reasoning == "" {
				reasoning = inlineThinking
			}
			content = rest
		}
		if toolBlock, ok := parsePseudoToolUse(content); ok {
			resp.Content = append(resp.Content, toolBlock)
			resp.StopReason = "tool_use"
		} else if content != "" {
			resp.Content = append(resp.Content, protocol.ContentBlock{Type: "text", Text: content})
		}
		for _, call := range choice.Message.ToolCalls {
			call = normalizeToolCall(call)
			resp.Content = append(resp.Content, protocol.ContentBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: json.RawMessage(call.Function.Arguments)})
		}
		rememberReasoningForToolCalls(reasoning, choice.Message.ToolCalls)
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens}
	return resp, nil
}

func reasoningFromResponseMessage(message oai.ResponseMessage) string {
	return reasoningFromFields(message.ReasoningContent, message.Reasoning, message.ReasoningDetails)
}

func reasoningFromFields(reasoningContent string, reasoning string, details []oai.ReasoningDetail) string {
	if reasoningContent != "" {
		return reasoningContent
	}
	if reasoning != "" {
		return reasoning
	}
	if len(details) == 0 {
		return ""
	}
	var parts []string
	for _, detail := range details {
		switch {
		case detail.Text != "":
			parts = append(parts, detail.Text)
		case detail.Content != "":
			parts = append(parts, detail.Content)
		case detail.Reasoning != "":
			parts = append(parts, detail.Reasoning)
		}
	}
	return strings.Join(parts, "")
}

func mapMessages(system []protocol.ContentBlock, messages []protocol.Message, replayReasoning bool) []oai.Message {
	out := make([]oai.Message, 0, len(system)+len(messages))
	for _, block := range system {
		if block.Type == "text" {
			out = append(out, oai.Message{Role: "system", Content: block.Text})
		}
	}
	for _, msg := range messages {
		switch msg.Role {
		case protocol.RoleAssistant:
			oaiMsg := oai.Message{Role: "assistant"}
			var textParts []string
			var thinkingParts []string
			for _, block := range msg.Content {
				switch block.Type {
				case "thinking":
					if block.Thinking != "" {
						thinkingParts = append(thinkingParts, block.Thinking)
					} else if block.Text != "" {
						thinkingParts = append(thinkingParts, block.Text)
					}
				case "text":
					if block.Text != "" {
						textParts = append(textParts, block.Text)
					}
				case "tool_use":
					oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, oai.ToolCall{
						ID:   block.ID,
						Type: "function",
						Function: oai.FunctionCall{
							Name:      block.Name,
							Arguments: string(block.Input),
						},
					})
				}
			}
			if replayReasoning {
				if len(thinkingParts) == 1 {
					oaiMsg.ReasoningContent = thinkingParts[0]
				} else if len(thinkingParts) > 1 {
					oaiMsg.ReasoningContent = strings.Join(thinkingParts, "\n")
				}
				if oaiMsg.ReasoningContent == "" {
					oaiMsg.ReasoningContent = lookupReasoningForToolCalls(oaiMsg.ToolCalls)
				}
			}
			if len(textParts) == 1 {
				oaiMsg.Content = textParts[0]
			} else if len(textParts) > 1 {
				oaiMsg.Content = strings.Join(textParts, "\n")
			} else if len(oaiMsg.ToolCalls) > 0 {
				oaiMsg.Content = ""
			}
			out = append(out, oaiMsg)
		default:
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					out = append(out, oai.Message{Role: "user", Content: block.Text})
				case "tool_result":
					out = append(out, oai.Message{Role: "tool", ToolCallID: block.ToolUseID, Content: extractToolContent(block.Content)})
				}
			}
		}
	}
	return out
}

func mapToolChoice(raw json.RawMessage) any {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "any", "tool":
			return "required"
		default:
			return s
		}
	}
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		switch obj.Type {
		case "auto":
			return "auto"
		case "any":
			return "required"
		case "tool":
			return map[string]any{"type": "function", "function": map[string]string{"name": obj.Name}}
		}
	}
	return raw
}

func extractToolContent(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var texts []string
		for _, b := range blocks {
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return string(raw)
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
