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
		"X-OpenRouter-Title": "Arkroute",
		"Cookie":             "session=abc",
		"X-API-Key":          "sk-123",
	}
	got := RedactMap(headers)
	if got["Authorization"] != "[redacted]" {
		t.Fatalf("Authorization = %q, want redacted", got["Authorization"])
	}
	if got["X-OpenRouter-Title"] != "Arkroute" {
		t.Fatalf("X-OpenRouter-Title = %q", got["X-OpenRouter-Title"])
	}
	if got["Cookie"] != "[redacted]" {
		t.Fatalf("Cookie = %q, want redacted", got["Cookie"])
	}
	if got["X-API-Key"] != "[redacted]" {
		t.Fatalf("X-API-Key = %q, want redacted", got["X-API-Key"])
	}
}

func TestLooksSecretDistinguishesBenignKeys(t *testing.T) {
	benign := []string{
		"Accept-Encoding",
		"Sec-WebSocket-Key",
		"Key-Id",
		"User-Agent",
		"X-Request-Id",
		"X-Trace-Id",
		"Content-Type",
	}
	for _, key := range benign {
		if LooksSecret(key) {
			t.Errorf("LooksSecret(%q) = true, want false", key)
		}
	}
}

func TestLooksSecretCoversKnownSecretHeaders(t *testing.T) {
	secret := []string{
		"Authorization",
		"X-API-Key",
		"X-Goog-Api-Key",
		"Anthropic-Api-Key",
		"X-Auth-Token",
		"Cookie",
		"X-CSRF-Token",
		"X-Account-Token",
		"X-Client-Secret",
		"X-Session-Key",
		"Proxy-Authorization",
	}
	for _, key := range secret {
		if !LooksSecret(key) {
			t.Errorf("LooksSecret(%q) = false, want true", key)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"LOCALHOST", true},
		{" 127.0.0.1 ", true},
		{"0.0.0.0", false},
		{"8.8.8.8", false},
		{"example.com", false},
		{"", false},
		{"::", false},
	}
	for _, tc := range cases {
		if got := IsLoopbackHost(tc.host); got != tc.want {
			t.Errorf("IsLoopbackHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}
