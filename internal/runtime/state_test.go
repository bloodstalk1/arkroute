package runtime

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
	"gopkg.in/yaml.v3"
)

func TestStateStartsAtGenerationOne(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	current := state.Current()
	if current.Number() != 1 {
		t.Fatalf("generation = %d, want 1", current.Number())
	}
	if current.LoadedAt().IsZero() {
		t.Fatalf("current generation loaded timestamp is zero")
	}
	if current.Snapshot().Config.Version != config.CurrentVersion {
		t.Fatalf("current generation missing snapshot config")
	}
	if state.Status().ConfigPath != path {
		t.Fatalf("config path = %q, want %q", state.Status().ConfigPath, path)
	}
}

func TestStateReloadSuccessSwapsGenerationAndKeepsSharedState(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	health := router.NewHealthStore()
	sink := newRuntimeStateMemorySink()
	state := newRuntimeStateForTestWithShared(t, path, "127.0.0.1", config.DefaultServerPort, health, sink)

	cfg.Routes = append(cfg.Routes, config.RouteConfig{
		Alias:    "opus",
		Strategy: "priority",
		Targets:  []config.RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}},
		Enabled:  true,
	})
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}
	if result.Generation != 2 {
		t.Fatalf("generation = %d, want 2", result.Generation)
	}
	if _, ok := state.Current().Snapshot().RoutesByAlias["opus"]; !ok {
		t.Fatalf("new route not visible after reload")
	}
	if state.Health() != health {
		t.Fatalf("health store was replaced")
	}
	if state.Trace() != sink {
		t.Fatalf("trace sink was replaced")
	}
	if sink.count(EventNameStringConfigReloadSucceeded) == 0 {
		t.Fatalf("reload success trace was not emitted")
	}
}

func TestStateGenerationPlansRoute(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	plan, err := state.Current().Plan("sonnet", router.Requirements{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Alias != "sonnet" {
		t.Fatalf("plan alias = %q, want sonnet", plan.Alias)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("plan targets = %d, want 1", len(plan.Targets))
	}
}

func TestStateGenerationPlanReturnsCopy(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].Headers["Authorization"] = "original"
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	plan, err := state.Current().Plan("sonnet", router.Requirements{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	plan.Targets[0].Provider.Headers["Authorization"] = "mutated"
	plan.Targets[0].Route.Targets[0].ModelID = "mutated"

	nextPlan, err := state.Current().Plan("sonnet", router.Requirements{})
	if err != nil {
		t.Fatalf("Plan() second error = %v", err)
	}
	if nextPlan.Targets[0].Provider.Headers["Authorization"] == "mutated" {
		t.Fatalf("provider headers mutation leaked into generation")
	}
	if nextPlan.Targets[0].Route.Targets[0].ModelID == "mutated" {
		t.Fatalf("route targets mutation leaked into generation")
	}
}

func TestStateReloadFailureKeepsCurrentGeneration(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	cfg.Routes[0].Targets = nil
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.Generation != 1 {
		t.Fatalf("active generation = %d, want 1", result.Generation)
	}
	if state.Current().Number() != 1 {
		t.Fatalf("current generation = %d, want 1", state.Current().Number())
	}
	status := state.Status()
	if status.FailedReloadCount != 1 {
		t.Fatalf("failed reload count = %d, want 1", status.FailedReloadCount)
	}
	if status.LastReloadErrorClass != string(failure.ErrorConfigValidationFailed) {
		t.Fatalf("last reload error class = %q", status.LastReloadErrorClass)
	}
	if !strings.Contains(status.LastReloadError, "routes[0].targets") {
		t.Fatalf("last reload error = %q", status.LastReloadError)
	}
}

func TestStateReloadDirectoryConfigIsReadFailure(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)
	dir := t.TempDir()
	state.configPath = dir

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.ErrorClass != failure.ErrorConfigReadFailed {
		t.Fatalf("error class = %s, want config_read_failed", result.ErrorClass)
	}
	if state.Current().Number() != 1 {
		t.Fatalf("generation changed to %d", state.Current().Number())
	}
}

func TestStateReloadRejectsListenerChange(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	cfg.Server.Port = config.DefaultServerPort + 1
	overwriteRuntimeStateConfig(t, path, cfg)

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.ErrorClass != failure.ErrorListenerChangeRequiresRestart {
		t.Fatalf("error class = %s, want listener_change_requires_restart", result.ErrorClass)
	}
	if state.Current().Number() != 1 {
		t.Fatalf("generation changed to %d", state.Current().Number())
	}
}

func TestStateReloadNilContextDoesNotPanic(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	result := state.Reload(nil, ReloadSourceAdmin, "req_reload")
	if !result.Success {
		t.Fatalf("reload failed: %+v", result)
	}
	if result.Generation != 2 {
		t.Fatalf("generation = %d, want 2", result.Generation)
	}
}

func TestStateSerializesConcurrentReloads(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeRuntimeStateConfig(t, cfg)
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	const reloads = 5
	var wg sync.WaitGroup
	results := make(chan ReloadResult, reloads)
	for i := 0; i < reloads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- state.Reload(context.Background(), ReloadSourceAdmin, fmt.Sprintf("req_reload_%d", i))
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for result := range results {
		if !result.Success {
			t.Fatalf("reload failed: %+v", result)
		}
		successes++
	}
	if successes != reloads {
		t.Fatalf("successes = %d, want %d", successes, reloads)
	}
	if state.Current().Number() != 1+reloads {
		t.Fatalf("generation = %d, want %d", state.Current().Number(), 1+reloads)
	}
}

func TestStateCurrentSnapshotReturnsCopy(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	snapshot := state.Current().Snapshot()
	snapshot.RoutesByAlias["mutated"] = config.RouteConfig{Alias: "mutated", Enabled: true}
	snapshot.Config.Routes[0].Alias = "mutated"
	snapshot.Config.Providers[0].Headers["Authorization"] = "mutated"

	nextSnapshot := state.Current().Snapshot()
	if _, ok := nextSnapshot.RoutesByAlias["mutated"]; ok {
		t.Fatalf("snapshot route map mutation leaked into current generation")
	}
	if nextSnapshot.Config.Routes[0].Alias == "mutated" {
		t.Fatalf("snapshot config route mutation leaked into current generation")
	}
	if nextSnapshot.Config.Providers[0].Headers["Authorization"] == "mutated" {
		t.Fatalf("snapshot provider headers mutation leaked into current generation")
	}
}

func TestStateReloadMalformedReadableConfigIsNotReadFailure(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)

	if err := os.WriteFile(path, []byte("version: ["), 0o600); err != nil {
		t.Fatal(err)
	}

	result := state.Reload(context.Background(), ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.ErrorClass == failure.ErrorConfigReadFailed {
		t.Fatalf("error class = %s, want content failure", result.ErrorClass)
	}
	if result.ErrorClass != failure.ErrorConfigValidationFailed {
		t.Fatalf("error class = %s, want config_validation_failed", result.ErrorClass)
	}
	if state.Current().Number() != 1 {
		t.Fatalf("generation changed to %d", state.Current().Number())
	}
}

func TestStateReloadCanceledContextBeforeStartDoesNotPublish(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := state.Reload(ctx, ReloadSourceAdmin, "req_reload")
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if result.Error == "" {
		t.Fatalf("reload error is empty")
	}
	if state.Current().Number() != 1 {
		t.Fatalf("generation changed to %d", state.Current().Number())
	}
	status := state.Status()
	if !status.LastReloadAttemptAt.IsZero() || status.ReloadCount != 0 || status.FailedReloadCount != 0 {
		t.Fatalf("canceled reload updated reload metadata: %+v", status)
	}
}

func TestStateReloadCanceledWhileWaitingForLockDoesNotPublish(t *testing.T) {
	path := writeRuntimeStateConfig(t, config.MinimalValidConfig("local-key"))
	state := newRuntimeStateForTest(t, path, "127.0.0.1", config.DefaultServerPort)
	state.reloadMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan ReloadResult, 1)

	go func() {
		resultCh <- state.Reload(ctx, ReloadSourceAdmin, "req_reload")
	}()

	cancel()
	state.reloadMu.Unlock()

	result := <-resultCh
	if result.Success {
		t.Fatalf("reload unexpectedly succeeded: %+v", result)
	}
	if state.Current().Number() != 1 {
		t.Fatalf("generation changed to %d", state.Current().Number())
	}
	status := state.Status()
	if !status.LastReloadAttemptAt.IsZero() || status.ReloadCount != 0 || status.FailedReloadCount != 0 {
		t.Fatalf("canceled reload updated reload metadata: %+v", status)
	}
}

func writeRuntimeStateConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	overwriteRuntimeStateConfig(t, path, cfg)
	return path
}

func overwriteRuntimeStateConfig(t *testing.T, path string, cfg config.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newRuntimeStateForTest(t *testing.T, path, host string, port int) *State {
	t.Helper()
	return newRuntimeStateForTestWithShared(t, path, host, port, router.NewHealthStore(), observability.NewNoopSink())
}

func newRuntimeStateForTestWithShared(t *testing.T, path, host string, port int, health *router.HealthStore, trace observability.TraceSink) *State {
	t.Helper()
	state, err := NewState(StateDeps{
		ConfigPath:   path,
		ListenerHost: host,
		ListenerPort: port,
		Health:       health,
		Trace:        trace,
		NewHTTPClient: func(cfg config.Config) *http.Client {
			return &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second}
		},
	})
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	return state
}

type runtimeStateMemorySink struct {
	events []observability.TraceEvent
}

const EventNameStringConfigReloadSucceeded = "config_reload_succeeded"

func newRuntimeStateMemorySink() *runtimeStateMemorySink {
	return &runtimeStateMemorySink{}
}

func (s *runtimeStateMemorySink) Emit(event observability.TraceEvent) {
	s.events = append(s.events, event)
}

func (s *runtimeStateMemorySink) Stats() observability.Stats {
	return observability.Stats{Emitted: int64(len(s.events))}
}

func (s *runtimeStateMemorySink) count(event string) int {
	count := 0
	for _, item := range s.events {
		if string(item.Event) == event {
			count++
		}
	}
	return count
}
