package setup

import "testing"

func TestPresetsIncludeCoreProviders(t *testing.T) {
	presets := Presets()
	wantIDs := map[string]bool{
		"openrouter":         false,
		"anthropic":          false,
		"gemini":             false,
		"openai-compatible":  false,
		"opencode-go":        false,
		"opencode-zen":       false,
		"custom":             false,
	}
	for _, preset := range presets {
		if _, ok := wantIDs[preset.ID]; ok {
			wantIDs[preset.ID] = true
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Fatalf("preset %q not found in %+v", id, presets)
		}
	}
}

func TestEnvNameForProvider(t *testing.T) {
	tests := []struct {
		providerID string
		want       string
	}{
		{providerID: "openrouter", want: "OPENROUTER_API_KEY"},
		{providerID: "OpenCode Go", want: "OPENCODE_GO_API_KEY"},
		{providerID: "my-provider.io", want: "MY_PROVIDER_IO_API_KEY"},
	}
	for _, tt := range tests {
		if got := EnvNameForProvider(tt.providerID); got != tt.want {
			t.Fatalf("EnvNameForProvider(%q) = %q, want %q", tt.providerID, got, tt.want)
		}
	}
}

func TestOpenCodeZenPresetMetadata(t *testing.T) {
	var got ProviderPreset
	found := false
	for _, preset := range Presets() {
		if preset.ID == "opencode-zen" {
			got = preset
			found = true
			break
		}
	}
	if !found {
		t.Fatal("opencode-zen preset not found")
	}
	if got.Name != "OpenCode Zen" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Type != "openai_compatible" {
		t.Fatalf("Type = %q", got.Type)
	}
	if got.BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("BaseURL = %q", got.BaseURL)
	}
	if got.DefaultModel != "kimi-k2.6" {
		t.Fatalf("DefaultModel = %q", got.DefaultModel)
	}
	if got.DefaultAlias != "opencode-zen-kimi" {
		t.Fatalf("DefaultAlias = %q", got.DefaultAlias)
	}
	if got.DefaultRoute != "sonnet" {
		t.Fatalf("DefaultRoute = %q", got.DefaultRoute)
	}
	if got.DiscoveryAlias != "claude-sonnet-4-20250514" {
		t.Fatalf("DiscoveryAlias = %q", got.DiscoveryAlias)
	}
}
