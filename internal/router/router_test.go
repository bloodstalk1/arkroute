package router

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func testSnapshot() config.Snapshot {
	cfg := config.MinimalValidConfig("local-key")
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		panic(err)
	}
	return snap
}

func TestResolvePriorityReturnsFirst(t *testing.T) {
	r := New(testSnapshot(), NewHealthStore())
	targets, err := r.Resolve("sonnet", Requirements{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("priority should return 1 target, got %d", len(targets))
	}
}

func TestResolveFallbackReturnsAll(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "fallback"
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: "openrouter2", Name: "OR2", Type: "openai_compatible",
		BaseURL: "https://openrouter.ai/api/v1", APIKey: "env:OPENROUTER_API_KEY", Enabled: true,
	})
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: "model2", ProviderID: "openrouter2", UpstreamModel: "anthropic/claude-4",
		ExposedAlias: "sonnet2", Enabled: true,
		Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true},
	})
	cfg.Routes[0].Targets = append(cfg.Routes[0].Targets, config.RouteTarget{ModelID: "model2", Enabled: true})
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	targets, err := r.Resolve("sonnet", Requirements{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("fallback should return 2 targets, got %d", len(targets))
	}
}

func TestRoundRobinReturnsOne(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "round_robin"
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	for i := 0; i < 10; i++ {
		targets, err := r.Resolve("sonnet", Requirements{})
		if err != nil {
			t.Fatal(err)
		}
		if len(targets) != 1 {
			t.Fatalf("round_robin should return 1 target, got %d", len(targets))
		}
	}
}

func TestWeightedReturnsOne(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "weighted"
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	targets, err := r.Resolve("sonnet", Requirements{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("weighted should return 1 target, got %d", len(targets))
	}
}

func TestCircuitBreakerSkipsFailedProvider(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "fallback"
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: "openrouter2", Name: "OR2", Type: "openai_compatible",
		BaseURL: "https://openrouter.ai/api/v1", APIKey: "env:KEY", Enabled: true,
	})
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: "model2", ProviderID: "openrouter2", UpstreamModel: "anthropic/claude-4",
		ExposedAlias: "sonnet2", Enabled: true,
		Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true},
	})
	cfg.Routes[0].Targets = append(cfg.Routes[0].Targets, config.RouteTarget{ModelID: "model2", Enabled: true})
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}

	health := NewHealthStore()
	// Simulate 3 consecutive failures on the first provider.
	for i := 0; i < 3; i++ {
		health.Update(Update{ProviderID: "openrouter", UpstreamModel: "anthropic/claude-sonnet-4.5", Status: "error"})
	}

	r := New(snap, health)
	targets, err := r.Resolve("sonnet", Requirements{})
	if err != nil {
		t.Fatal(err)
	}
	// First provider should be skipped by circuit breaker,
	// so only the second target is returned.
	if len(targets) != 1 {
		t.Fatalf("circuit breaker should skip failed provider, got %d targets", len(targets))
	}
	if targets[0].Provider.ID != "openrouter2" {
		t.Errorf("expected openrouter2, got %s", targets[0].Provider.ID)
	}
}

func TestValidateAllowsNewStrategies(t *testing.T) {
	for _, strategy := range []string{"priority", "fallback", "round_robin", "weighted"} {
		cfg := config.MinimalValidConfig("local-key")
		cfg.Routes[0].Strategy = strategy
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() for %s: %v, want nil", strategy, err)
		}
	}
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() for invalid strategy: nil, want error")
	}
}

func TestResolveUnknownAliasReturnsError(t *testing.T) {
	r := New(testSnapshot(), NewHealthStore())
	_, err := r.Resolve("does-not-exist", Requirements{})
	if err == nil {
		t.Fatal("expected error for unknown alias")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want substring 'not found'", err)
	}
}

func TestResolveModelRejectsUnsupportedCapabilities(t *testing.T) {
	r := New(testSnapshot(), NewHealthStore())
	// MinimalValidConfig's model advertises no Vision, so requesting it
	// must fail.
	_, err := r.Resolve("sonnet-or", Requirements{Vision: true})
	if err == nil {
		t.Fatal("expected error for missing capability")
	}
	if !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("error = %q, want substring 'does not support'", err)
	}
}

func TestResolveDiscoveryAlias(t *testing.T) {
	r := New(testSnapshot(), NewHealthStore())
	targets, err := r.Resolve("claude-sonnet-4-20250514", Requirements{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("discovery alias returned %d targets, want 1", len(targets))
	}
}

func TestResolveRouteNoMatchingCapability(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	_, err = r.Resolve("sonnet", Requirements{Vision: true})
	if err == nil {
		t.Fatal("expected error when route has no vision-capable target")
	}
	if !strings.Contains(err.Error(), "no target matching") {
		t.Fatalf("error = %q, want 'no target matching'", err)
	}
}

func TestResolveRouteDisabledTargetsSkipped(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: false}}
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	_, err = r.Resolve("sonnet", Requirements{})
	if err == nil {
		t.Fatal("expected error when all targets are disabled")
	}
}

func TestSelectRoundRobinRotatesAcrossTargets(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "round_robin"
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: "or2", Name: "OR2", Type: "openai_compatible",
		BaseURL: "https://x", APIKey: "env:K", Enabled: true,
	})
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: "m2", ProviderID: "or2", UpstreamModel: "u",
		ExposedAlias: "a2", Enabled: true,
		Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true},
	})
	cfg.Routes[0].Targets = []config.RouteTarget{
		{ModelID: "openrouter-sonnet", Enabled: true},
		{ModelID: "m2", Enabled: true},
	}
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	seen := map[string]int{}
	for i := 0; i < 20; i++ {
		targets, err := r.Resolve("sonnet", Requirements{})
		if err != nil {
			t.Fatal(err)
		}
		if len(targets) != 1 {
			t.Fatalf("round_robin returned %d targets, want 1", len(targets))
		}
		seen[targets[0].Provider.ID]++
	}
	if seen["openrouter"] == 0 || seen["or2"] == 0 {
		t.Fatalf("expected both providers to be picked, got %v", seen)
	}
}

func TestSelectWeightedPrefersHigherWeight(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Routes[0].Strategy = "weighted"
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: "or2", Name: "OR2", Type: "openai_compatible",
		BaseURL: "https://x", APIKey: "env:K", Enabled: true,
	})
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: "m2", ProviderID: "or2", UpstreamModel: "u",
		ExposedAlias: "a2", Enabled: true,
		Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true},
	})
	cfg.Routes[0].Targets = []config.RouteTarget{
		{ModelID: "openrouter-sonnet", Enabled: true, Weight: 1},
		{ModelID: "m2", Enabled: true, Weight: 99},
	}
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	seen := map[string]int{}
	for i := 0; i < 200; i++ {
		targets, err := r.Resolve("sonnet", Requirements{})
		if err != nil {
			t.Fatal(err)
		}
		seen[targets[0].Provider.ID]++
	}
	if seen["or2"] <= seen["openrouter"]*5 {
		t.Fatalf("expected or2 to dominate, got %v", seen)
	}
}

func TestPolicyForUnknownStrategyFallsBackToPriority(t *testing.T) {
	policy := PolicyFor("nonsense")
	if _, ok := policy.(PriorityPolicy); !ok {
		t.Fatalf("PolicyFor(\"nonsense\") = %T, want PriorityPolicy", policy)
	}
}

func TestPolicyForFallbackReturnsFallbackPolicy(t *testing.T) {
	policy := PolicyFor("fallback")
	if _, ok := policy.(FallbackPolicy); !ok {
		t.Fatalf("PolicyFor(\"fallback\") = %T, want FallbackPolicy", policy)
	}
}
