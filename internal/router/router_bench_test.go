package router

import (
	"strconv"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

// makeBenchSnapshot builds a snapshot with `routes` distinct fallback
// routes, each with `targetsPerRoute` enabled targets. Aliases follow
// the pattern r0, r1, ... and model ids follow the pattern m0, m1, ...
func makeBenchSnapshot(b *testing.B, routes, targetsPerRoute int) config.Snapshot {
	b.Helper()
	cfg := config.MinimalValidConfig("bench-key")
	cfg.Providers = nil
	cfg.Models = nil
	cfg.Routes = nil

	// One provider backing all models.
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID:      "bench",
		Name:    "Bench",
		Type:    "openai_compatible",
		BaseURL: "https://bench.example/v1",
		APIKey:  "k",
		Enabled: true,
	})

	for r := 0; r < routes; r++ {
		alias := "r" + strconv.Itoa(r)
		var targets []config.RouteTarget
		for t := 0; t < targetsPerRoute; t++ {
			modelID := "m" + strconv.Itoa(r*targetsPerRoute+t)
			cfg.Models = append(cfg.Models, config.ModelConfig{
				ID:            modelID,
				ProviderID:    "bench",
				UpstreamModel: "u/" + modelID,
				ExposedAlias:  modelID,
				DisplayName:   modelID,
				Enabled:       true,
				Capabilities: config.Capabilities{
					Streaming:      true,
					Tools:          true,
					SystemMessages: true,
					ContextWindow:  100000,
				},
			})
			targets = append(targets, config.RouteTarget{ModelID: modelID, Enabled: true})
		}
		cfg.Routes = append(cfg.Routes, config.RouteConfig{
			Alias:    alias,
			Strategy: "fallback",
			Targets:  targets,
			Enabled:  true,
		})
	}
	// Drop the default profiles inherited from MinimalValidConfig; they
	// reference "sonnet" which is not in the benchmark snapshot.
	cfg.Profiles = map[string]string{}
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		b.Fatal(err)
	}
	return snap
}
func BenchmarkResolveSmall(b *testing.B) {
	snap := makeBenchSnapshot(b, 10, 5)
	r := New(snap, NewHealthStore())
	req := Requirements{Streaming: true, Tools: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Resolve("r0", req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResolveLarge measures Resolve on a snapshot with 100 routes
// of 10 targets each. Models a heavily-loaded gateway.
func BenchmarkResolveLarge(b *testing.B) {
	snap := makeBenchSnapshot(b, 100, 10)
	r := New(snap, NewHealthStore())
	req := Requirements{Streaming: true, Tools: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Resolve("r50", req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResolvePriority exercises the priority-strategy path: the
// resolver can stop after the first match without scanning the rest
// of the route's targets.
func BenchmarkResolvePriority(b *testing.B) {
	snap := makeBenchSnapshot(b, 10, 50)
	r := New(snap, NewHealthStore())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Resolve("r0", Requirements{Streaming: true, Tools: true})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResolveWeighted exercises the weighted selection path which
// rolls math/rand and walks a sorted list.
func BenchmarkResolveWeighted(b *testing.B) {
	cfg := config.MinimalValidConfig("bench-key")
	cfg.Providers = nil
	cfg.Models = nil
	cfg.Routes = nil
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: "w0", Name: "W0", Type: "openai_compatible", BaseURL: "https://x", APIKey: "k", Enabled: true,
	}, config.ProviderConfig{
		ID: "w1", Name: "W1", Type: "openai_compatible", BaseURL: "https://y", APIKey: "k", Enabled: true,
	}, config.ProviderConfig{
		ID: "w2", Name: "W2", Type: "openai_compatible", BaseURL: "https://z", APIKey: "k", Enabled: true,
	})
	cfg.Models = []config.ModelConfig{
		{ID: "wm0", ProviderID: "w0", UpstreamModel: "u0", ExposedAlias: "wm0", Enabled: true, Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true}},
		{ID: "wm1", ProviderID: "w1", UpstreamModel: "u1", ExposedAlias: "wm1", Enabled: true, Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true}},
		{ID: "wm2", ProviderID: "w2", UpstreamModel: "u2", ExposedAlias: "wm2", Enabled: true, Capabilities: config.Capabilities{Streaming: true, Tools: true, SystemMessages: true}},
	}
	cfg.Routes = []config.RouteConfig{{
		Alias:    "weighted",
		Strategy: "weighted",
		Targets: []config.RouteTarget{
			{ModelID: "wm0", Enabled: true, Weight: 1},
			{ModelID: "wm1", Enabled: true, Weight: 3},
			{ModelID: "wm2", Enabled: true, Weight: 9},
		},
		Enabled: true,
	}}
	cfg.Profiles = map[string]string{}
	snap, err := config.BuildSnapshot(cfg)
	if err != nil {
		b.Fatal(err)
	}
	r := New(snap, NewHealthStore())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Resolve("weighted", Requirements{Streaming: true, Tools: true})
		if err != nil {
			b.Fatal(err)
		}
	}
}
