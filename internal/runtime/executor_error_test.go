package runtime

import (
	"strings"
	"testing"
)

func TestFormatUpstreamError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "json with message",
			status: 502,
			body:   `{"error":{"message":"bad gateway"}}`,
			want:   "upstream returned 502: bad gateway",
		},
		{
			name:   "json with top-level message",
			status: 500,
			body:   `{"message":"server on fire"}`,
			want:   "upstream returned 500: server on fire",
		},
		{
			name:   "empty body",
			status: 503,
			body:   "",
			want:   "upstream returned 503",
		},
		{
			name:   "whitespace body",
			status: 504,
			body:   "   \n",
			want:   "upstream returned 504",
		},
		{
			name:   "plain text body",
			status: 500,
			body:   "totally unstructured error",
			want:   "upstream returned 500: totally unstructured error",
		},
		{
			name:   "array body with message",
			status: 422,
			body:   `[{"message":"first"}]`,
			want:   "upstream returned 422: first",
		},
		{
			name:   "long message truncated with ellipsis",
			status: 500,
			body:   `{"message":"` + strings.Repeat("x", 800) + `"}`,
			want:   "upstream returned 500: " + strings.Repeat("x", 500) + "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUpstreamError(tt.status, []byte(tt.body)); got != tt.want {
				t.Fatalf("formatUpstreamError(%d, %q) = %q, want %q", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestFindMessageField(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"empty map", map[string]any{}, ""},
		{"string map", map[string]any{"message": "hi"}, "hi"},
		{"whitespace message ignored", map[string]any{"message": "   "}, ""},
		{"nested in object", map[string]any{"error": map[string]any{"message": "deep"}}, "deep"},
		{"array of objects", []any{map[string]any{"message": "first"}}, "first"},
		{"no message anywhere", map[string]any{"code": "x"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findMessageField(tt.in); got != tt.want {
				t.Fatalf("findMessageField(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
