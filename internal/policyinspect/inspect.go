package policyinspect

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/policyedit"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	providercatalog "github.com/bloodstalk1/arkroute/internal/provider"
	"github.com/bloodstalk1/arkroute/internal/reasoning"
)

var (
	ErrModelNotFound    = errors.New("model not found")
	ErrProviderNotFound = errors.New("provider not found")
)

type Inspection struct {
	SchemaVersion     int                                        `json:"schema_version"`
	ModelID           string                                     `json:"model_id"`
	ProviderID        string                                     `json:"provider_id"`
	ProviderType      string                                     `json:"provider_type"`
	UpstreamModel     string                                     `json:"upstream_model"`
	Protocol          string                                     `json:"protocol"`
	MatchedPolicies   []config.CompatibilityPolicyMatch          `json:"matched_policies"`
	ResolvedReasoning ResolvedReasoning                          `json:"resolved_reasoning"`
	ReasoningSources  map[string]config.CompatibilityFieldSource `json:"reasoning_sources"`
	Explain           []string                                   `json:"explain"`
	UserOverride      policyedit.UserOverride                    `json:"user_override"`
}

type ResolvedReasoning struct {
	Enabled            bool   `json:"enabled"`
	Effort             string `json:"effort"`
	AutoEnable         bool   `json:"auto_enable"`
	AutoEffort         string `json:"auto_effort"`
	Replay             bool   `json:"replay"`
	OmitToolChoice     bool   `json:"omit_tool_choice"`
	FollowClaudeEffort bool   `json:"follow_claude_effort"`
}

func InspectModel(cfg config.Config, modelID string) (Inspection, error) {
	model, ok := findModel(cfg, modelID)
	if !ok {
		return Inspection{}, fmt.Errorf("%w: %s", ErrModelNotFound, modelID)
	}
	provider, ok := findProvider(cfg, model.ProviderID)
	if !ok {
		return Inspection{}, fmt.Errorf("%w: %s", ErrProviderNotFound, model.ProviderID)
	}
	compat := config.InspectCompatibilityPolicies(provider, model, cfg.CompatibilityPolicies)
	behavior := reasoning.ResolveMerged(compat.Model, protocol.Request{})
	protocolName := resolveProtocol(provider, compat.Model)

	return Inspection{
		SchemaVersion:     1,
		ModelID:           compat.Model.ID,
		ProviderID:        provider.ID,
		ProviderType:      provider.Type,
		UpstreamModel:     compat.Model.UpstreamModel,
		Protocol:          protocolName,
		MatchedPolicies:   compat.MatchedPolicies,
		ResolvedReasoning: resolvedReasoning(compat.Model, behavior),
		ReasoningSources:  compat.ReasoningSources,
		Explain:           compat.Explain,
		UserOverride:      policyedit.FindModelOverride(cfg, compat.Model.ID),
	}, nil
}

func (i Inspection) String() string {
	data, err := json.Marshal(i)
	if err != nil {
		type raw Inspection
		return fmt.Sprintf("%+v", raw(i))
	}
	return string(data)
}

func findModel(cfg config.Config, modelID string) (config.ModelConfig, bool) {
	for _, model := range cfg.Models {
		if model.ID == modelID {
			return model, true
		}
	}
	return config.ModelConfig{}, false
}

func findProvider(cfg config.Config, providerID string) (config.ProviderConfig, bool) {
	for _, provider := range cfg.Providers {
		if provider.ID == providerID {
			return provider, true
		}
	}
	return config.ProviderConfig{}, false
}

func resolveProtocol(provider config.ProviderConfig, model config.ModelConfig) string {
	return providercatalog.DefaultResolver().Resolve(providercatalog.ProviderRef{
		ID:      provider.ID,
		Name:    provider.Name,
		Type:    provider.Type,
		BaseURL: provider.BaseURL,
	}, providercatalog.ModelRef{
		Protocol:      model.Protocol,
		UpstreamModel: model.UpstreamModel,
	})
}

func resolvedReasoning(model config.ModelConfig, behavior reasoning.Behavior) ResolvedReasoning {
	return ResolvedReasoning{
		Enabled:            behavior.Enabled,
		Effort:             behavior.Effort,
		AutoEnable:         boolValue(model.Reasoning.AutoEnable),
		AutoEffort:         model.Reasoning.AutoEffort,
		Replay:             behavior.Replay,
		OmitToolChoice:     behavior.OmitToolChoice,
		FollowClaudeEffort: behavior.FollowClaudeEffort,
	}
}

func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
