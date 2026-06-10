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
	if out.Models[0].ID != "openrouter-sonnet-or" {
		t.Fatalf("unexpected model ID = %q, want openrouter-sonnet-or", out.Models[0].ID)
	}
	if out.Routes[0].Targets[0].ModelID != "openrouter-sonnet-or" {
		t.Fatalf("unexpected target model ID = %q, want openrouter-sonnet-or", out.Routes[0].Targets[0].ModelID)
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

func TestApplyProviderSetupBuildsOpenCodeZenConfig(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "opencode-zen",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENCODE_API_KEY",
		UpstreamModel: "kimi-k2.6",
		ExposedAlias:  "opencode-zen-kimi",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("ApplyProviderSetup() error = %v", err)
	}
	if out.Providers[0].ID != "opencode-zen" {
		t.Fatalf("provider ID = %q", out.Providers[0].ID)
	}
	if out.Providers[0].Type != "openai_compatible" {
		t.Fatalf("provider type = %q", out.Providers[0].Type)
	}
	if out.Providers[0].BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("provider base URL = %q", out.Providers[0].BaseURL)
	}
	if out.Providers[0].APIKey != "env:OPENCODE_API_KEY" {
		t.Fatalf("provider API key = %q", out.Providers[0].APIKey)
	}
	if out.Models[0].UpstreamModel != "kimi-k2.6" || out.Models[0].ExposedAlias != "opencode-zen-kimi" {
		t.Fatalf("unexpected model config: %+v", out.Models[0])
	}
	if out.Routes[0].Alias != "sonnet" || out.Routes[0].Targets[0].ModelID != "opencode-zen-kimi" {
		t.Fatalf("unexpected route config: %+v", out.Routes[0])
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupPreservesExistingProvidersAndAddsRouteTarget(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	cfg, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "ANTHROPIC_API_KEY",
		UpstreamModel: "claude-sonnet-4-5",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("first ApplyProviderSetup() error = %v", err)
	}

	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("second ApplyProviderSetup() error = %v", err)
	}

	if got, want := len(cfg.Providers), 2; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if got, want := len(cfg.Models), 2; got != want {
		t.Fatalf("models = %d, want %d: %+v", got, want, cfg.Models)
	}
	if got, want := len(cfg.Routes), 1; got != want {
		t.Fatalf("routes = %d, want %d: %+v", got, want, cfg.Routes)
	}
	targets := cfg.Routes[0].Targets
	if got, want := len(targets), 2; got != want {
		t.Fatalf("route targets = %d, want %d: %+v", got, want, targets)
	}
	if targets[0].ModelID != "anthropic-sonnet-anthropic" || targets[1].ModelID != "openrouter-sonnet-or" {
		t.Fatalf("route target order = %+v, want anthropic then openrouter", targets)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupUpdatesExistingProviderWithoutDuplicateTarget(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	cfg, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("first ApplyProviderSetup() error = %v", err)
	}

	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_ALT_KEY",
		UpstreamModel: "openai/gpt-4o",
		ExposedAlias:  "openrouter-gpt4o",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("second ApplyProviderSetup() error = %v", err)
	}

	if got, want := len(cfg.Providers), 1; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if cfg.Providers[0].APIKey != "env:OPENROUTER_ALT_KEY" {
		t.Fatalf("provider key = %q, want updated env reference", cfg.Providers[0].APIKey)
	}
	if got, want := len(cfg.Models), 1; got != want {
		t.Fatalf("models = %d, want %d: %+v", got, want, cfg.Models)
	}
	if cfg.Models[0].ID != "openrouter-openrouter-gpt4o" || cfg.Models[0].UpstreamModel != "openai/gpt-4o" {
		t.Fatalf("model = %+v, want edited OpenRouter model", cfg.Models[0])
	}
	if got, want := len(cfg.Routes[0].Targets), 1; got != want {
		t.Fatalf("route targets = %d, want %d: %+v", got, want, cfg.Routes[0].Targets)
	}
	if cfg.Routes[0].Targets[0].ModelID != "openrouter-openrouter-gpt4o" {
		t.Fatalf("target = %+v, want edited model target", cfg.Routes[0].Targets[0])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRemoveProviderSetupRemovesOwnedModelsAndRouteTargets(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	var err error
	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "ANTHROPIC_API_KEY",
		UpstreamModel: "claude-sonnet-4-5",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("anthropic ApplyProviderSetup() error = %v", err)
	}
	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("openrouter ApplyProviderSetup() error = %v", err)
	}

	cfg, err = RemoveProviderSetup(cfg, "openrouter")
	if err != nil {
		t.Fatalf("RemoveProviderSetup() error = %v", err)
	}

	if got, want := len(cfg.Providers), 1; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if cfg.Providers[0].ID != "anthropic" {
		t.Fatalf("remaining provider = %+v, want anthropic", cfg.Providers[0])
	}
	if got, want := len(cfg.Models), 1; got != want {
		t.Fatalf("models = %d, want %d: %+v", got, want, cfg.Models)
	}
	if cfg.Models[0].ProviderID != "anthropic" {
		t.Fatalf("remaining model = %+v, want anthropic-owned model", cfg.Models[0])
	}
	if got, want := len(cfg.Routes), 1; got != want {
		t.Fatalf("routes = %d, want %d: %+v", got, want, cfg.Routes)
	}
	if got, want := len(cfg.Routes[0].Targets), 1; got != want {
		t.Fatalf("targets = %d, want %d: %+v", got, want, cfg.Routes[0].Targets)
	}
	if cfg.Routes[0].Targets[0].ModelID != "anthropic-sonnet-anthropic" {
		t.Fatalf("target = %+v, want anthropic target", cfg.Routes[0].Targets[0])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRemoveProviderSetupRejectsMissingProvider(t *testing.T) {
	_, err := RemoveProviderSetup(config.BootstrapLocalConfig("local-key"), "missing")
	if err == nil || !strings.Contains(err.Error(), "provider not found") {
		t.Fatalf("error = %v, want provider not found", err)
	}
}
