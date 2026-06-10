package catalog

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", c.SchemaVersion)
	}
	if len(c.Providers) < 5 {
		t.Errorf("expected at least 5 providers, got %d", len(c.Providers))
	}
}

func TestGetKnownProvider(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"anthropic", "openai", "gemini", "openrouter", "groq", "ollama"} {
		p := c.Get(id)
		if p == nil {
			t.Errorf("Get(%q) returned nil", id)
			continue
		}
		if p.DefaultBaseURL == "" {
			t.Errorf("provider %q has empty default_base_url", id)
		}
		if len(p.Models) == 0 {
			t.Errorf("provider %q has no models", id)
		}
		hasDefault := false
		for _, m := range p.Models {
			if m.Default {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			t.Errorf("provider %q has no default model", id)
		}
	}
}

func TestGetCaseInsensitive(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Get("Anthropic") == nil {
		t.Error("Get(\"Anthropic\") returned nil; expected case-insensitive match")
	}
	if c.Get("  OPENAI  ") == nil {
		t.Error("Get with whitespace returned nil; expected trim")
	}
}

func TestGetUnknownReturnsNil(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Get("does-not-exist"); p != nil {
		t.Errorf("expected nil for unknown preset, got %+v", p)
	}
	if p := c.Get(""); p != nil {
		t.Errorf("expected nil for empty preset, got %+v", p)
	}
}

func TestIDsSorted(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ids := c.IDs()
	if len(ids) < 5 {
		t.Fatalf("got %d ids, want at least 5", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if strings.Compare(ids[i-1], ids[i]) > 0 {
			t.Errorf("IDs not sorted: %q > %q at %d", ids[i-1], ids[i], i)
		}
	}
}

func TestOllamaProviderIsLoopback(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ollama := c.Get("ollama")
	if ollama == nil {
		t.Fatal("ollama provider missing")
	}
	if !strings.HasPrefix(ollama.DefaultBaseURL, "http://") {
		t.Errorf("ollama default_base_url = %q, expected http:// prefix", ollama.DefaultBaseURL)
	}
	// The host portion should be a loopback address; we don't pin to 127.0.0.1
	// specifically because some operators prefer "localhost". Both are valid.
	host := strings.TrimPrefix(ollama.DefaultBaseURL, "http://")
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		t.Errorf("ollama host = %q, expected loopback address", host)
	}
}
