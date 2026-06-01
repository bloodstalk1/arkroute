package router

import (
	"testing"

	"bat.dev/arkrouter/internal/config"
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
