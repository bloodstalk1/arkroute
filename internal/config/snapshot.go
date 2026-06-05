package config

import (
	"os"
	"strings"
	"time"
)

type Snapshot struct {
	LoadedAt               time.Time
	Config                 Config
	ProvidersByID          map[string]ProviderConfig
	ModelsByID             map[string]ModelConfig
	ModelsByExposedAlias   map[string]ModelConfig
	RoutesByAlias          map[string]RouteConfig
	RoutesByDiscoveryAlias map[string]RouteConfig
}

func BuildSnapshot(cfg Config) (Snapshot, error) {
	if err := cfg.Validate(); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{
		LoadedAt:               time.Now().UTC(),
		Config:                 cfg,
		ProvidersByID:          map[string]ProviderConfig{},
		ModelsByID:             map[string]ModelConfig{},
		ModelsByExposedAlias:   map[string]ModelConfig{},
		RoutesByAlias:          map[string]RouteConfig{},
		RoutesByDiscoveryAlias: map[string]RouteConfig{},
	}
	for _, provider := range cfg.Providers {
		if provider.Enabled {
			provider.APIKey = resolveEnv(provider.APIKey)
			s.ProvidersByID[provider.ID] = provider
		}
	}
	for _, model := range cfg.Models {
		if model.Enabled {
			s.ModelsByID[model.ID] = model
			s.ModelsByExposedAlias[model.ExposedAlias] = model
		}
	}
	for _, route := range cfg.Routes {
		if route.Enabled {
			s.RoutesByAlias[route.Alias] = route
			if route.ClaudeDiscoveryAlias != "" {
				s.RoutesByDiscoveryAlias[route.ClaudeDiscoveryAlias] = route
			}
		}
	}
	return s, nil
}

func resolveEnv(value string) string {
	if strings.HasPrefix(value, "env:") {
		return os.Getenv(strings.TrimPrefix(value, "env:"))
	}
	return value
}
