package router

import (
	"fmt"

	"bat.dev/arkrouter/internal/config"
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
	snapshot config.Snapshot
	health   *HealthStore
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
	targets := []Target{}
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
		targets = append(targets, Target{Model: model, Provider: provider, Route: route})
		if route.Strategy == "priority" {
			break
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("route %s has no target matching requested capabilities", route.Alias)
	}
	return targets, nil
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
