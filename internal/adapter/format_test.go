package adapter

import (
	"encoding/json"
	"testing"
)

func TestFormatStreamError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty body", "", "upstream stream error"},
		{"null body", "null", "upstream stream error"},
		{"whitespace only", "   \n\t", "upstream stream error"},
		{"bare string", `"rate limit exceeded"`, "rate limit exceeded"},
		{"anthropic message", `{"type":"error","message":"bad request"}`, "bad request"},
		{"openai message", `{"type":"error","code":"rate_limit","message":"slow down"}`, "slow down"},
		{"fallback to type when no message", `{"type":"overloaded"}`, "overloaded"},
		{"fallback to code when no message/type", `{"code":"billing"}`, "billing"},
		{"invalid json returns raw", `{not json`, `{not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatStreamError(json.RawMessage(tt.raw)); got != tt.want {
				t.Fatalf("FormatStreamError(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
