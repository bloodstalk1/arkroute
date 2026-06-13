package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/router"
)

// TestStateHotReloadSwapsGeneration verifies that Reload atomically
// installs a new Generation with a new model alias while a parallel
// goroutine keeps reading the previous generation. No goroutine should
// see a torn snapshot.
func TestStateHotReloadSwapsGeneration(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	pre := state.Current()
	preSnapshot := pre.Snapshot()
	preGen := pre.Number()
	if _, ok := preSnapshot.ModelsByExposedAlias["sonnet-or"]; !ok {
		t.Fatal("pre-reload snapshot missing sonnet-or alias")
	}

	// Update config: change the upstream model on the existing route
	// target, then Reload. The exposed_alias stays so downstream
	// references don't break, but the model_id changes.
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models[0].UpstreamModel = "anthropic/claude-sonnet-4.6"
	overwriteRuntimeStateConfig(t, path, cfg)

	const readers = 8
	const reads = 200
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reads; j++ {
				select {
				case <-stop:
					return
				default:
				}
				g := state.Current()
				if g == nil {
					t.Error("nil generation under load")
					return
				}
				_ = g.Snapshot()
			}
		}()
	}

	result := state.Reload(context.Background(), ReloadSourceAdmin, "test_hot_reload")
	if !result.Success {
		t.Fatalf("reload failed: %s", result.Error)
	}
	if result.Generation <= preGen {
		t.Fatalf("generation did not advance: pre=%d post=%d", preGen, result.Generation)
	}

	post := state.Current()
	if post.Number() != result.Generation {
		t.Fatalf("Current().Number() = %d, want %d", post.Number(), result.Generation)
	}
	if _, ok := post.Snapshot().ModelsByExposedAlias["sonnet-or"]; !ok {
		t.Fatal("post-reload snapshot missing sonnet-or alias")
	}

	close(stop)
	wg.Wait()
}

// TestStateHotReloadInvalidConfigKeepsOldGeneration verifies that a
// reload that fails validation does NOT swap the active generation.
// Callers can keep using the pre-reload generation safely.
func TestStateHotReloadInvalidConfigKeepsOldGeneration(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	pre := state.Current()
	preGen := pre.Number()

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "does-not-exist", Enabled: true}}
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "test_bad_reload")
	if result.Success {
		t.Fatal("expected reload to fail validation")
	}
	if state.Current().Number() != preGen {
		t.Fatalf("generation changed after failed reload: pre=%d now=%d", preGen, state.Current().Number())
	}
	if state.Current() != pre {
		t.Fatal("active Generation pointer changed after failed reload")
	}
}

// TestStateHotReloadDuringPlanExecution runs Plan repeatedly across a
// reload. The pre-reload generation must keep returning the original
// model; the post-reload generation (after the swap) must return the
// new upstream model.
func TestStateHotReloadDuringPlanExecution(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	pre := state.Current()

	// Pre-reload Plan must succeed with the original upstream model.
	prePlan, err := pre.Plan("sonnet", router.Requirements{Streaming: true, Tools: true})
	if err != nil {
		t.Fatalf("pre-reload Plan: %v", err)
	}
	if len(prePlan.Targets) == 0 || prePlan.Targets[0].Model.UpstreamModel != "anthropic/claude-sonnet-4.5" {
		t.Fatalf("pre-reload upstream = %q, want anthropic/claude-sonnet-4.5",
			prePlan.Targets[0].Model.UpstreamModel)
	}

	// Start a reload that updates the upstream model.
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models[0].UpstreamModel = "anthropic/claude-sonnet-4.6"
	overwriteRuntimeStateConfig(t, path, cfg)

	reloadDone := make(chan ReloadResult, 1)
	go func() {
		reloadDone <- state.Reload(context.Background(), ReloadSourceAdmin, "test_concurrent_reload")
	}()

	// Hold a reference to the pre-reload generation across the reload
	// and call Plan repeatedly. The pre-reload generation must keep
	// returning the original upstream model.
	for i := 0; i < 50; i++ {
		plan, err := pre.Plan("sonnet", router.Requirements{Streaming: true, Tools: true})
		if err != nil {
			t.Fatalf("pre-reload Plan during reload (iter %d): %v", i, err)
		}
		if plan.Targets[0].Model.UpstreamModel != "anthropic/claude-sonnet-4.5" {
			t.Fatalf("pre-reload Plan mutated (iter %d): upstream = %q",
				i, plan.Targets[0].Model.UpstreamModel)
		}
	}

	select {
	case r := <-reloadDone:
		if !r.Success {
			t.Fatalf("reload failed: %s", r.Error)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reload did not complete")
	}

	// Post-reload Plan must reflect the new upstream model.
	post := state.Current()
	if post == pre {
		t.Fatal("active generation did not change after reload")
	}
	postPlan, err := post.Plan("sonnet", router.Requirements{Streaming: true, Tools: true})
	if err != nil {
		t.Fatalf("post-reload Plan: %v", err)
	}
	if postPlan.Targets[0].Model.UpstreamModel != "anthropic/claude-sonnet-4.6" {
		t.Fatalf("post-reload upstream = %q, want anthropic/claude-sonnet-4.6",
			postPlan.Targets[0].Model.UpstreamModel)
	}
}
