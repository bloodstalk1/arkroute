package config

import (
	"strings"
	"testing"
)

func TestValidateAcceptsMinimalGeneratedConfig(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestGeneratedConfigsUseDefaultServerPort(t *testing.T) {
	for name, cfg := range map[string]Config{
		"minimal":   MinimalValidConfig("ark-local-key"),
		"bootstrap": BootstrapLocalConfig("ark-local-key"),
	} {
		if cfg.Server.Port != 2002 {
			t.Fatalf("%s config server port = %d, want 2002", name, cfg.Server.Port)
		}
	}
}

func TestApplyDefaultsUsesDefaultServerPort(t *testing.T) {
	cfg := Config{}
	ApplyDefaults(&cfg)
	if cfg.Server.Port != 2002 {
		t.Fatalf("default server port = %d, want 2002", cfg.Server.Port)
	}
}

func TestValidateAcceptsAutoDetectedProviderType(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Providers[0].Type = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() empty provider type error = %v", err)
	}
	cfg.Providers[0].Type = "auto"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() auto provider type error = %v", err)
	}
}

func TestValidateAcceptsModelProtocolOverride(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].Protocol = "anthropic"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() model protocol override error = %v", err)
	}
}

func TestValidateRejectsInvalidModelProtocolOverride(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].Protocol = "claude-compatible"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want model protocol error")
	}
	if !strings.Contains(err.Error(), "models[0].protocol") {
		t.Fatalf("error = %q, want model protocol field", err.Error())
	}
}

func TestValidateRejectsNonLoopbackHost(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Server.Host = "0.0.0.0"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "server.host") {
		t.Fatalf("error = %q, want server.host", err.Error())
	}
}

func TestValidateRejectsBrokenReferences(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].ProviderID = "missing"
	cfg.Routes[0].Targets[0].ModelID = "missing-model"
	cfg.Profiles["default"] = "missing-route"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	for _, want := range []string{"models[0].provider_id", "routes[0].targets[0].model_id", "profiles.default"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %s", err.Error(), want)
		}
	}
}

func TestValidateRejectsInvalidReasoningEffort(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].Reasoning.Effort = "ultracode"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "models[0].reasoning.effort") {
		t.Fatalf("error = %q, want reasoning effort field", err.Error())
	}
}

func TestValidateRejectsInvalidReasoningMode(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].Reasoning.Mode = "hardcoded"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "models[0].reasoning.mode") {
		t.Fatalf("error = %q, want reasoning mode field", err.Error())
	}
}

func TestBuildSnapshotAppliesCompatibilityPolicyWithoutOverridingModelReasoning(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].Reasoning.Replay = configBoolPtr(false)
	cfg.CompatibilityPolicies = []CompatibilityPolicyConfig{{
		ID: "openrouter-sonnet-reasoning",
		Match: CompatibilityMatchConfig{
			ProviderIDContains:    []string{"openrouter"},
			UpstreamModelPatterns: []string{"*claude-sonnet*"},
		},
		Reasoning: CompatibilityReasoningConfig{
			AutoEnable:     configBoolPtr(true),
			AutoEffort:     "high",
			Replay:         configBoolPtr(true),
			OmitToolChoice: configBoolPtr(true),
		},
	}}

	snapshot, err := BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	got := snapshot.ModelsByID[cfg.Models[0].ID].Reasoning
	if got.Replay == nil || *got.Replay {
		t.Fatalf("Replay = %v, want model-level false to win", got.Replay)
	}
	if got.OmitToolChoice == nil || !*got.OmitToolChoice {
		t.Fatalf("OmitToolChoice = %v, want policy true", got.OmitToolChoice)
	}
	if got.AutoEnable == nil || !*got.AutoEnable {
		t.Fatalf("AutoEnable = %v, want policy true", got.AutoEnable)
	}
	if got.AutoEffort != "high" {
		t.Fatalf("AutoEffort = %q, want high", got.AutoEffort)
	}
}

func TestValidateRejectsInvalidCompatibilityPolicy(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.CompatibilityPolicies = []CompatibilityPolicyConfig{{
		ID: "bad-policy",
		Match: CompatibilityMatchConfig{
			UpstreamModelPatterns: []string{""},
		},
		Reasoning: CompatibilityReasoningConfig{AutoEffort: "ultracode"},
	}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want compatibility policy error")
	}
	for _, want := range []string{
		"compatibility_policies[0].match.upstream_model_patterns[0]",
		"compatibility_policies[0].reasoning.auto_effort",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %s", err.Error(), want)
		}
	}
}

func TestValidateRejectsRedactedMarkers(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")

	// Test client key
	cfg.Server.ClientKey = "[redacted]"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "server.client_key: cannot contain [redacted] marker") {
		t.Fatalf("expected client_key error, got %v", err)
	}
	cfg.Server.ClientKey = "valid-key"

	// Test provider API key
	cfg.Providers[0].APIKey = "[redacted]"
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "providers[0].api_key: cannot contain [redacted] marker") {
		t.Fatalf("expected api_key error, got %v", err)
	}
	cfg.Providers[0].APIKey = "valid-api-key"

	// Test provider headers
	cfg.Providers[0].Headers = map[string]string{"X-Header": "[redacted]"}
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "providers[0].headers[X-Header]: cannot contain [redacted] marker") {
		t.Fatalf("expected header error, got %v", err)
	}
}

func TestBuildSnapshotIndexesAliases(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	snapshot, err := BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	if _, ok := snapshot.RoutesByAlias["sonnet"]; !ok {
		t.Fatal("RoutesByAlias missing sonnet")
	}
	if _, ok := snapshot.RoutesByDiscoveryAlias["claude-sonnet-4-20250514"]; !ok {
		t.Fatal("RoutesByDiscoveryAlias missing Claude alias")
	}
	if _, ok := snapshot.ModelsByExposedAlias["sonnet-or"]; !ok {
		t.Fatal("ModelsByExposedAlias missing sonnet-or")
	}
}

func TestValidateAcceptsBootstrapLocalConfig(t *testing.T) {
	cfg := BootstrapLocalConfig("ark-local-key")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() bootstrap error = %v", err)
	}
	if len(cfg.Providers) != 0 || len(cfg.Models) != 0 || len(cfg.Routes) != 0 {
		t.Fatalf("bootstrap config should not create providers/models/routes: %+v", cfg)
	}
	if !cfg.Clients.Claude.Enabled || !cfg.Clients.Claude.ModelDiscovery {
		t.Fatalf("bootstrap Claude settings = %+v", cfg.Clients.Claude)
	}
}

func TestValidateRejectsBrokenReferencesWhenPartialSetupExists(t *testing.T) {
	cfg := BootstrapLocalConfig("ark-local-key")
	cfg.Models = []ModelConfig{{
		ID:            "broken-model",
		ProviderID:    "missing",
		UpstreamModel: "provider/model",
		ExposedAlias:  "broken",
		Enabled:       true,
	}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want missing provider reference")
	}
	if !strings.Contains(err.Error(), "models[0].provider_id") {
		t.Fatalf("error = %q, want models[0].provider_id", err.Error())
	}
}

func configBoolPtr(value bool) *bool {
	return &value
}

func TestInspectCompatibilityPoliciesExplainsPrecedence(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].UpstreamModel = "provider/deepseek-v4-pro"
	cfg.Models[0].Reasoning.Replay = configBoolPtr(false)
	cfg.CompatibilityPolicies = []CompatibilityPolicyConfig{{
		ID: "user-deepseek-v4",
		Match: CompatibilityMatchConfig{
			UpstreamModelPatterns: []string{"*deepseek*v4*"},
		},
		Reasoning: CompatibilityReasoningConfig{
			OmitToolChoice: configBoolPtr(false),
		},
	}}

	got := InspectCompatibilityPolicies(cfg.Providers[0], cfg.Models[0], cfg.CompatibilityPolicies)
	for _, want := range []CompatibilityPolicyMatch{
		{ID: "user-deepseek-v4", Source: "user"},
		{ID: "deepseek-v4-openai-compatible", Source: "builtin"},
		{ID: "reasoning-replay-provider-families", Source: "builtin"},
		{ID: "reasoning-replay-model-families", Source: "builtin"},
	} {
		if !hasCompatibilityPolicyMatch(got.MatchedPolicies, want.ID, want.Source) {
			t.Fatalf("matched policies = %+v, missing %+v", got.MatchedPolicies, want)
		}
	}
	if got.Model.Reasoning.Replay == nil || *got.Model.Reasoning.Replay {
		t.Fatalf("replay = %v, want model override false", got.Model.Reasoning.Replay)
	}
	if got.Model.Reasoning.OmitToolChoice == nil || *got.Model.Reasoning.OmitToolChoice {
		t.Fatalf("omit_tool_choice = %v, want user override false", got.Model.Reasoning.OmitToolChoice)
	}
	if got.Model.Reasoning.AutoEnable == nil || !*got.Model.Reasoning.AutoEnable {
		t.Fatalf("auto_enable = %v, want builtin true", got.Model.Reasoning.AutoEnable)
	}
	if got.Model.Reasoning.AutoEffort != "max" {
		t.Fatalf("auto_effort = %q, want max", got.Model.Reasoning.AutoEffort)
	}
	if got.ReasoningSources["replay"].Source != "model" {
		t.Fatalf("replay source = %+v, want model source", got.ReasoningSources["replay"])
	}
	if got.ReasoningSources["omit_tool_choice"].PolicyID != "user-deepseek-v4" {
		t.Fatalf("omit_tool_choice source = %+v, want user policy source", got.ReasoningSources["omit_tool_choice"])
	}
	if got.ReasoningSources["auto_enable"].PolicyID != "deepseek-v4-openai-compatible" {
		t.Fatalf("auto_enable source = %+v, want builtin policy source", got.ReasoningSources["auto_enable"])
	}
	explain := strings.Join(got.Explain, "\n")
	for _, want := range []string{
		"models[].reasoning.replay overrides policy deepseek-v4-openai-compatible replay",
		"user policy user-deepseek-v4 sets omit_tool_choice",
		"builtin policy deepseek-v4-openai-compatible sets auto_enable",
	} {
		if !strings.Contains(explain, want) {
			t.Fatalf("explain missing %q: %s", want, explain)
		}
	}
}

func hasCompatibilityPolicyMatch(matches []CompatibilityPolicyMatch, id, source string) bool {
	for _, match := range matches {
		if match.ID == id && match.Source == source {
			return true
		}
	}
	return false
}

func TestLoadBytesAppliesDefaultsAndMigration(t *testing.T) {
	data := []byte(`
version: 1
server:
  host: 127.0.0.1
  client_key: local-key
clients:
  claude:
    enabled: true
providers: []
models: []
routes: []
profiles: {}
`)
	cfg, err := LoadBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != DefaultServerPort {
		t.Fatalf("port = %d, want default %d", cfg.Server.Port, DefaultServerPort)
	}
	if cfg.Server.UpstreamTimeoutSeconds != 600 {
		t.Fatalf("timeout = %d, want 600", cfg.Server.UpstreamTimeoutSeconds)
	}
	if cfg.Profiles == nil {
		t.Fatal("profiles = nil, want initialized map")
	}
}

func TestLoadBytesRejectsInvalidYAML(t *testing.T) {
	if _, err := LoadBytes([]byte("server:\n  host: [")); err == nil {
		t.Fatal("LoadBytes error = nil, want YAML parse error")
	}
}

