package router

import (
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
