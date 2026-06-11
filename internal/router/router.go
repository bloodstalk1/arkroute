package router

import (
	"fmt"
	"math/rand"
	"sort"
	"sync/atomic"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type Requirements struct {
	Streaming bool
	Tools     bool
	Vision    bool
}

type Target struct {
	Model    config.ModelConfig
	Provider config.ProviderConfig
	Route    config.RouteConfig
}

type Router struct {
	snapshot     config.Snapshot
	health       *HealthStore
	roundCounter atomic.Uint64
}

func New(snapshot config.Snapshot, health *HealthStore) *Router {
	return &Router{snapshot: snapshot, health: health}
}

func (r *Router) Resolve(alias string, req Requirements) ([]Target, error) {
	if route, ok := r.snapshot.RoutesByAlias[alias]; ok {
		return r.resolveRoute(route, req)
	}
	if route, ok := r.snapshot.RoutesByDiscoveryAlias[alias]; ok {
		return r.resolveRoute(route, req)
	}
	if model, ok := r.snapshot.ModelsByExposedAlias[alias]; ok {
		provider := r.snapshot.ProvidersByID[model.ProviderID]
		if !supports(model.Capabilities, req) {
			return nil, fmt.Errorf("model %s does not support requested capabilities", model.ID)
		}
		return []Target{{Model: model, Provider: provider}}, nil
	}
	return nil, fmt.Errorf("model or route %q not found", alias)
}

func (r *Router) resolveRoute(route config.RouteConfig, req Requirements) ([]Target, error) {
	// Gather all matching targets.
	candidates := r.candidates(route, req)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("route %s has no target matching requested capabilities", route.Alias)
	}

	switch route.Strategy {
	case "priority":
		return r.selectPriority(candidates), nil
	case "fallback":
		return r.selectFallback(candidates), nil
	case "round_robin":
		return r.selectRoundRobin(route, candidates), nil
	case "weighted":
		return r.selectWeighted(route, candidates), nil
	default:
		return r.selectPriority(candidates), nil
	}
}

// candidates returns all enabled, capability-matching targets for a route.
func (r *Router) candidates(route config.RouteConfig, req Requirements) []Target {
	var targets []Target
	for _, routeTarget := range route.Targets {
		if !routeTarget.Enabled {
			continue
		}
		model, ok := r.snapshot.ModelsByID[routeTarget.ModelID]
		if !ok || !model.Enabled || !supports(model.Capabilities, req) {
			continue
		}
		provider, ok := r.snapshot.ProvidersByID[model.ProviderID]
		if !ok || !provider.Enabled {
			continue
		}
		// Skip target if circuit breaker is open.
		if r.health.IsCircuited(provider.ID, model.UpstreamModel) {
			continue
		}
		targets = append(targets, Target{Model: model, Provider: provider, Route: route})
	}
	return targets
}

func (r *Router) selectPriority(targets []Target) []Target {
	return targets[:1]
}

func (r *Router) selectFallback(targets []Target) []Target {
	return targets
}

func (r *Router) selectRoundRobin(route config.RouteConfig, targets []Target) []Target {
	if len(targets) <= 1 {
		return targets
	}
	// Use atomic counter to pick next target in rotation.
	n := r.roundCounter.Add(1) - 1
	idx := int(n % uint64(len(targets)))
	return []Target{targets[idx]}
}

func (r *Router) selectWeighted(route config.RouteConfig, targets []Target) []Target {
	if len(targets) <= 1 {
		return targets
	}
	// Build weighted list from target weights (default weight = 1).
	type weightedTarget struct {
		target Target
		weight int
	}
	var list []weightedTarget
	totalWeight := 0
	for i, t := range targets {
		w := 1
		if i < len(route.Targets) {
			if route.Targets[i].Weight > 0 {
				w = route.Targets[i].Weight
			}
		}
		totalWeight += w
		list = append(list, weightedTarget{target: t, weight: w})
	}
	if totalWeight == 0 {
		return []Target{targets[0]}
	}
	// Sort by weight descending, then pick based on random roll within totalWeight.
	sort.Slice(list, func(i, j int) bool { return list[i].weight > list[j].weight })
	roll := rand.Intn(totalWeight)
	cumulative := 0
	for _, wt := range list {
		cumulative += wt.weight
		if roll < cumulative {
			return []Target{wt.target}
		}
	}
	return []Target{targets[0]}
}

type RoutePlan struct {
	Alias        string
	Strategy     string
	Requirements Requirements
	Targets      []Target
}

func (r *Router) Plan(alias string, req Requirements) (RoutePlan, error) {
	targets, err := r.Resolve(alias, req)
	if err != nil {
		return RoutePlan{}, err
	}
	strategy := "priority"
	resolvedAlias := alias
	if len(targets) > 0 && targets[0].Route.Alias != "" {
		strategy = targets[0].Route.Strategy
		resolvedAlias = targets[0].Route.Alias
	}
	return RoutePlan{Alias: resolvedAlias, Strategy: strategy, Requirements: req, Targets: targets}, nil
}

func supports(cap config.Capabilities, req Requirements) bool {
	if req.Streaming && !cap.Streaming {
		return false
	}
	if req.Tools && !cap.Tools {
		return false
	}
	if req.Vision && !cap.Vision {
		return false
	}
	return true
}

