package setup

import (
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type ProviderSetup struct {
	PresetID       string `json:"preset_id"`
	ProviderName   string `json:"provider_name"`
	BaseURL        string `json:"base_url"`
	Type           string `json:"type"`
	APIKeyMode     string `json:"api_key_mode"`
	APIKey         string `json:"api_key"`
	EnvName        string `json:"env_name"`
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
	envName := firstNonEmpty(input.EnvName, EnvNameForProvider(providerID))
	apiKey := providerAPIKey(input.APIKeyMode, input.APIKey, envName)

	modelID := providerID + "-" + normalizeID(exposedAlias)
	if providerID == "opencode-zen" {
		modelID = normalizeID(exposedAlias)
	}
	cfg.Providers = []config.ProviderConfig{{
		ID: providerID, Name: providerName, Type: providerType, BaseURL: baseURL,
		APIKey: apiKey, Headers: cloneStringMap(preset.Headers), Enabled: true,
	}}
	cfg.Models = []config.ModelConfig{{
		ID: modelID, ProviderID: providerID, UpstreamModel: upstreamModel, ExposedAlias: exposedAlias,
		ClaudeDiscoveryAlias: preset.DiscoveryAlias, DisplayName: providerName + " " + upstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	}}
	cfg.Routes = []config.RouteConfig{{
		Alias: routeAlias, ClaudeDiscoveryAlias: preset.DiscoveryAlias, Strategy: "fallback",
		Targets: []config.RouteTarget{{ModelID: modelID, Enabled: true}}, Enabled: true,
	}}
	cfg.Profiles = map[string]string{"default": routeAlias, "best": routeAlias}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func findPreset(id string) (ProviderPreset, bool) {
	for _, preset := range Presets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return ProviderPreset{}, false
}

func providerAPIKey(mode string, raw string, envName string) string {
	if mode == APIKeyModeConfig {
		return raw
	}
	return "env:" + envName
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
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
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
