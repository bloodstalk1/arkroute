package security

import "testing"

func TestRedactSecret(t *testing.T) {
	got := Redact("sk-or-secret")
	if got != "[redacted]" {
		t.Fatalf("Redact() = %q, want [redacted]", got)
	}
}

func TestRedactMap(t *testing.T) {
	headers := map[string]string{
		"Authorization":      "Bearer secret",
		"X-OpenRouter-Title": "Arkrouter",
	}
	got := RedactMap(headers)
	if got["Authorization"] != "[redacted]" {
		t.Fatalf("Authorization = %q, want redacted", got["Authorization"])
	}
	if got["X-OpenRouter-Title"] != "Arkrouter" {
		t.Fatalf("X-OpenRouter-Title = %q", got["X-OpenRouter-Title"])
	}
}
