package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/router"
)

func DecodeChatRequest(body []byte) (ChatRequest, error) {
	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ChatRequest{}, err
	}
	return req, nil
}

func NormalizeChatRequest(req ChatRequest) (protocol.Request, router.Requirements, error) {
	if req.N > 1 {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported OpenAI chat feature: n > 1")
	}
	if req.Logprobs != nil && *req.Logprobs {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported OpenAI chat feature: logprobs")
	}
	if req.TopLogprobs != nil {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported OpenAI chat feature: top_logprobs")
	}
	if err := validateTextOnlyModalities("OpenAI chat", req.Modalities); err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	if req.Audio != nil {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported OpenAI chat feature: audio")
	}
	if req.ResponseFormat != nil && req.ResponseFormat.Type != "" && req.ResponseFormat.Type != "text" {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported OpenAI chat response_format %q", req.ResponseFormat.Type)
	}
	normalized := protocol.Request{
		Model:           req.Model,
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		Stream:          req.Stream,
		ReasoningEffort: req.ReasoningEffort,
	}
	if normalized.MaxTokens == 0 {
		normalized.MaxTokens = req.MaxCompletionTokens
	}
	toolChoice, err := rawJSON(req.ToolChoice)
	if err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	normalized.ToolChoice = toolChoice
	tools, err := normalizeChatTools(req.Tools)
	if err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	normalized.Tools = tools
	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system", "developer":
			blocks, err := textContentBlocks(msg.Content)
			if err != nil {
				return protocol.Request{}, router.Requirements{}, fmt.Errorf("%s message: %w", role, err)
			}
			normalized.System = append(normalized.System, blocks...)
		case "user":
			blocks, err := textContentBlocks(msg.Content)
			if err != nil {
				return protocol.Request{}, router.Requirements{}, fmt.Errorf("user message: %w", err)
			}
			normalized.Messages = append(normalized.Messages, protocol.Message{Role: protocol.RoleUser, Content: blocks})
		case "assistant":
			blocks, err := assistantContentBlocks(msg)
			if err != nil {
				return protocol.Request{}, router.Requirements{}, err
			}
			normalized.Messages = append(normalized.Messages, protocol.Message{Role: protocol.RoleAssistant, Content: blocks})
		case "tool":
			content, err := rawJSON(msg.Content)
			if err != nil {
				return protocol.Request{}, router.Requirements{}, err
			}
			normalized.Messages = append(normalized.Messages, protocol.Message{
				Role: protocol.RoleUser,
				Content: []protocol.ContentBlock{{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   content,
				}},
			})
		default:
			return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported OpenAI chat role %q", msg.Role)
		}
	}
	requirements := router.Requirements{Streaming: req.Stream, Tools: len(normalized.Tools) > 0 || containsToolBlocks(normalized.Messages)}
	return normalized, requirements, nil
}

func normalizeChatTools(tools []Tool) ([]protocol.Tool, error) {
	out := make([]protocol.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			return nil, fmt.Errorf("unsupported OpenAI chat tool type %q", tool.Type)
		}
		out = append(out, protocol.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	return out, nil
}

func assistantContentBlocks(msg Message) ([]protocol.ContentBlock, error) {
	blocks, err := textContentBlocks(msg.Content)
	if err != nil {
		return nil, fmt.Errorf("assistant message: %w", err)
	}
	if msg.ReasoningContent != "" {
		blocks = append(blocks, protocol.ContentBlock{Type: "thinking", Thinking: msg.ReasoningContent})
	}
	for _, call := range msg.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			return nil, fmt.Errorf("unsupported OpenAI chat tool call type %q", call.Type)
		}
		blocks = append(blocks, protocol.ContentBlock{
			Type:  "tool_use",
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: json.RawMessage(call.Function.Arguments),
		})
	}
	return blocks, nil
}

func textContentBlocks(content any) ([]protocol.ContentBlock, error) {
	if content == nil {
		return nil, nil
	}
	switch value := content.(type) {
	case string:
		if value == "" {
			return nil, nil
		}
		return []protocol.ContentBlock{{Type: "text", Text: value}}, nil
	case []any:
		blocks := make([]protocol.ContentBlock, 0, len(value))
		for _, item := range value {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("content array items must be objects")
			}
			itemType, _ := itemMap["type"].(string)
			switch itemType {
			case "text", "input_text":
				text, _ := itemMap["text"].(string)
				if text != "" {
					blocks = append(blocks, protocol.ContentBlock{Type: "text", Text: text})
				}
			default:
				return nil, fmt.Errorf("unsupported content part type %q", itemType)
			}
		}
		return blocks, nil
	default:
		return nil, fmt.Errorf("content must be a string or text content array")
	}
}

func rawJSON(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func containsToolBlocks(messages []protocol.Message) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == "tool_use" || block.Type == "tool_result" {
				return true
			}
		}
	}
	return false
}

func validateTextOnlyModalities(scope string, modalities []string) error {
	for _, modality := range modalities {
		normalized := strings.ToLower(strings.TrimSpace(modality))
		if normalized != "" && normalized != "text" {
			return fmt.Errorf("unsupported %s feature: modalities %q", scope, modality)
		}
	}
	return nil
}
