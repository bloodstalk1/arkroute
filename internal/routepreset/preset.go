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
	inputs := resolveInputs(req, preset)
	if err := checkConflicts(cfg, inputs.providerID, inputs.modelID, req.ConfirmOverwrite); err != nil {
		return config.Config{}, ApplySummary{}, err
	}
	cfg = removeExisting(cfg, inputs.providerID, inputs.modelID, req.ConfirmOverwrite)
	discovery := nextDiscoveryAlias(defaultDiscoveryAlias, cfg.Models)
	cfg = appendProvider(cfg, req, preset, inputs)
	cfg = appendReasoningPolicy(cfg, preset, inputs)
	cfg = appendModelEntry(cfg, preset, inputs, discovery)
	cfg = upsertRouteAndProfile(cfg, req, inputs)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, ApplySummary{}, err
	}
	return cfg, summaryFromInputs(inputs, preset, req), nil
}

const defaultDiscoveryAlias = "claude-sonnet-4-20250514"

type applyInputs struct {
	providerID   string
	providerName string
	routeAlias   string
	modelAlias   string
	modelID      string
}

func resolveInputs(req ApplyRequest, preset Preset) applyInputs {
	providerID := firstNonEmpty(req.ProviderID, preset.DefaultProviderID, preset.ID)
	providerName := firstNonEmpty(req.ProviderName, preset.Name)
	routeAlias := firstNonEmpty(req.RouteAlias, preset.DefaultRoute)
	modelAlias := preset.DefaultAlias
	modelID := normalizeID(providerID + "-" + modelAlias)
	return applyInputs{
		providerID:   providerID,
		providerName: providerName,
		routeAlias:   routeAlias,
		modelAlias:   modelAlias,
		modelID:      modelID,
	}
}

func appendProvider(cfg config.Config, req ApplyRequest, preset Preset, in applyInputs) config.Config {
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: in.providerID, Name: in.providerName, Type: preset.ProviderType, BaseURL: preset.BaseURL,
		APIKey: providerAPIKey(req, preset, in.providerID), Enabled: true,
	})
	return cfg
}

func nextDiscoveryAlias(candidate string, existing []config.ModelConfig) string {
	for _, m := range existing {
		if m.ClaudeDiscoveryAlias == candidate {
			return ""
		}
	}
	return candidate
}

func appendReasoningPolicy(cfg config.Config, preset Preset, in applyInputs) config.Config {
	var autoEnable, replay *bool
	if preset.AutoThinking {
		autoEnable = boolPtr(true)
	}
	if preset.ReasoningReplay {
		replay = boolPtr(true)
	}
	if autoEnable == nil && replay == nil {
		return cfg
	}
	policyID := compatpolicy.StableModelPolicyID(in.modelID)
	cfg.CompatibilityPolicies = compatpolicy.RemoveByID(cfg.CompatibilityPolicies, policyID)
	cfg.CompatibilityPolicies = append(cfg.CompatibilityPolicies, config.CompatibilityPolicyConfig{
		ID: policyID,
		Match: config.CompatibilityMatchConfig{
			ProviderIDs:    []string{in.providerID},
			UpstreamModels: []string{preset.UpstreamModel},
		},
		Reasoning: config.CompatibilityReasoningConfig{
			AutoEnable: autoEnable,
			Replay:     replay,
		},
	})
	return cfg
}

func appendModelEntry(cfg config.Config, preset Preset, in applyInputs, discoveryAlias string) config.Config {
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: in.modelID, ProviderID: in.providerID, UpstreamModel: preset.UpstreamModel,
		ExposedAlias: in.modelAlias, ClaudeDiscoveryAlias: discoveryAlias,
		DisplayName:  in.providerName + " " + preset.UpstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	})
	return cfg
}

func upsertRouteAndProfile(cfg config.Config, req ApplyRequest, in applyInputs) config.Config {
	cfg.Routes = upsertRoute(cfg.Routes, in.routeAlias, in.modelID, req.AppendToRoute)
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]string{}
	}
	if name := strings.TrimSpace(req.ProfileName); name != "" {
		cfg.Profiles[name] = in.routeAlias
	}
	return cfg
}

func summaryFromInputs(in applyInputs, preset Preset, req ApplyRequest) ApplySummary {
	return ApplySummary{
		ProviderID: in.providerID, ModelID: in.modelID, RouteAlias: in.routeAlias, ProfileName: req.ProfileName,
		AppendedRouteTarget: req.AppendToRoute,
	}
}

func boolPtr(value bool) *bool { return &value }

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
	cfg.Providers = filterBy(cfg.Providers, func(p config.ProviderConfig) bool { return p.ID != providerID })
	cfg.Models = filterBy(cfg.Models, func(m config.ModelConfig) bool { return m.ID != modelID })
	for i := range cfg.Routes {
		cfg.Routes[i].Targets = filterBy(cfg.Routes[i].Targets, func(t config.RouteTarget) bool { return t.ModelID != modelID })
	}
	cfg.CompatibilityPolicies = compatpolicy.RemoveByID(cfg.CompatibilityPolicies, compatpolicy.StableModelPolicyID(modelID))
	return cfg
}

func filterBy[T any](in []T, keep func(T) bool) []T {
	out := make([]T, 0, len(in))
	for _, item := range in {
		if keep(item) {
			out = append(out, item)
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
		Alias: alias, ClaudeDiscoveryAlias: defaultDiscoveryAlias, Strategy: "fallback",
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
	return compatpolicy.Slug(value, "")
}
