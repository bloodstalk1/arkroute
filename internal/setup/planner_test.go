package setup

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestApplyProviderSetupStoresEnvReference(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:       "openrouter",
		APIKeyMode:     APIKeyModeEnv,
		APIKey:         "sk-or-secret",
		EnvName:        "OPENROUTER_API_KEY",
		UpstreamModel:  "anthropic/claude-sonnet-4.5",
		ExposedAlias:   "sonnet-or",
		RouteAlias:     "sonnet",
		ActivateClaude: true,
	})
	if err != nil {
		t.Fatalf("ApplyProviderSetup() error = %v", err)
	}
	if out.Providers[0].APIKey != "env:OPENROUTER_API_KEY" {
		t.Fatalf("provider api key = %q", out.Providers[0].APIKey)
	}
	if out.Models[0].ProviderID != "openrouter" || out.Routes[0].Alias != "sonnet" {
		t.Fatalf("unexpected config: %+v", out)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupCanStoreRawConfigSecret(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    APIKeyModeConfig,
		APIKey:        "sk-ant-secret",
		UpstreamModel: "claude-sonnet-4-20250514",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("ApplyProviderSetup() error = %v", err)
	}
	if out.Providers[0].APIKey != "sk-ant-secret" {
		t.Fatalf("provider api key = %q", out.Providers[0].APIKey)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupRejectsUnknownPreset(t *testing.T) {
	_, err := ApplyProviderSetup(config.BootstrapLocalConfig("local-key"), ProviderSetup{PresetID: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unknown preset") {
		t.Fatalf("error = %v, want unknown preset", err)
	}
}
