// Package runtime owns the long-lived serving state of arkroute: the
// currently active config generation, the router, the upstream
// executor, and the reloading machinery triggered by SIGHUP or the
// admin /internal/reload endpoint. A process holds exactly one
// [*State]; all HTTP handlers receive it through their Deps.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bloodstalk1/arkroute/internal/adapter"
	"github.com/bloodstalk1/arkroute/internal/adapter/builtin"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
	"gopkg.in/yaml.v3"
)

// ReloadSource identifies what triggered a [State.Reload] call. The
// value is surfaced in trace events so operators can tell admin
// reloads apart from SIGHUPs.
type ReloadSource string

const (
	ReloadSourceAdmin  ReloadSource = "admin"
	ReloadSourceSignal ReloadSource = "signal"
)

// StateDeps wires a [State] to the rest of the process: the on-disk
// config path, the shared adapter registry, the health store, the
// trace sink, and an optional constructor for the upstream HTTP
// client (tests use it to inject a short-timeout client).
type StateDeps struct {
	ConfigPath    string
	ListenerHost  string
	ListenerPort  int
	Adapters      adapter.Registry
	Health        *router.HealthStore
	Trace         observability.TraceSink
	NewHTTPClient func(config.Config) *http.Client
}

// State is the long-lived runtime handle. It is safe for concurrent
// use; all mutating operations (Reload) are serialised internally.
type State struct {
	mu            sync.RWMutex
	reloadMu      sync.Mutex
	configPath    string
	listenerHost  string
	listenerPort  int
	adapters      adapter.Registry
	health        *router.HealthStore
	trace         observability.TraceSink
	newHTTPClient func(config.Config) *http.Client
	current       *Generation
	meta          ReloadStatus
}

// Generation is one immutable snapshot of the routing/executing
// pipeline. Reload swaps the active Generation atomically.
type Generation struct {
	number   uint64
	loadedAt time.Time
	snapshot config.Snapshot
	router   *router.Router
	executor *Executor
}

// ReloadStatus is the JSON-serialisable view of the runtime's reload
// history. It is what the admin endpoint returns.
type ReloadStatus struct {
	ConfigPath             string    `json:"config_path"`
	Generation             uint64    `json:"generation"`
	ConfigLoadedAt         time.Time `json:"config_loaded_at"`
	LastReloadAttemptAt    time.Time `json:"last_reload_attempt_at,omitempty"`
	LastSuccessfulReloadAt time.Time `json:"last_successful_reload_at,omitempty"`
	LastFailedReloadAt     time.Time `json:"last_failed_reload_at,omitempty"`
	LastReloadErrorClass   string    `json:"last_reload_error_class,omitempty"`
	LastReloadError        string    `json:"last_reload_error,omitempty"`
	ReloadCount            uint64    `json:"reload_count"`
	FailedReloadCount      uint64    `json:"failed_reload_count"`
}

// ReloadResult is the per-call outcome of [State.Reload].
type ReloadResult struct {
	Success        bool
	Status         string
	Generation     uint64
	ConfigLoadedAt time.Time
	ErrorClass     failure.ErrorClass
	Error          string
}

func NewState(deps StateDeps) (*State, error) {
	if deps.ConfigPath == "" {
		return nil, fmt.Errorf("config path must be non-empty")
	}
	if deps.Health == nil {
		deps.Health = router.NewHealthStore()
	}
	if deps.Trace == nil {
		deps.Trace = observability.NewNoopSink()
	}
	if deps.Adapters == nil {
		deps.Adapters = builtin.DefaultRegistry()
	}
	if deps.NewHTTPClient == nil {
		deps.NewHTTPClient = func(cfg config.Config) *http.Client {
			return &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second}
		}
	}
	cfg, err := config.LoadFile(deps.ConfigPath)
	if err != nil {
		return nil, err
	}
	gen, err := buildGeneration(1, cfg, deps.Adapters, deps.Health, deps.Trace, deps.NewHTTPClient)
	if err != nil {
		return nil, err
	}
	state := &State{
		configPath:    deps.ConfigPath,
		listenerHost:  deps.ListenerHost,
		listenerPort:  deps.ListenerPort,
		adapters:      deps.Adapters,
		health:        deps.Health,
		trace:         deps.Trace,
		newHTTPClient: deps.NewHTTPClient,
		current:       gen,
	}
	state.meta = ReloadStatus{
		ConfigPath:     deps.ConfigPath,
		Generation:     gen.number,
		ConfigLoadedAt: gen.loadedAt,
	}
	return state, nil
}

// Current returns the active [Generation]. The pointer is stable for
// the lifetime of a generation; the snapshot it exposes is always a
// deep clone, so callers may freely mutate it.
func (s *State) Current() *Generation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Status returns the current [ReloadStatus]. Safe to call from any
// goroutine; the returned value is a copy.
func (s *State) Status() ReloadStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

// Health returns the shared [router.HealthStore] used by both the
// router (to skip circuited upstreams) and the admin endpoints (to
// display health).
func (s *State) Health() *router.HealthStore {
	return s.health
}

// Trace returns the [observability.TraceSink] the runtime writes
// lifecycle events to. May be a no-op sink if none was configured.
func (s *State) Trace() observability.TraceSink {
	return s.trace
}

// Number is the monotonically increasing generation counter. The first
// generation loaded by [NewState] is 1.
func (g *Generation) Number() uint64 {
	return g.number
}

// LoadedAt is the UTC time the generation's snapshot was first built.
func (g *Generation) LoadedAt() time.Time {
	return g.loadedAt
}

// Snapshot returns a deep clone of the generation's immutable
// [config.Snapshot]. Mutating the returned value does not affect the
// router or any other caller.
func (g *Generation) Snapshot() config.Snapshot {
	return cloneSnapshot(g.snapshot)
}

// Plan asks the router to resolve alias under req and packages the
// result. The returned plan is a deep clone.
func (g *Generation) Plan(alias string, req router.Requirements) (router.RoutePlan, error) {
	plan, err := g.router.Plan(alias, req)
	if err != nil {
		return router.RoutePlan{}, err
	}
	return cloneRoutePlan(plan), nil
}

// Execute runs a non-streaming chat completion. See [Executor.Execute]
// for the contract.
func (g *Generation) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	return g.executor.Execute(ctx, req)
}

// Stream runs a streaming chat completion. See [Executor.Stream] for
// the contract.
func (g *Generation) Stream(ctx context.Context, req ExecuteRequest) (StreamResult, error) {
	return g.executor.Stream(ctx, req)
}

// Reload reads the config from disk, validates it, and atomically
// swaps in a new [Generation]. Concurrent calls serialise on
// reloadMu; the returned [ReloadResult] describes the outcome. Changes
// to server.host or server.port are rejected (those require a process
// restart).
func (s *State) Reload(ctx context.Context, source ReloadSource, requestID string) ReloadResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.acquireReloadLock(ctx); err != nil {
		return s.reloadCanceledResult(err)
	}
	defer s.reloadMu.Unlock()
	if err := ctx.Err(); err != nil {
		return s.reloadCanceledResult(err)
	}

	start := time.Now()
	before := s.Current()
	s.trace.Emit(observability.TraceEvent{
		Event:                    observability.EventConfigReloadStarted,
		RequestID:                requestID,
		Client:                   string(source),
		ConfigGeneration:         before.number,
		PreviousConfigGeneration: before.number,
		ConfigPath:               s.configPath,
	})

	s.mu.Lock()
	s.meta.LastReloadAttemptAt = time.Now().UTC()
	s.mu.Unlock()

	cfg, err := config.LoadFile(s.configPath)
	if err != nil {
		return s.recordReloadFailure(source, requestID, before, classifyReloadLoadError(err), err, start)
	}
	if cfg.Server.Host != s.listenerHost || cfg.Server.Port != s.listenerPort {
		err := fmt.Errorf("server.host/server.port change requires restart: running %s:%d, config %s:%d", s.listenerHost, s.listenerPort, cfg.Server.Host, cfg.Server.Port)
		return s.recordReloadFailure(source, requestID, before, failure.ErrorListenerChangeRequiresRestart, err, start)
	}
	next, err := buildGeneration(before.number+1, cfg, s.adapters, s.health, s.trace, s.newHTTPClient)
	if err != nil {
		return s.recordReloadFailure(source, requestID, before, classifyReloadBuildError(err), err, start)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.current = next
	s.meta.Generation = next.number
	s.meta.ConfigLoadedAt = next.loadedAt
	s.meta.LastSuccessfulReloadAt = now
	s.meta.LastReloadErrorClass = ""
	s.meta.LastReloadError = ""
	s.meta.ReloadCount++
	s.mu.Unlock()

	s.trace.Emit(observability.TraceEvent{
		Event:                    observability.EventConfigReloadSucceeded,
		RequestID:                requestID,
		Client:                   string(source),
		ConfigGeneration:         next.number,
		PreviousConfigGeneration: before.number,
		NextConfigGeneration:     next.number,
		ConfigPath:               s.configPath,
		LatencyMS:                time.Since(start).Milliseconds(),
	})
	return ReloadResult{Success: true, Status: "reloaded", Generation: next.number, ConfigLoadedAt: next.loadedAt}
}

func buildGeneration(number uint64, cfg config.Config, adapters adapter.Registry, health *router.HealthStore, trace observability.TraceSink, newHTTPClient func(config.Config) *http.Client) (*Generation, error) {
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		return nil, err
	}
	rt := router.New(snapshot, health)
	executor := NewExecutor(Deps{
		Snapshot: snapshot,
		Router:   rt,
		Adapters: adapters,
		Health:   health,
		Trace:    trace,
		Client:   newHTTPClient(cfg),
	})
	return &Generation{number: number, loadedAt: snapshot.LoadedAt, snapshot: snapshot, router: rt, executor: executor}, nil
}

func (s *State) recordReloadFailure(source ReloadSource, requestID string, before *Generation, class failure.ErrorClass, err error, start time.Time) ReloadResult {
	message := sanitizeReloadError(err)
	s.mu.Lock()
	s.meta.LastFailedReloadAt = time.Now().UTC()
	s.meta.LastReloadErrorClass = string(class)
	s.meta.LastReloadError = message
	s.meta.FailedReloadCount++
	active := s.current
	s.mu.Unlock()

	s.trace.Emit(observability.TraceEvent{
		Event:                    observability.EventConfigReloadFailed,
		RequestID:                requestID,
		Client:                   string(source),
		ConfigGeneration:         active.number,
		PreviousConfigGeneration: before.number,
		ConfigPath:               s.configPath,
		LatencyMS:                time.Since(start).Milliseconds(),
		ErrorClass:               string(class),
		Reason:                   message,
	})
	return ReloadResult{Success: false, Status: "failed", Generation: before.number, ConfigLoadedAt: before.loadedAt, ErrorClass: class, Error: message}
}

func (s *State) acquireReloadLock(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.reloadMu.TryLock() {
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *State) reloadCanceledResult(err error) ReloadResult {
	current := s.Current()
	return ReloadResult{
		Success:        false,
		Status:         "failed",
		Generation:     current.number,
		ConfigLoadedAt: current.loadedAt,
		ErrorClass:     failure.ErrorConfigReloadFailed,
		Error:          sanitizeReloadError(err),
	}
}

func classifyReloadLoadError(err error) failure.ErrorClass {
	if os.IsNotExist(err) || os.IsPermission(err) || isPathError(err) {
		return failure.ErrorConfigReadFailed
	}
	var typeErr *yaml.TypeError
	var migrationErr config.MigrationError
	if errors.As(err, &typeErr) || errors.As(err, &migrationErr) || strings.Contains(err.Error(), "yaml:") {
		return failure.ErrorConfigValidationFailed
	}
	return failure.ErrorConfigReloadFailed
}

func isPathError(err error) bool {
	var pathErr *os.PathError
	return errors.As(err, &pathErr)
}

func classifyReloadBuildError(err error) failure.ErrorClass {
	var validationErr config.ValidationError
	if errors.As(err, &validationErr) || strings.Contains(err.Error(), "config validation failed") {
		return failure.ErrorConfigValidationFailed
	}
	return failure.ErrorConfigReloadFailed
}

func sanitizeReloadError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}

func cloneSnapshot(snapshot config.Snapshot) config.Snapshot {
	return config.Snapshot{
		LoadedAt:               snapshot.LoadedAt,
		Config:                 cloneConfig(snapshot.Config),
		ProvidersByID:          cloneProviderMap(snapshot.ProvidersByID),
		ModelsByID:             cloneModelMap(snapshot.ModelsByID),
		ModelsByExposedAlias:   cloneModelMap(snapshot.ModelsByExposedAlias),
		RoutesByAlias:          cloneRouteMap(snapshot.RoutesByAlias),
		RoutesByDiscoveryAlias: cloneRouteMap(snapshot.RoutesByDiscoveryAlias),
	}
}

func cloneRoutePlan(plan router.RoutePlan) router.RoutePlan {
	out := plan
	if plan.Targets != nil {
		out.Targets = make([]router.Target, len(plan.Targets))
		for i, target := range plan.Targets {
			out.Targets[i] = cloneTarget(target)
		}
	}
	return out
}

func cloneTarget(target router.Target) router.Target {
	return router.Target{
		Model:    target.Model,
		Provider: cloneProvider(target.Provider),
		Route:    cloneRoute(target.Route),
	}
}

func cloneConfig(cfg config.Config) config.Config {
	out := cfg
	out.Providers = cloneProviderSlice(cfg.Providers)
	out.Models = cloneModelSlice(cfg.Models)
	out.Routes = cloneRouteSlice(cfg.Routes)
	out.Profiles = cloneStringMap(cfg.Profiles)
	return out
}

func cloneProviderSlice(providers []config.ProviderConfig) []config.ProviderConfig {
	if providers == nil {
		return nil
	}
	out := make([]config.ProviderConfig, len(providers))
	for i, provider := range providers {
		out[i] = cloneProvider(provider)
	}
	return out
}

func cloneModelSlice(models []config.ModelConfig) []config.ModelConfig {
	if models == nil {
		return nil
	}
	out := make([]config.ModelConfig, len(models))
	copy(out, models)
	return out
}

func cloneRouteSlice(routes []config.RouteConfig) []config.RouteConfig {
	if routes == nil {
		return nil
	}
	out := make([]config.RouteConfig, len(routes))
	for i, route := range routes {
		out[i] = cloneRoute(route)
	}
	return out
}

func cloneProviderMap(providers map[string]config.ProviderConfig) map[string]config.ProviderConfig {
	if providers == nil {
		return nil
	}
	out := make(map[string]config.ProviderConfig, len(providers))
	for id, provider := range providers {
		out[id] = cloneProvider(provider)
	}
	return out
}

func cloneModelMap(models map[string]config.ModelConfig) map[string]config.ModelConfig {
	if models == nil {
		return nil
	}
	out := make(map[string]config.ModelConfig, len(models))
	for id, model := range models {
		out[id] = model
	}
	return out
}

func cloneRouteMap(routes map[string]config.RouteConfig) map[string]config.RouteConfig {
	if routes == nil {
		return nil
	}
	out := make(map[string]config.RouteConfig, len(routes))
	for alias, route := range routes {
		out[alias] = cloneRoute(route)
	}
	return out
}

func cloneProvider(provider config.ProviderConfig) config.ProviderConfig {
	out := provider
	out.Headers = cloneStringMap(provider.Headers)
	return out
}

func cloneRoute(route config.RouteConfig) config.RouteConfig {
	out := route
	if route.Targets != nil {
		out.Targets = make([]config.RouteTarget, len(route.Targets))
		copy(out.Targets, route.Targets)
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
