package adapter

import (
	"encoding/json"
	"strings"
)

// FormatStreamError extracts a human-readable message from a streaming
// error body. The body may be either a bare JSON string, an object
// with {message, type, code} (OpenAI style), or an envelope that
// nests the message inside an "error" sub-object (Anthropic style:
// {type: "error", error: {type: ..., message: ...}}). Empty or null
// bodies return "upstream stream error" so callers always get a
// non-empty string to surface to the client.
func FormatStreamError(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "upstream stream error"
	}
	if msg, ok := extractMessage(raw); ok {
		return msg
	}
	return string(raw)
}

// extractMessage searches the (possibly nested) JSON for the first
// non-empty "message" or "error.message" string. It also falls back
// to "type" and "code" when no message is present.
func extractMessage(raw json.RawMessage) (string, bool) {
	var message string
	if err := json.Unmarshal(raw, &message); err == nil && message != "" {
		return message, true
	}
	var decoded struct {
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Code    string          `json:"code"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", false
	}
	if decoded.Message != "" {
		return decoded.Message, true
	}
	if len(decoded.Error) > 0 {
		if msg, ok := extractMessage(decoded.Error); ok {
			return msg, true
		}
	}
	if decoded.Type != "" && decoded.Type != "error" {
		return decoded.Type, true
	}
	if decoded.Code != "" {
		return decoded.Code, true
	}
	return "", false
}
