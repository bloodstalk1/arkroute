package router

import (
	"strings"
	"testing"
	"time"

	"bat.dev/arkroute/internal/config"
)

func TestResolveRouteByAliasAndDiscoveryAlias(t *testing.T) {
	snapshot, err := config.BuildSnapshot(config.MinimalValidConfig("local-key"))
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	r := New(snapshot, NewHealthStore())
	for _, alias := range []string{"sonnet", "claude-sonnet-4-20250514"} {
		targets, err := r.Resolve(alias, Requirements{Streaming: true, Tools: true})
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", alias, err)
		}
		if len(targets) != 1 || targets[0].Model.ID != "openrouter-sonnet" {
			t.Fatalf("targets = %+v", targets)
		}
	}
}

func TestResolveRejectsMissingCapability(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Models[0].Capabilities.Tools = false
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	_, err = New(snapshot, NewHealthStore()).Resolve("sonnet", Requirements{Tools: true})
	if err == nil {
		t.Fatal("Resolve() error = nil, want unsupported capability")
	}
}

func TestRetryableStatus(t *testing.T) {
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		if !IsRetryableStatus(status) {
			t.Fatalf("%d should be retryable", status)
		}
	}
	for _, status := range []int{400, 401, 403, 404} {
		if IsRetryableStatus(status) {
			t.Fatalf("%d should not be retryable", status)
		}
	}
}

func TestPlanReturnsRoutePlan(t *testing.T) {
	snapshot, err := config.BuildSnapshot(config.MinimalValidConfig("local-key"))
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	plan, err := New(snapshot, NewHealthStore()).Plan("sonnet", Requirements{Streaming: true, Tools: true})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Alias != "sonnet" || plan.Strategy != "fallback" || len(plan.Targets) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestFallbackPolicyReturnsAllTargets(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers = append(cfg.Providers, cfg.Providers[0])
	cfg.Providers[1].ID = "backup"
	cfg.Providers[1].Enabled = true
	cfg.Models = append(cfg.Models, cfg.Models[0])
	cfg.Models[1].ID = "backup-model"
	cfg.Models[1].ProviderID = "backup"
	cfg.Models[1].ClaudeDiscoveryAlias = ""
	cfg.Models[1].ExposedAlias = "backup-or"
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}, {ModelID: "backup-model", Enabled: true}}
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	plan, err := New(snapshot, NewHealthStore()).Plan("sonnet", Requirements{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	targets, reason := FallbackPolicy{}.Select(plan, NewHealthStore().Snapshot())
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	if reason == "" {
		t.Fatal("reason is empty")
	}
}

func TestHealthStoreStoresSanitizedDetails(t *testing.T) {
	store := NewHealthStore()
	store.Update(Update{
		ProviderID:    "openrouter",
		UpstreamModel: "model",
		Status:        "degraded",
		StatusCode:    429,
		ErrorClass:    "upstream_rate_limit",
		ErrorMessage:  strings.Repeat("x", 300),
		Latency:       time.Second,
	})
	health := store.Snapshot()["openrouter"]
	if health.Status != "degraded" || health.LastStatusCode != 429 {
		t.Fatalf("health = %+v", health)
	}
	if len(health.LastErrorMessage) > 160 {
		t.Fatalf("error message not limited: %d", len(health.LastErrorMessage))
	}
}
