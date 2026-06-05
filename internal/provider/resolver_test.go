package provider

import "testing"

func TestResolverHonorsExplicitProviderConfig(t *testing.T) {
	resolver := DefaultResolver()
	provider := ProviderRef{ID: "opencode-go", Type: TypeOpenAICompatible, BaseURL: "https://opencode.ai/zen/go"}
	model := ModelRef{UpstreamModel: "qwen3.7-max"}
	if got := resolver.Resolve(provider, model); got != TypeOpenAICompatible {
		t.Fatalf("Resolve() = %q, want explicit openai_compatible", got)
	}
}

func TestResolverModelProtocolOverridesProviderType(t *testing.T) {
	resolver := DefaultResolver()
	provider := ProviderRef{ID: "opencode-go", Type: TypeOpenAICompatible, BaseURL: "https://opencode.ai/zen/go"}
	model := ModelRef{Protocol: TypeAnthropic, UpstreamModel: "custom-model"}
	if got := resolver.Resolve(provider, model); got != TypeAnthropic {
		t.Fatalf("Resolve() = %q, want model protocol override", got)
	}
}

func TestResolverAutoDetectsOpenCodeGoModels(t *testing.T) {
	resolver := DefaultResolver()
	provider := ProviderRef{ID: "opencode-go", Type: TypeAuto, BaseURL: "https://opencode.ai/zen/go"}
	tests := []struct {
		model string
		want  string
	}{
		{model: "qwen3.7-max", want: TypeAnthropic},
		{model: "qwen3.6-plus", want: TypeAnthropic},
		{model: "minimax-m3", want: TypeAnthropic},
		{model: "minimax-m2.7", want: TypeAnthropic},
		{model: "mimax-m3", want: TypeAnthropic},
		{model: "deepseek-v4-pro", want: TypeOpenAICompatible},
		{model: "glm-5.1", want: TypeOpenAICompatible},
		{model: "kimi-k2.6", want: TypeOpenAICompatible},
	}
	for _, tt := range tests {
		if got := resolver.Resolve(provider, ModelRef{UpstreamModel: tt.model}); got != tt.want {
			t.Fatalf("Resolve(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestResolverAutoDetectsEndpointShape(t *testing.T) {
	resolver := DefaultResolver()
	tests := []struct {
		provider ProviderRef
		want     string
	}{
		{provider: ProviderRef{BaseURL: "https://example.test/v1/messages"}, want: TypeAnthropic},
		{provider: ProviderRef{BaseURL: "https://example.test/v1/chat/completions"}, want: TypeOpenAICompatible},
		{provider: ProviderRef{BaseURL: "https://generativelanguage.googleapis.com/v1beta"}, want: TypeGemini},
		{provider: ProviderRef{ID: "anthropic", BaseURL: "https://api.anthropic.com"}, want: TypeAnthropic},
	}
	for _, tt := range tests {
		if got := resolver.Resolve(tt.provider, ModelRef{}); got != tt.want {
			t.Fatalf("Resolve(%+v) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}
