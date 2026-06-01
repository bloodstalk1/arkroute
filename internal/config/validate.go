package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type ValidationError struct {
	Fields map[string]string
}

func (e ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for key := range e.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+e.Fields[key])
	}
	return "config validation failed: " + strings.Join(parts, "; ")
}

func (cfg Config) Validate() error {
	fields := map[string]string{}
	if cfg.Version != CurrentVersion {
		fields["version"] = "must be 1"
	}
	if cfg.Server.Host != "127.0.0.1" && cfg.Server.Host != "localhost" && cfg.Server.Host != "::1" {
		fields["server.host"] = "must be loopback"
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		fields["server.port"] = "must be between 1 and 65535"
	}
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		fields["server.client_key"] = "must be non-empty"
	}

	providers := map[string]ProviderConfig{}
	enabledProviders := map[string]ProviderConfig{}
	for i, provider := range cfg.Providers {
		path := fmt.Sprintf("providers[%d]", i)
		if provider.ID == "" {
			fields[path+".id"] = "must be non-empty"
		} else if _, exists := providers[provider.ID]; exists {
			fields[path+".id"] = "must be unique"
		}
		if provider.Type != "openai_compatible" && provider.Type != "gemini" && provider.Type != "anthropic" {
			fields[path+".type"] = "unsupported provider type"
		}
		if _, err := url.ParseRequestURI(provider.BaseURL); err != nil {
			fields[path+".base_url"] = "must be an absolute URL"
		}
		providers[provider.ID] = provider
		if provider.Enabled {
			enabledProviders[provider.ID] = provider
		}
	}

	models := map[string]ModelConfig{}
	enabledModels := map[string]ModelConfig{}
	exposedAliases := map[string]string{}
	modelDiscoveryAliases := map[string]string{}
	for i, model := range cfg.Models {
		path := fmt.Sprintf("models[%d]", i)
		if model.ID == "" {
			fields[path+".id"] = "must be non-empty"
		} else if _, exists := models[model.ID]; exists {
			fields[path+".id"] = "must be unique"
		}
		if model.Enabled {
			if _, ok := enabledProviders[model.ProviderID]; !ok {
				fields[path+".provider_id"] = "must reference an enabled provider"
			}
		}
		if model.UpstreamModel == "" {
			fields[path+".upstream_model"] = "must be non-empty"
		}
		if model.ExposedAlias == "" {
			fields[path+".exposed_alias"] = "must be non-empty"
		} else if owner, exists := exposedAliases[model.ExposedAlias]; exists {
			fields[path+".exposed_alias"] = "must be unique; already used by " + owner
		}
		exposedAliases[model.ExposedAlias] = path
		validateDiscoveryAlias(fields, modelDiscoveryAliases, path+".claude_discovery_alias", model.ClaudeDiscoveryAlias)
		models[model.ID] = model
		if model.Enabled {
			enabledModels[model.ID] = model
		}
	}

	routeAliases := map[string]string{}
	routeDiscoveryAliases := map[string]string{}
	for i, route := range cfg.Routes {
		path := fmt.Sprintf("routes[%d]", i)
		if route.Alias == "" {
			fields[path+".alias"] = "must be non-empty"
		} else if owner, exists := routeAliases[route.Alias]; exists {
			fields[path+".alias"] = "must be unique; already used by " + owner
		}
		routeAliases[route.Alias] = path
		if route.Strategy != "priority" && route.Strategy != "fallback" {
			fields[path+".strategy"] = "must be priority or fallback"
		}
		if len(route.Targets) == 0 {
			fields[path+".targets"] = "must contain at least one target"
		}
		for j, target := range route.Targets {
			if route.Enabled && target.Enabled {
				if _, ok := enabledModels[target.ModelID]; !ok {
					fields[fmt.Sprintf("%s.targets[%d].model_id", path, j)] = "must reference an enabled model"
				}
			}
		}
		validateDiscoveryAlias(fields, routeDiscoveryAliases, path+".claude_discovery_alias", route.ClaudeDiscoveryAlias)
	}

	for name, alias := range cfg.Profiles {
		if _, routeOK := routeAliases[alias]; !routeOK {
			if _, modelOK := exposedAliases[alias]; !modelOK {
				fields["profiles."+name] = "must reference a route alias or exposed model alias"
			}
		}
	}
	if len(fields) > 0 {
		return ValidationError{Fields: fields}
	}
	return nil
}

func validateDiscoveryAlias(fields map[string]string, seen map[string]string, path string, value string) {
	if value == "" {
		return
	}
	if !strings.HasPrefix(value, "claude") && !strings.HasPrefix(value, "anthropic") {
		fields[path] = "must start with claude or anthropic"
	}
	if owner, exists := seen[value]; exists {
		fields[path] = "must be unique; already used by " + owner
	}
	seen[value] = path
}
