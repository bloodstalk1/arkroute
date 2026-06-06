package openai

import "encoding/json"

type ChatRequest struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	N                   int             `json:"n,omitempty"`
	Logprobs            *bool           `json:"logprobs,omitempty"`
	TopLogprobs         *int            `json:"top_logprobs,omitempty"`
	Modalities          []string        `json:"modalities,omitempty"`
	Audio               any             `json:"audio,omitempty"`
	Thinking            *ThinkingConfig `json:"thinking,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
}

type ThinkingConfig struct {
	Type string `json:"type"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type Message struct {
	Role             string     `json:"role"`
	Content          any        `json:"content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
}

type ResponseMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content"`
	Reasoning        string            `json:"reasoning"`
	ReasoningDetails []ReasoningDetail `json:"reasoning_details"`
	ToolCalls        []ToolCall        `json:"tool_calls"`
}

type ReasoningDetail struct {
	Text      string `json:"text"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning"`
}

type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      ResponseMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
