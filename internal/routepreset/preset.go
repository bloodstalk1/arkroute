package routepreset

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/compatpolicy"
	"github.com/bloodstalk1/arkroute/internal/config"
	setupcore "github.com/bloodstalk1/arkroute/internal/setup"
)

var ErrConflict = errors.New("preset target already exists")

type Preset struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	ProviderType      string              `json:"provider_type"`
	BaseURL           string              `json:"base_url"`
	UpstreamModel     string              `json:"upstream_model"`
	DefaultAlias      string              `json:"default_alias"`
	DefaultRoute      string              `json:"default_route"`
	DefaultProviderID string              `json:"default_provider_id"`
	DefaultEnvName    string              `json:"default_env_name"`
	Capabilities      config.Capabilities `json:"capabilities"`
	ReasoningReplay   bool                `json:"reasoning_replay"`
	AutoThinking      bool                `json:"auto_thinking"`
}

type ApplyRequest struct {
	PresetID         string `json:"preset_id"`
	ProviderID       string `json:"provider_id"`
	ProviderName     string `json:"provider_name"`
	APIKeyMode       string `json:"api_key_mode"`
	APIKey           string `json:"api_key"`
	EnvName          string `json:"env_name"`
	RouteAlias       string `json:"route_alias"`
	ProfileName      string `json:"profile_name"`
	AppendToRoute    bool   `json:"append_to_route"`
	ConfirmOverwrite bool   `json:"confirm_overwrite"`
}

type ApplySummary struct {
	ProviderID          string `json:"provider_id"`
	ModelID             string `json:"model_id"`
	RouteAlias          string `json:"route_alias"`
	ProfileName         string `json:"profile_name,omitempty"`
	AddedProvider       bool   `json:"added_provider"`
	AddedModel          bool   `json:"added_model"`
	AppendedRouteTarget bool   `json:"appended_route_target"`
}

func Presets() []Preset {
	caps := config.Capabilities{
		Streaming: true, Tools: true, ToolResults: true, SystemMessages: true,
		ContextWindow: 200000, MaxOutputTokens: 8192,
	}
	return []Preset{
		{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", ProviderType: "openai_compatible", BaseURL: "https://api.deepseek.com/v1", UpstreamModel: "deepseek-v4-pro", DefaultAlias: "deepseek-v4-pro", DefaultRoute: "sonnet", DefaultProviderID: "deepseek", DefaultEnvName: "DEEPSEEK_API_KEY", Capabilities: caps, ReasoningReplay: true, AutoThinking: true},
		{ID: "qwen-coder", Name: "Qwen Coder / Thinking", ProviderType: "openai_compatible", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", UpstreamModel: "qwen3-coder-plus", DefaultAlias: "qwen-coder", DefaultRoute: "sonnet", DefaultProviderID: "qwen", DefaultEnvName: "DASHSCOPE_API_KEY", Capabilities: caps, ReasoningReplay: true},
		{ID: "glm", Name: "GLM", ProviderType: "openai_compatible", BaseURL: "https://open.bigmodel.cn/api/paas/v4", UpstreamModel: "glm-4.6", DefaultAlias: "glm", DefaultRoute: "sonnet", DefaultProviderID: "glm", DefaultEnvName: "ZHIPU_API_KEY", Capabilities: caps, ReasoningReplay: true},
		{ID: "kimi-k2", Name: "Kimi K2", ProviderType: "openai_compatible", BaseURL: "https://api.moonshot.ai/v1", UpstreamModel: "kimi-k2-0905-preview", DefaultAlias: "kimi-k2", DefaultRoute: "sonnet", DefaultProviderID: "kimi", DefaultEnvName: "MOONSHOT_API_KEY", Capabilities: caps, ReasoningReplay: true},
		{ID: "minimax", Name: "MiniMax", ProviderType: "openai_compatible", BaseURL: "https://api.minimax.io/v1", UpstreamModel: "minimax-m2", DefaultAlias: "minimax", DefaultRoute: "sonnet", DefaultProviderID: "minimax", DefaultEnvName: "MINIMAX_API_KEY", Capabilities: caps},
		{ID: "claude-openrouter", Name: "Claude via OpenRouter", ProviderType: "openai_compatible", BaseURL: "https://openrouter.ai/api/v1", UpstreamModel: "anthropic/claude-sonnet-4.5", DefaultAlias: "sonnet-or", DefaultRoute: "sonnet", DefaultProviderID: "openrouter", DefaultEnvName: "OPENROUTER_API_KEY", Capabilities: caps},
		{ID: "generic-openai-compatible", Name: "Generic OpenAI-compatible", ProviderType: "openai_compatible", BaseURL: "https://example.com/v1", UpstreamModel: "provider/model", DefaultAlias: "custom-model", DefaultRoute: "sonnet", DefaultProviderID: "openai-compatible", DefaultEnvName: "OPENAI_API_KEY", Capabilities: caps},
	}
}

func Apply(cfg config.Config, req ApplyRequest) (config.Config, ApplySummary, error) {
	preset, ok := findPreset(req.PresetID)
	if !ok {
		return config.Config{}, ApplySummary{}, fmt.Errorf("unknown route preset %q", req.PresetID)
	}
	providerID := firstNonEmpty(req.ProviderID, preset.DefaultProviderID, preset.ID)
	providerName := firstNonEmpty(req.ProviderName, preset.Name)
	routeAlias := firstNonEmpty(req.RouteAlias, preset.DefaultRoute)
	modelAlias := preset.DefaultAlias
	modelID := normalizeID(providerID + "-" + modelAlias)
	if err := checkConflicts(cfg, providerID, modelID, req.ConfirmOverwrite); err != nil {
		return config.Config{}, ApplySummary{}, err
	}
	cfg = removeExisting(cfg, providerID, modelID, req.ConfirmOverwrite)
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: providerID, Name: providerName, Type: preset.ProviderType, BaseURL: preset.BaseURL,
		APIKey: providerAPIKey(req, preset, providerID), Enabled: true,
	})
	discoveryAlias := "claude-sonnet-4-20250514"
	for _, m := range cfg.Models {
		if m.ClaudeDiscoveryAlias == discoveryAlias {
			discoveryAlias = ""
			break
		}
	}
	var autoEnable *bool
	if preset.AutoThinking {
		val := true
		autoEnable = &val
	}
	var replay *bool
	if preset.ReasoningReplay {
		val := true
		replay = &val
	}
	if autoEnable != nil || replay != nil {
		policyID := compatpolicy.StableModelPolicyID(modelID)
		cfg.CompatibilityPolicies = removePolicyByID(cfg.CompatibilityPolicies, policyID)
		cfg.CompatibilityPolicies = append(cfg.CompatibilityPolicies, config.CompatibilityPolicyConfig{
			ID: policyID,
			Match: config.CompatibilityMatchConfig{
				ProviderIDs:    []string{providerID},
				UpstreamModels: []string{preset.UpstreamModel},
			},
			Reasoning: config.CompatibilityReasoningConfig{
				AutoEnable: autoEnable,
				Replay:     replay,
			},
		})
	}
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: modelID, ProviderID: providerID, UpstreamModel: preset.UpstreamModel,
		ExposedAlias: modelAlias, ClaudeDiscoveryAlias: discoveryAlias,
		DisplayName: providerName + " " + preset.UpstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	})
	cfg.Routes = upsertRoute(cfg.Routes, routeAlias, modelID, req.AppendToRoute)
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]string{}
	}
	if strings.TrimSpace(req.ProfileName) != "" {
		cfg.Profiles[strings.TrimSpace(req.ProfileName)] = routeAlias
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, ApplySummary{}, err
	}
	return cfg, ApplySummary{
		ProviderID: providerID, ModelID: modelID, RouteAlias: routeAlias, ProfileName: req.ProfileName,
		AddedProvider: true, AddedModel: true, AppendedRouteTarget: req.AppendToRoute,
	}, nil
}

func findPreset(id string) (Preset, bool) {
	for _, preset := range Presets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

func checkConflicts(cfg config.Config, providerID string, modelID string, confirm bool) error {
	if confirm {
		return nil
	}
	for _, provider := range cfg.Providers {
		if provider.ID == providerID {
			return fmt.Errorf("%w: provider %s already exists", ErrConflict, providerID)
		}
	}
	for _, model := range cfg.Models {
		if model.ID == modelID {
			return fmt.Errorf("%w: model %s already exists", ErrConflict, modelID)
		}
	}
	return nil
}

func removeExisting(cfg config.Config, providerID string, modelID string, confirm bool) config.Config {
	if !confirm {
		return cfg
	}
	cfg.Providers = filterProviders(cfg.Providers, providerID)
	cfg.Models = filterModels(cfg.Models, modelID)
	for i := range cfg.Routes {
		cfg.Routes[i].Targets = filterTargets(cfg.Routes[i].Targets, modelID)
	}
	cfg.CompatibilityPolicies = removePolicyByID(cfg.CompatibilityPolicies, compatpolicy.StableModelPolicyID(modelID))
	return cfg
}

func removePolicyByID(policies []config.CompatibilityPolicyConfig, id string) []config.CompatibilityPolicyConfig {
	out := policies[:0]
	for _, p := range policies {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}


func upsertRoute(routes []config.RouteConfig, alias string, modelID string, appendToRoute bool) []config.RouteConfig {
	for i := range routes {
		if routes[i].Alias == alias {
			if appendToRoute {
				routes[i].Strategy = "fallback"
				routes[i].Targets = append(routes[i].Targets, config.RouteTarget{ModelID: modelID, Enabled: true})
			} else {
				routes[i].Strategy = "fallback"
				routes[i].Targets = []config.RouteTarget{{ModelID: modelID, Enabled: true}}
			}
			routes[i].Enabled = true
			return routes
		}
	}
	return append(routes, config.RouteConfig{
		Alias: alias, ClaudeDiscoveryAlias: "claude-sonnet-4-20250514", Strategy: "fallback",
		Targets: []config.RouteTarget{{ModelID: modelID, Enabled: true}}, Enabled: true,
	})
}

func providerAPIKey(req ApplyRequest, preset Preset, providerID string) string {
	if req.APIKeyMode == setupcore.APIKeyModeConfig {
		return req.APIKey
	}
	return "env:" + firstNonEmpty(req.EnvName, preset.DefaultEnvName, setupcore.EnvNameForProvider(providerID))
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

func filterProviders(in []config.ProviderConfig, providerID string) []config.ProviderConfig {
	out := in[:0]
	for _, item := range in {
		if item.ID != providerID {
			out = append(out, item)
		}
	}
	return out
}

func filterModels(in []config.ModelConfig, modelID string) []config.ModelConfig {
	out := in[:0]
	for _, item := range in {
		if item.ID != modelID {
			out = append(out, item)
		}
	}
	return out
}

func filterTargets(in []config.RouteTarget, modelID string) []config.RouteTarget {
	out := in[:0]
	for _, item := range in {
		if item.ModelID != modelID {
			out = append(out, item)
		}
	}
	return out
}
