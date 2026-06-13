package adapter

import (
	"encoding/json"
	"strings"
)

// FormatStreamError extracts a human-readable message from a streaming
// error body. The body may be either a bare JSON string, an object with
// {message, type, code} (OpenAI-style), or {message, type} (Anthropic-style).
// Empty or null bodies return "upstream stream error" so callers always
// get a non-empty string to surface to the client.
func FormatStreamError(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "upstream stream error"
	}
	var message string
	if err := json.Unmarshal(raw, &message); err == nil && message != "" {
		return message
	}
	var decoded struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(raw, &decoded); err == nil {
		if decoded.Message != "" {
			return decoded.Message
		}
		if decoded.Type != "" {
			return decoded.Type
		}
		if decoded.Code != "" {
			return decoded.Code
		}
	}
	return string(raw)
}
