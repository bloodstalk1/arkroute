package setup

import (
	"strings"
	"unicode"

	"github.com/bloodstalk1/arkroute/internal/config"
)

const (
	APIKeyModeEnv    = "env"
	APIKeyModeConfig = "config"
)

type ProviderPreset struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Type           string              `json:"type"`
	BaseURL        string              `json:"base_url"`
	DefaultModel   string              `json:"default_model"`
	DefaultAlias   string              `json:"default_alias"`
	DefaultRoute   string              `json:"default_route"`
	Headers        map[string]string   `json:"headers,omitempty"`
	Capabilities   config.Capabilities `json:"capabilities"`
	DiscoveryAlias string              `json:"claude_discovery_alias"`
}

func Presets() []ProviderPreset {
	return []ProviderPreset{
		{
			ID: "openrouter", Name: "OpenRouter", Type: "openai_compatible",
			BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "anthropic/claude-sonnet-4.5",
			DefaultAlias: "sonnet-or", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Headers: map[string]string{"X-OpenRouter-Title": "Arkroute"},
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "anthropic", Name: "Anthropic", Type: "anthropic",
			BaseURL: "https://api.anthropic.com", DefaultModel: "claude-sonnet-4-20250514",
			DefaultAlias: "sonnet-anthropic", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "gemini", Name: "Gemini", Type: "gemini",
			BaseURL: "https://generativelanguage.googleapis.com/v1beta", DefaultModel: "gemini-2.5-pro",
			DefaultAlias: "gemini-pro", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "openai-compatible", Name: "OpenAI-compatible", Type: "openai_compatible",
			BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-5.1",
			DefaultAlias: "openai-model", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "opencode-go", Name: "OpenCode Go", Type: "auto",
			BaseURL: "https://opencode.ai/zen/go", DefaultModel: "qwen3.7-max",
			DefaultAlias: "qwen37", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "custom", Name: "Custom", Type: "auto",
			BaseURL: "https://example.com/v1", DefaultModel: "provider/model",
			DefaultAlias: "custom-model", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
	}
}

func EnvNameForProvider(providerID string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range providerID {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "PROVIDER"
	}
	return out + "_API_KEY"
}

func defaultClaudeCapabilities() config.Capabilities {
	return config.Capabilities{
		Streaming:       true,
		Tools:           true,
		ToolResults:     true,
		SystemMessages:  true,
		ContextWindow:   200000,
		MaxOutputTokens: 8192,
	}
}
