package setup

import (
	"strings"
	"unicode"

	"github.com/bloodstalk1/arkroute/internal/config"
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
			BaseURL: "https://api.anthropic.com", DefaultModel: "claude-sonnet-4-5",
			DefaultAlias: "sonnet-anthropic", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "gemini", Name: "Google Gemini", Type: "gemini",
			BaseURL: "https://generativelanguage.googleapis.com/v1beta", DefaultModel: "gemini-2.5-pro",
			DefaultAlias: "gemini-pro", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "openai", Name: "OpenAI", Type: "openai_compatible",
			BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o",
			DefaultAlias: "openai-gpt4o", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "openai-compatible", Name: "OpenAI-compatible", Type: "openai_compatible",
			BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-5.1",
			DefaultAlias: "openai-model", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "deepseek", Name: "DeepSeek", Type: "openai_compatible",
			BaseURL: "https://api.deepseek.com/v1", DefaultModel: "deepseek-chat",
			DefaultAlias: "deepseek", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "groq", Name: "Groq", Type: "openai_compatible",
			BaseURL: "https://api.groq.com/openai/v1", DefaultModel: "llama-3.3-70b-versatile",
			DefaultAlias: "groq-llama", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "mistral", Name: "Mistral AI", Type: "openai_compatible",
			BaseURL: "https://api.mistral.ai/v1", DefaultModel: "mistral-large-latest",
			DefaultAlias: "mistral-large", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "together", Name: "Together AI", Type: "openai_compatible",
			BaseURL: "https://api.together.xyz/v1", DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			DefaultAlias: "together-llama", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "fireworks", Name: "Fireworks AI", Type: "openai_compatible",
			BaseURL: "https://api.fireworks.ai/inference/v1", DefaultModel: "accounts/fireworks/models/llama-v3p3-70b-instruct",
			DefaultAlias: "fireworks-llama", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "cohere", Name: "Cohere", Type: "openai_compatible",
			BaseURL: "https://api.cohere.ai/compatibility/v1", DefaultModel: "command-r-plus",
			DefaultAlias: "cohere-rplus", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "xai", Name: "xAI (Grok)", Type: "openai_compatible",
			BaseURL: "https://api.x.ai/v1", DefaultModel: "grok-2",
			DefaultAlias: "xai-grok", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "nvidia", Name: "NVIDIA NIM", Type: "openai_compatible",
			BaseURL: "https://integrate.api.nvidia.com/v1", DefaultModel: "meta/llama-3.3-70b-instruct",
			DefaultAlias: "nvidia-llama", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "cerebras", Name: "Cerebras", Type: "openai_compatible",
			BaseURL: "https://api.cerebras.ai/v1", DefaultModel: "llama-3.3-70b",
			DefaultAlias: "cerebras-llama", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "qwen", Name: "Qwen (Alibaba)", Type: "openai_compatible",
			BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", DefaultModel: "qwen-plus",
			DefaultAlias: "qwen-plus", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "kimi", Name: "Kimi (Moonshot)", Type: "openai_compatible",
			BaseURL: "https://api.moonshot.ai/v1", DefaultModel: "moonshot-v1-128k",
			DefaultAlias: "kimi-128k", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "glm", Name: "GLM (Zhipu)", Type: "openai_compatible",
			BaseURL: "https://open.bigmodel.cn/api/paas/v4", DefaultModel: "glm-4.6",
			DefaultAlias: "glm-46", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "opencode-go", Name: "OpenCode Go", Type: "auto",
			BaseURL: "https://opencode.ai/zen/go", DefaultModel: "qwen3.7-max",
			DefaultAlias: "qwen37", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "opencode-zen", Name: "OpenCode Zen", Type: "openai_compatible",
			BaseURL: "https://opencode.ai/zen/v1", DefaultModel: "kimi-k2.6",
			DefaultAlias: "opencode-zen-kimi", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "ollama", Name: "Ollama (Local)", Type: "openai_compatible",
			BaseURL: "http://127.0.0.1:11434/v1", DefaultModel: "llama3.2",
			DefaultAlias: "ollama-local", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "lm-studio", Name: "LM Studio (Local)", Type: "openai_compatible",
			BaseURL: "http://127.0.0.1:1234/v1", DefaultModel: "loaded-model",
			DefaultAlias: "lmstudio-local", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "vllm", Name: "vLLM (Local)", Type: "openai_compatible",
			BaseURL: "http://127.0.0.1:8000/v1", DefaultModel: "served-model",
			DefaultAlias: "vllm-local", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
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
