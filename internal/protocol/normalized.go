package protocol

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type Request struct {
	Model           string
	System          []ContentBlock
	Messages        []Message
	Tools           []Tool
	ToolChoice      json.RawMessage
	MaxTokens       int
	Temperature     *float64
	Stream          bool
	Thinking        ThinkingConfig
	ReasoningEffort string
	Metadata        map[string]string
}

type ThinkingConfig struct {
	Type         string
	BudgetTokens int
}

type Message struct {
	Role    Role
	Content []ContentBlock
}

type ContentBlock struct {
	Type      string
	Text      string
	ID        string
	Name      string
	Input     json.RawMessage
	ToolUseID string
	Content   json.RawMessage
	Thinking  string
	Signature string
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type Response struct {
	ID         string
	Model      string
	Role       Role
	Content    []ContentBlock
	StopReason string
	Usage      Usage
	Metadata   map[string]string
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type StreamEvent struct {
	Type       string
	Index      int
	Delta      string
	Block      ContentBlock
	Usage      Usage
	Error      string
	StopReason string
}

type Capabilities struct {
	Streaming       bool
	Tools           bool
	ToolResults     bool
	Vision          bool
	SystemMessages  bool
	PromptCache     bool
	ContextWindow   int
	MaxOutputTokens int
}
