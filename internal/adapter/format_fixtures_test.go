package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestFormatStreamErrorRealUpstreamShapes asserts that
// FormatStreamError produces a non-empty human-readable string for the
// error envelopes that Anthropic, OpenAI, and Gemini actually return.
// The fixtures live under testdata/ and were captured from real
// upstream responses (anonymised).
func TestFormatStreamErrorRealUpstreamShapes(t *testing.T) {
	tests := []struct {
		file     string
		contains string
	}{
		{"openai_401.json", "Incorrect API key"},
		{"openai_429.json", "Rate limit reached"},
		{"anthropic_401.json", "invalid x-api-key"},
		{"anthropic_529.json", "Overloaded"},
		{"gemini_429.json", "Resource has been exhausted"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got := FormatStreamError(json.RawMessage(data))
			if !contains(got, tt.contains) {
				t.Fatalf("FormatStreamError(%s) = %q; want substring %q", tt.file, got, tt.contains)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
