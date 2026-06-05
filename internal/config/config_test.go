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
