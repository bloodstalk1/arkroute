package anthropic

import "encoding/json"

type MessageRequest struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	Messages     []Message       `json:"messages"`
	System       json.RawMessage `json:"system,omitempty"`
	Tools        []Tool          `json:"tools,omitempty"`
	ToolChoice   json.RawMessage `json:"tool_choice,omitempty"`
	Temperature  *float64        `json:"temperature,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
	Thinking     *ThinkingConfig `json:"thinking,omitempty"`
	OutputConfig *OutputConfig   `json:"output_config,omitempty"`
}

type ThinkingConfig struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type OutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}
