// Package protocol defines the provider-agnostic request/response
// types that arkroute uses internally. Adapters translate each
// upstream's wire format into these types on the way in, and back
// again on the way out. See the adapter packages for provider-specific
// codecs.
package protocol

import "encoding/json"

// Role is the speaker of a [Message]. The values are deliberately
// lowercase to match both Anthropic and OpenAI wire formats.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// Request is the normalised form of a chat completion request handed
// to the runtime. Providers may not support every field; adapters map
// what they support and ignore the rest.
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

// ThinkingConfig controls extended-thinking behaviour. BudgetTokens is
// the soft budget the model is told to stay under; 0 means "no
// budget enforced".
type ThinkingConfig struct {
	Type         string
	BudgetTokens int
}

// Message is a single turn in the conversation. The role's content
// blocks are ordered exactly as the caller sent them.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// ContentBlock is the union of all block types arkroute understands:
// plain text, tool use, tool result, thinking, and (indirectly via
// Input) image/document parts. The Type field discriminates.
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

// Tool is a tool definition offered to the model. InputSchema is the
// raw JSON schema for the tool's input; adapters re-encode it into the
// provider's preferred shape.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// Response is the normalised form of a chat completion response. The
// stream variant produces a sequence of [StreamEvent] values that
// accumulate into the same shape.
type Response struct {
	ID         string
	Model      string
	Role       Role
	Content    []ContentBlock
	StopReason string
	Usage      Usage
	Metadata   map[string]string
}

// Usage is the billing-relevant token count for a request/response.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// StreamEvent is one delta from a streaming upstream. The Type
// discriminator is the same string used by the Anthropic wire format
// ("message_start", "content_block_delta", "message_stop", etc.) so
// downstream code can switch on it directly.
type StreamEvent struct {
	Type       string
	Index      int
	Delta      string
	Block      ContentBlock
	Usage      Usage
	Error      string
	StopReason string
}

// Capabilities describe what a model can do. The router uses these to
// filter route targets against the caller's [router.Requirements].
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
