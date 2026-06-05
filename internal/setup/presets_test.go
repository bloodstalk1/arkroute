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
