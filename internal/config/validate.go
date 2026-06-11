package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	providercatalog "github.com/bloodstalk1/arkroute/internal/provider"
	"github.com/bloodstalk1/arkroute/internal/security"
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
	if !security.IsLoopbackHost(cfg.Server.Host) {
		fields["server.host"] = "must be loopback"
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		fields["server.port"] = "must be between 1 and 65535"
	}
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		fields["server.client_key"] = "must be non-empty"
	} else if cfg.Server.ClientKey == "[redacted]" {
		fields["server.client_key"] = "cannot contain [redacted] marker"
	}
	validateCompatibilityPolicies(fields, cfg.CompatibilityPolicies)

	providers := map[string]ProviderConfig{}
	enabledProviders := map[string]ProviderConfig{}
	for i, provider := range cfg.Providers {
		path := fmt.Sprintf("providers[%d]", i)
		if provider.ID == "" {
			fields[path+".id"] = "must be non-empty"
		} else if _, exists := providers[provider.ID]; exists {
			fields[path+".id"] = "must be unique"
		}
		if !providercatalog.IsAutoProtocol(provider.Type) && !providercatalog.IsKnownProtocol(provider.Type) {
			fields[path+".type"] = "unsupported provider type"
		}
		if parsed, err := url.ParseRequestURI(provider.BaseURL); err != nil {
			fields[path+".base_url"] = "must be an absolute URL"
		} else if parsed.Scheme != "https" && !isLoopbackURL(parsed) {
			fields[path+".base_url"] = "scheme must be https (or http for loopback)"
		}
		if provider.APIKey == "[redacted]" {
			fields[path+".api_key"] = "cannot contain [redacted] marker"
		}
		for k, v := range provider.Headers {
			if v == "[redacted]" {
				fields[fmt.Sprintf("%s.headers[%s]", path, k)] = "cannot contain [redacted] marker"
			}
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
		if strings.TrimSpace(model.Protocol) != "" && !providercatalog.IsKnownProtocol(model.Protocol) {
			fields[path+".protocol"] = "must be openai_compatible, anthropic, or gemini"
		}
		if model.ExposedAlias == "" {
			fields[path+".exposed_alias"] = "must be non-empty"
		} else if owner, exists := exposedAliases[model.ExposedAlias]; exists {
			fields[path+".exposed_alias"] = "must be unique; already used by " + owner
		}
		if model.Reasoning.Mode != "" && !validReasoningMode(model.Reasoning.Mode) {
			fields[path+".reasoning.mode"] = "must be passthrough, auto, custom, or adaptive"
		}
		if model.Reasoning.Effort != "" && !validReasoningEffort(model.Reasoning.Effort) {
			fields[path+".reasoning.effort"] = "must be low, medium, high, or max"
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
		if route.Strategy != "priority" && route.Strategy != "fallback" && route.Strategy != "round_robin" && route.Strategy != "weighted" {
			fields[path+".strategy"] = "must be priority, fallback, round_robin, or weighted"
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

func validateCompatibilityPolicies(fields map[string]string, policies []CompatibilityPolicyConfig) {
	seen := map[string]string{}
	for i, policy := range policies {
		prefix := fmt.Sprintf("compatibility_policies[%d]", i)
		if strings.TrimSpace(policy.ID) == "" {
			fields[prefix+".id"] = "must be non-empty"
		} else if owner, exists := seen[policy.ID]; exists {
			fields[prefix+".id"] = "must be unique; already used by " + owner
		}
		seen[policy.ID] = prefix
		validateCompatibilityMatch(fields, prefix+".match", policy.Match)
		if policy.Reasoning.AutoEffort != "" && !validReasoningEffort(policy.Reasoning.AutoEffort) {
			fields[prefix+".reasoning.auto_effort"] = "must be low, medium, high, or max"
		}
	}
}

func validateCompatibilityMatch(fields map[string]string, prefix string, match CompatibilityMatchConfig) {
	hasMatcher := len(match.ProviderIDs) > 0 ||
		len(match.ProviderIDContains) > 0 ||
		len(match.ProviderTypeContains) > 0 ||
		len(match.UpstreamModels) > 0 ||
		len(match.UpstreamModelContains) > 0 ||
		len(match.UpstreamModelPatterns) > 0
	if !hasMatcher {
		fields[prefix] = "must define at least one matcher"
	}
	validateNonEmptyList(fields, prefix+".provider_ids", match.ProviderIDs)
	validateNonEmptyList(fields, prefix+".provider_id_contains", match.ProviderIDContains)
	validateNonEmptyList(fields, prefix+".provider_type_contains", match.ProviderTypeContains)
	validateNonEmptyList(fields, prefix+".upstream_models", match.UpstreamModels)
	validateNonEmptyList(fields, prefix+".upstream_model_contains", match.UpstreamModelContains)
	for i, pattern := range match.UpstreamModelPatterns {
		path := fmt.Sprintf("%s.upstream_model_patterns[%d]", prefix, i)
		if strings.TrimSpace(pattern) == "" {
			fields[path] = "must be non-empty"
			continue
		}
		if _, err := wildcardPatternRegexp(strings.ToLower(pattern)); err != nil {
			fields[path] = "must be a valid glob pattern"
		}
	}
}

func validateNonEmptyList(fields map[string]string, prefix string, values []string) {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			fields[fmt.Sprintf("%s[%d]", prefix, i)] = "must be non-empty"
		}
	}
}

func validReasoningEffort(value string) bool {
	switch value {
	case "low", "medium", "high", "max":
		return true
	default:
		return false
	}
}

func validReasoningMode(value string) bool {
	switch value {
	case "passthrough", "auto", "custom", "adaptive":
		return true
	default:
		return false
	}
}

func isLoopbackURL(parsed *url.URL) bool {
	host := parsed.Hostname()
	return security.IsLoopbackHost(host)
}
