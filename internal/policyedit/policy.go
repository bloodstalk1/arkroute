package policyedit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

var ErrModelNotFound = errors.New("model not found")

type OverrideInput struct {
	ModelID        string `json:"model_id"`
	AutoEnable    *bool  `json:"auto_enable,omitempty"`
	AutoEffort    string `json:"auto_effort,omitempty"`
	Replay        *bool  `json:"replay,omitempty"`
	OmitToolChoice *bool  `json:"omit_tool_choice,omitempty"`
}

type UserOverride struct {
	Exists         bool   `json:"exists"`
	PolicyID       string `json:"policy_id"`
	AutoEnable     *bool  `json:"auto_enable,omitempty"`
	AutoEffort     string `json:"auto_effort,omitempty"`
	Replay         *bool  `json:"replay,omitempty"`
	OmitToolChoice *bool  `json:"omit_tool_choice,omitempty"`
}

func StableModelPolicyID(modelID string) string {
	clean := strings.ToLower(strings.TrimSpace(modelID))
	var b strings.Builder
	lastDash := false
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		value = "model"
	}
	return "model-" + value + "-compat"
}

func UpsertModelOverride(cfg config.Config, input OverrideInput) (config.Config, config.CompatibilityPolicyConfig, error) {
	model, err := findModel(cfg, input.ModelID)
	if err != nil {
		return config.Config{}, config.CompatibilityPolicyConfig{}, err
	}
	if err := validateOverrideInput(input); err != nil {
		return config.Config{}, config.CompatibilityPolicyConfig{}, err
	}
	policy := config.CompatibilityPolicyConfig{
		ID: StableModelPolicyID(model.ID),
		Match: config.CompatibilityMatchConfig{
			ProviderIDs:    []string{model.ProviderID},
			UpstreamModels: []string{model.UpstreamModel},
		},
		Reasoning: config.CompatibilityReasoningConfig{
			AutoEnable:     cloneBool(input.AutoEnable),
			AutoEffort:     input.AutoEffort,
			Replay:         cloneBool(input.Replay),
			OmitToolChoice: cloneBool(input.OmitToolChoice),
		},
	}
	next := removePolicyByID(cfg.CompatibilityPolicies, policy.ID)
	next = append(next, policy)
	cfg.CompatibilityPolicies = next
	if err := cfg.Validate(); err != nil {
		return config.Config{}, config.CompatibilityPolicyConfig{}, err
	}
	return cfg, policy, nil
}

func DeleteModelOverride(cfg config.Config, modelID string) (config.Config, string, error) {
	if _, err := findModel(cfg, modelID); err != nil {
		return config.Config{}, "", err
	}
	policyID := StableModelPolicyID(modelID)
	cfg.CompatibilityPolicies = removePolicyByID(cfg.CompatibilityPolicies, policyID)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, "", err
	}
	return cfg, policyID, nil
}

func FindModelOverride(cfg config.Config, modelID string) UserOverride {
	policyID := StableModelPolicyID(modelID)
	for _, policy := range cfg.CompatibilityPolicies {
		if policy.ID == policyID {
			return UserOverride{
				Exists:         true,
				PolicyID:       policy.ID,
				AutoEnable:     cloneBool(policy.Reasoning.AutoEnable),
				AutoEffort:     policy.Reasoning.AutoEffort,
				Replay:         cloneBool(policy.Reasoning.Replay),
				OmitToolChoice: cloneBool(policy.Reasoning.OmitToolChoice),
			}
		}
	}
	return UserOverride{Exists: false, PolicyID: policyID}
}

func validateOverrideInput(input OverrideInput) error {
	if strings.TrimSpace(input.ModelID) == "" {
		return errors.New("model_id must be non-empty")
	}
	if input.AutoEffort != "" {
		switch input.AutoEffort {
		case "low", "medium", "high", "max":
		default:
			return fmt.Errorf("auto_effort must be low, medium, high, or max")
		}
	}
	if input.AutoEnable == nil && input.AutoEffort == "" && input.Replay == nil && input.OmitToolChoice == nil {
		return errors.New("at least one override field must be set")
	}
	return nil
}

func findModel(cfg config.Config, modelID string) (config.ModelConfig, error) {
	for _, model := range cfg.Models {
		if model.ID == modelID {
			return model, nil
		}
	}
	return config.ModelConfig{}, fmt.Errorf("%w: %s", ErrModelNotFound, modelID)
}

func removePolicyByID(policies []config.CompatibilityPolicyConfig, policyID string) []config.CompatibilityPolicyConfig {
	next := make([]config.CompatibilityPolicyConfig, 0, len(policies))
	for _, policy := range policies {
		if policy.ID != policyID {
			next = append(next, policy)
		}
	}
	return next
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
