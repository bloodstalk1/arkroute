package setup

import (
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/compatpolicy"
	"github.com/bloodstalk1/arkroute/internal/config"
)

type ProviderSetup struct {
	PresetID       string `json:"preset_id"`
	ProviderName   string `json:"provider_name"`
	BaseURL        string `json:"base_url"`
	Type           string `json:"type"`
	APIKey         string `json:"api_key"`
	UpstreamModel  string `json:"upstream_model"`
	ExposedAlias   string `json:"exposed_alias"`
	RouteAlias     string `json:"route_alias"`
	ActivateClaude bool   `json:"activate_claude"`
}

func ApplyProviderSetup(cfg config.Config, input ProviderSetup) (config.Config, error) {
	preset, ok := findPreset(input.PresetID)
	if !ok {
		return config.Config{}, fmt.Errorf("unknown preset %q", input.PresetID)
	}
	providerID := preset.ID
	providerName := firstNonEmpty(input.ProviderName, preset.Name)
	baseURL := firstNonEmpty(input.BaseURL, preset.BaseURL)
	providerType := firstNonEmpty(input.Type, preset.Type)
	upstreamModel := firstNonEmpty(input.UpstreamModel, preset.DefaultModel)
	exposedAlias := firstNonEmpty(input.ExposedAlias, preset.DefaultAlias)
	routeAlias := firstNonEmpty(input.RouteAlias, preset.DefaultRoute)
	apiKey := input.APIKey
	if apiKey == "" {
		for _, p := range cfg.Providers {
			if p.ID == providerID {
				apiKey = p.APIKey
				break
			}
		}
	}

	modelID := providerID + "-" + normalizeID(exposedAlias)
	if providerID == "opencode-zen" {
		modelID = normalizeID(exposedAlias)
	}

	oldModelIDs := modelIDsForProvider(cfg.Models, providerID)
	cfg.Providers = upsertProvider(cfg.Providers, config.ProviderConfig{
		ID: providerID, Name: providerName, Type: providerType, BaseURL: baseURL,
		APIKey: apiKey, Headers: cloneStringMap(preset.Headers), Enabled: true,
	})
	cfg.Models = removeModelsForProvider(cfg.Models, providerID)
	cfg.Routes = removeRouteTargets(cfg.Routes, oldModelIDs)
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: modelID, ProviderID: providerID, UpstreamModel: upstreamModel, ExposedAlias: exposedAlias,
		ClaudeDiscoveryAlias: nextDiscoveryAlias(preset.DiscoveryAlias, cfg.Models), DisplayName: providerName + " " + upstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	})
	cfg.Routes = upsertRouteTarget(cfg.Routes, config.RouteConfig{
		Alias: routeAlias, ClaudeDiscoveryAlias: preset.DiscoveryAlias, Strategy: "fallback",
		Targets: []config.RouteTarget{{ModelID: modelID, Enabled: true}}, Enabled: true,
	})
	ensureProfiles(&cfg, routeAlias)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func RemoveProviderSetup(cfg config.Config, providerID string) (config.Config, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return config.Config{}, fmt.Errorf("provider id is required")
	}
	if !hasProvider(cfg.Providers, providerID) {
		return config.Config{}, fmt.Errorf("provider not found: %s", providerID)
	}
	modelIDs := modelIDsForProvider(cfg.Models, providerID)
	cfg.Providers = removeProviderByID(cfg.Providers, providerID)
	cfg.Models = removeModelsForProvider(cfg.Models, providerID)
	cfg.Routes = removeRouteTargets(cfg.Routes, modelIDs)
	cfg.Routes = removeEmptyRoutes(cfg.Routes)
	pruneProfiles(&cfg)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func upsertProvider(providers []config.ProviderConfig, provider config.ProviderConfig) []config.ProviderConfig {
	out := make([]config.ProviderConfig, 0, len(providers)+1)
	replaced := false
	for _, existing := range providers {
		if existing.ID == provider.ID {
			out = append(out, provider)
			replaced = true
			continue
		}
		out = append(out, existing)
	}
	if !replaced {
		out = append(out, provider)
	}
	return out
}

func hasProvider(providers []config.ProviderConfig, providerID string) bool {
	for _, provider := range providers {
		if provider.ID == providerID {
			return true
		}
	}
	return false
}

func removeProviderByID(providers []config.ProviderConfig, providerID string) []config.ProviderConfig {
	out := make([]config.ProviderConfig, 0, len(providers))
	for _, provider := range providers {
		if provider.ID != providerID {
			out = append(out, provider)
		}
	}
	return out
}

func modelIDsForProvider(models []config.ModelConfig, providerID string) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, model := range models {
		if model.ProviderID == providerID {
			ids[model.ID] = struct{}{}
		}
	}
	return ids
}

func removeModelsForProvider(models []config.ModelConfig, providerID string) []config.ModelConfig {
	out := make([]config.ModelConfig, 0, len(models))
	for _, model := range models {
		if model.ProviderID != providerID {
			out = append(out, model)
		}
	}
	return out
}

func removeRouteTargets(routes []config.RouteConfig, removed map[string]struct{}) []config.RouteConfig {
	if len(removed) == 0 {
		return routes
	}
	out := make([]config.RouteConfig, 0, len(routes))
	for _, route := range routes {
		targets := make([]config.RouteTarget, 0, len(route.Targets))
		for _, target := range route.Targets {
			if _, drop := removed[target.ModelID]; !drop {
				targets = append(targets, target)
			}
		}
		route.Targets = targets
		out = append(out, route)
	}
	return out
}

func upsertRouteTarget(routes []config.RouteConfig, route config.RouteConfig) []config.RouteConfig {
	modelID := route.Targets[0].ModelID
	out := make([]config.RouteConfig, 0, len(routes)+1)
	replacedRoute := false
	for _, existing := range routes {
		if existing.Alias != route.Alias {
			out = append(out, existing)
			continue
		}
		existing.ClaudeDiscoveryAlias = firstNonEmpty(existing.ClaudeDiscoveryAlias, route.ClaudeDiscoveryAlias)
		existing.Strategy = firstNonEmpty(existing.Strategy, route.Strategy)
		existing.Enabled = true
		targets := make([]config.RouteTarget, 0, len(existing.Targets)+1)
		replacedTarget := false
		for _, target := range existing.Targets {
			if target.ModelID == modelID {
				targets = append(targets, config.RouteTarget{ModelID: modelID, Enabled: true})
				replacedTarget = true
				continue
			}
			targets = append(targets, target)
		}
		if !replacedTarget {
			targets = append(targets, config.RouteTarget{ModelID: modelID, Enabled: true})
		}
		existing.Targets = targets
		out = append(out, existing)
		replacedRoute = true
	}
	if !replacedRoute {
		out = append(out, route)
	}
	return out
}

func removeEmptyRoutes(routes []config.RouteConfig) []config.RouteConfig {
	out := make([]config.RouteConfig, 0, len(routes))
	for _, route := range routes {
		if len(route.Targets) > 0 {
			out = append(out, route)
		}
	}
	return out
}

func ensureProfiles(cfg *config.Config, routeAlias string) {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]string{}
	}
	if strings.TrimSpace(cfg.Profiles["default"]) == "" {
		cfg.Profiles["default"] = routeAlias
	}
	if strings.TrimSpace(cfg.Profiles["best"]) == "" {
		cfg.Profiles["best"] = routeAlias
	}
}

func pruneProfiles(cfg *config.Config) {
	if len(cfg.Profiles) == 0 {
		return
	}
	valid := map[string]struct{}{}
	for _, route := range cfg.Routes {
		valid[route.Alias] = struct{}{}
	}
	for _, model := range cfg.Models {
		valid[model.ExposedAlias] = struct{}{}
	}
	for name, alias := range cfg.Profiles {
		if _, ok := valid[alias]; !ok {
			delete(cfg.Profiles, name)
		}
	}
	if len(cfg.Profiles) == 0 && len(cfg.Routes) > 0 {
		cfg.Profiles["default"] = cfg.Routes[0].Alias
		cfg.Profiles["best"] = cfg.Routes[0].Alias
	}
}


func findPreset(id string) (ProviderPreset, bool) {
	for _, preset := range Presets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return ProviderPreset{}, false
}


func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeID(value string) string {
	return compatpolicy.Slug(value, "")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func nextDiscoveryAlias(candidate string, existing []config.ModelConfig) string {
	for _, m := range existing {
		if m.ClaudeDiscoveryAlias == candidate {
			return ""
		}
	}
	return candidate
}

