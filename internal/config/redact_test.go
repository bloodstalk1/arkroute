package config

import "testing"

func TestRedactedHidesSecrets(t *testing.T) {
	cfg := MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-test"
	redacted := Redacted(cfg)
	if redacted.Server.ClientKey != "[redacted]" {
		t.Fatalf("client key = %q", redacted.Server.ClientKey)
	}
	if redacted.Providers[0].APIKey != "[redacted]" {
		t.Fatalf("api key = %q", redacted.Providers[0].APIKey)
	}
}
