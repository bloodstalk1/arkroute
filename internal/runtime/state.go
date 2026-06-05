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

type ReloadSource string

const (
	ReloadSourceAdmin  ReloadSource = "admin"
	ReloadSourceSignal ReloadSource = "signal"
)

type StateDeps struct {
	ConfigPath    string
	ListenerHost  string
	ListenerPort  int
	Adapters      adapter.Registry
	Health        *router.HealthStore
	Trace         observability.TraceSink
	NewHTTPClient func(config.Config) *http.Client
}

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

type Generation struct {
	number   uint64
	loadedAt time.Time
	snapshot config.Snapshot
	router   *router.Router
	executor *Executor
}

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

func (s *State) Current() *Generation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *State) Status() ReloadStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

func (s *State) Health() *router.HealthStore {
	return s.health
}

func (s *State) Trace() observability.TraceSink {
	return s.trace
}

func (g *Generation) Number() uint64 {
	return g.number
}

func (g *Generation) LoadedAt() time.Time {
	return g.loadedAt
}

func (g *Generation) Snapshot() config.Snapshot {
	return cloneSnapshot(g.snapshot)
}

func (g *Generation) Plan(alias string, req router.Requirements) (router.RoutePlan, error) {
	plan, err := g.router.Plan(alias, req)
	if err != nil {
		return router.RoutePlan{}, err
	}
	return cloneRoutePlan(plan), nil
}

func (g *Generation) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	return g.executor.Execute(ctx, req)
}

func (g *Generation) Stream(ctx context.Context, req ExecuteRequest) (StreamResult, error) {
	return g.executor.Stream(ctx, req)
}

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
