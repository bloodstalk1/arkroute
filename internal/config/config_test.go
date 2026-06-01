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
