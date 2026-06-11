// Package tokens provides simple character-based token estimation.
//
// This is an approximation (~4 chars per token for English text, ~3 for code).
// It is intentionally lightweight — no tiktoken or sentencepiece dependency.
// Callers that receive actual usage counts from upstream APIs should prefer
// those real numbers over these estimates.
//
// Estimation heuristic:
//
//	english  ~ 4 chars per token
//	code     ~ 3 chars per token
//	general  ~ 3.5 chars per token (default)
package tokens

import "strings"

// Estimate returns the estimated token count for a given string.
// The heuristic averages 3.5 characters per token, which is a reasonable
// approximation for English + code mixed prompts.
func Estimate(text string) int {
	if text == "" {
		return 0
	}
	return max(1, (len(text)+1)/3)
}

// RequestEstimate estimates the total input tokens for a tokenRequest by
// summing all message content, system instructions, tool definitions,
// and tool choice JSON. It does not count overhead framing (roles, JSON
// structure) that providers add — real usage is always slightly higher.
func RequestEstimate(req TokenRequest) int {
	var total strings.Builder
	total.WriteString(strings.Join(req.SystemTexts, "\n"))
	for _, msg := range req.Messages {
		total.WriteString(msg.Content)
	}
	for _, tool := range req.Tools {
		total.WriteString(tool.Definition)
	}
	return Estimate(total.String())
}

// TokenRequest holds the fields needed for token estimation. It is a subset of
// protocol.Request with only the text content we can estimate.
type TokenRequest struct {
	SystemTexts []string
	Messages    []Message
	Tools       []Tool
}

// Message is one turn in the conversation.
type Message struct {
	Content string // all text content blocks concatenated
}

// Tool is one tool definition.
type Tool struct {
	Definition string // JSON schema as a string
}

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
