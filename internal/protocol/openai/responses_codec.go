package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/router"
)

type ResponsesRequest struct {
	Model              string              `json:"model"`
	Input              any                 `json:"input"`
	Instructions       any                 `json:"instructions,omitempty"`
	Tools              []ResponsesTool     `json:"tools,omitempty"`
	ToolChoice         any                 `json:"tool_choice,omitempty"`
	MaxOutputTokens    int                 `json:"max_output_tokens,omitempty"`
	Temperature        *float64            `json:"temperature,omitempty"`
	Stream             bool                `json:"stream,omitempty"`
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	Store              *bool               `json:"store,omitempty"`
	Include            []string            `json:"include,omitempty"`
	Reasoning          *ResponsesReasoning `json:"reasoning,omitempty"`
	Text               *ResponsesText      `json:"text,omitempty"`
	Modalities         []string            `json:"modalities,omitempty"`
}

type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type ResponsesText struct {
	Format *ResponsesTextFormat `json:"format,omitempty"`
}

type ResponsesTextFormat struct {
	Type string `json:"type,omitempty"`
}

type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func DecodeResponsesRequest(body []byte) (ResponsesRequest, error) {
	var req ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ResponsesRequest{}, err
	}
	return req, nil
}

func NormalizeResponsesRequest(req ResponsesRequest) (protocol.Request, router.Requirements, error) {
	if req.PreviousResponseID != "" {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported Responses feature: previous_response_id")
	}
	if req.Store != nil && *req.Store {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported Responses feature: store")
	}
	for _, include := range req.Include {
		if strings.TrimSpace(include) != "" {
			return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported Responses feature: include")
		}
	}
	if err := validateTextOnlyModalities("Responses", req.Modalities); err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	if req.Text != nil && req.Text.Format != nil && req.Text.Format.Type != "" && req.Text.Format.Type != "text" {
		return protocol.Request{}, router.Requirements{}, fmt.Errorf("unsupported Responses text.format %q", req.Text.Format.Type)
	}
	normalized := protocol.Request{
		Model:       req.Model,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}
	if req.Reasoning != nil {
		normalized.ReasoningEffort = req.Reasoning.Effort
	}
	toolChoice, err := rawJSON(req.ToolChoice)
	if err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	normalized.ToolChoice = toolChoice
	tools, err := normalizeResponsesTools(req.Tools)
	if err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	normalized.Tools = tools
	instructions, err := responsesInstructions(req.Instructions)
	if err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	normalized.System = append(normalized.System, instructions...)
	messages, system, err := responsesInput(req.Input)
	if err != nil {
		return protocol.Request{}, router.Requirements{}, err
	}
	normalized.System = append(normalized.System, system...)
	normalized.Messages = messages
	requirements := router.Requirements{Streaming: req.Stream, Tools: len(normalized.Tools) > 0 || containsToolBlocks(normalized.Messages)}
	return normalized, requirements, nil
}

func normalizeResponsesTools(tools []ResponsesTool) ([]protocol.Tool, error) {
	out := make([]protocol.Tool, 0, len(tools))
	for _, tool := range tools {
		switch tool.Type {
		case "function":
			out = append(out, protocol.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Parameters})
		case "namespace":
			continue
		default:
			return nil, fmt.Errorf("unsupported Responses tool type %q", tool.Type)
		}
	}
	return out, nil
}

func responsesInstructions(value any) ([]protocol.ContentBlock, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		return []protocol.ContentBlock{{Type: "text", Text: typed}}, nil
	default:
		return nil, fmt.Errorf("unsupported Responses instructions shape")
	}
}

func responsesInput(value any) ([]protocol.Message, []protocol.ContentBlock, error) {
	if value == nil {
		return nil, nil, nil
	}
	switch typed := value.(type) {
	case string:
		return []protocol.Message{{Role: protocol.RoleUser, Content: []protocol.ContentBlock{{Type: "text", Text: typed}}}}, nil, nil
	case []any:
		messages := []protocol.Message{}
		system := []protocol.ContentBlock{}
		for _, item := range typed {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("Responses input items must be objects")
			}
			itemMessages, itemSystem, err := responsesInputItem(itemMap)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, itemMessages...)
			system = append(system, itemSystem...)
		}
		return messages, system, nil
	default:
		return nil, nil, fmt.Errorf("unsupported Responses input shape")
	}
}

func responsesInputItem(item map[string]any) ([]protocol.Message, []protocol.ContentBlock, error) {
	itemType, _ := item["type"].(string)
	role, _ := item["role"].(string)
	if itemType == "function_call_output" {
		callID, _ := item["call_id"].(string)
		output := item["output"]
		content, err := rawJSON(output)
		if err != nil {
			return nil, nil, err
		}
		return []protocol.Message{{
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: callID,
				Content:   content,
			}},
		}}, nil, nil
	}
	if role == "" {
		return nil, nil, fmt.Errorf("unsupported Responses input item type %q", itemType)
	}
	blocks, err := responsesContentBlocks(item["content"])
	if err != nil {
		return nil, nil, err
	}
	switch role {
	case "system", "developer":
		return nil, blocks, nil
	case "user":
		return []protocol.Message{{Role: protocol.RoleUser, Content: blocks}}, nil, nil
	case "assistant":
		return []protocol.Message{{Role: protocol.RoleAssistant, Content: blocks}}, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported Responses role %q", role)
	}
}

func responsesContentBlocks(value any) ([]protocol.ContentBlock, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil, nil
		}
		return []protocol.ContentBlock{{Type: "text", Text: typed}}, nil
	case []any:
		blocks := make([]protocol.ContentBlock, 0, len(typed))
		for _, item := range typed {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Responses content items must be objects")
			}
			itemType, _ := itemMap["type"].(string)
			switch itemType {
			case "input_text", "output_text", "text":
				text, _ := itemMap["text"].(string)
				if text != "" {
					blocks = append(blocks, protocol.ContentBlock{Type: "text", Text: text})
				}
			default:
				return nil, fmt.Errorf("unsupported Responses content type %q", itemType)
			}
		}
		return blocks, nil
	default:
		return nil, fmt.Errorf("unsupported Responses content shape")
	}
}
