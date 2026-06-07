package config

import "fmt"

const (
	CompatibilityPolicySourceUser    = "user"
	CompatibilityPolicySourceBuiltin = "builtin"
	CompatibilityPolicySourceModel   = "model"
	CompatibilityPolicySourceDefault = "capability_default"
)

type CompatibilityPolicyMatch struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type CompatibilityFieldSource struct {
	Source   string `json:"source"`
	PolicyID string `json:"policy_id,omitempty"`
	Reason   string `json:"reason"`
}

type CompatibilityInspection struct {
	Model            ModelConfig                         `json:"-"`
	MatchedPolicies  []CompatibilityPolicyMatch          `json:"matched_policies"`
	ReasoningSources map[string]CompatibilityFieldSource `json:"reasoning_sources"`
	Explain          []string                            `json:"explain"`
}

func InspectCompatibilityPolicies(provider ProviderConfig, model ModelConfig, policies []CompatibilityPolicyConfig) CompatibilityInspection {
	inspection := CompatibilityInspection{
		Model:            model,
		ReasoningSources: initialReasoningSources(model),
	}
	inspection.applyPolicies(provider, policies, CompatibilityPolicySourceUser)
	inspection.applyPolicies(provider, builtinCompatibilityPolicies(), CompatibilityPolicySourceBuiltin)
	return inspection
}

func initialReasoningSources(model ModelConfig) map[string]CompatibilityFieldSource {
	sources := map[string]CompatibilityFieldSource{
		"enabled":              {Source: CompatibilityPolicySourceDefault, Reason: "capabilities.reasoning default"},
		"effort":               {Source: CompatibilityPolicySourceDefault, Reason: "empty effort default"},
		"auto_enable":          {Source: CompatibilityPolicySourceDefault, Reason: "unset auto_enable default"},
		"auto_effort":          {Source: CompatibilityPolicySourceDefault, Reason: "unset auto_effort default"},
		"replay":               {Source: CompatibilityPolicySourceDefault, Reason: "capabilities.reasoning default"},
		"omit_tool_choice":     {Source: CompatibilityPolicySourceDefault, Reason: "capabilities.reasoning default"},
		"follow_claude_effort": {Source: CompatibilityPolicySourceDefault, Reason: "mode default"},
	}
	if model.Reasoning.Enabled != nil {
		sources["enabled"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.enabled"}
	}
	if model.Reasoning.Effort != "" {
		sources["effort"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.effort"}
	}
	if model.Reasoning.AutoEnable != nil {
		sources["auto_enable"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.auto_enable"}
	}
	if model.Reasoning.AutoEffort != "" {
		sources["auto_effort"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.auto_effort"}
	}
	if model.Reasoning.Replay != nil {
		sources["replay"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.replay"}
	}
	if model.Reasoning.OmitToolChoice != nil {
		sources["omit_tool_choice"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.omit_tool_choice"}
	}
	if model.Reasoning.FollowClaudeEffort != nil {
		sources["follow_claude_effort"] = CompatibilityFieldSource{Source: CompatibilityPolicySourceModel, Reason: "models[].reasoning.follow_claude_effort"}
	}
	return sources
}

func (i *CompatibilityInspection) applyPolicies(provider ProviderConfig, policies []CompatibilityPolicyConfig, source string) {
	for _, policy := range policies {
		if !compatibilityPolicyMatches(provider, i.Model, policy.Match) {
			continue
		}
		i.MatchedPolicies = append(i.MatchedPolicies, CompatibilityPolicyMatch{ID: policy.ID, Source: source})
		i.applyPolicyReasoning(policy, source)
	}
}

func (i *CompatibilityInspection) applyPolicyReasoning(policy CompatibilityPolicyConfig, source string) {
	i.applyPolicyBool("auto_enable", policy.ID, source, &i.Model.Reasoning.AutoEnable, policy.Reasoning.AutoEnable)
	i.applyPolicyString("auto_effort", policy.ID, source, &i.Model.Reasoning.AutoEffort, policy.Reasoning.AutoEffort)
	i.applyPolicyBool("replay", policy.ID, source, &i.Model.Reasoning.Replay, policy.Reasoning.Replay)
	i.applyPolicyBool("omit_tool_choice", policy.ID, source, &i.Model.Reasoning.OmitToolChoice, policy.Reasoning.OmitToolChoice)
}

func (i *CompatibilityInspection) applyPolicyBool(field, policyID, source string, current **bool, incoming *bool) {
	if incoming == nil {
		return
	}
	if *current == nil {
		value := *incoming
		*current = &value
		i.ReasoningSources[field] = CompatibilityFieldSource{
			Source:   source,
			PolicyID: policyID,
			Reason:   fmt.Sprintf("%s policy %s sets %s", source, policyID, field),
		}
		i.Explain = append(i.Explain, fmt.Sprintf("%s policy %s sets %s", source, policyID, field))
		return
	}
	i.Explain = append(i.Explain, fmt.Sprintf("%s overrides policy %s %s", i.ReasoningSources[field].Reason, policyID, field))
}

func (i *CompatibilityInspection) applyPolicyString(field, policyID, source string, current *string, incoming string) {
	if incoming == "" {
		return
	}
	if *current == "" {
		*current = incoming
		i.ReasoningSources[field] = CompatibilityFieldSource{
			Source:   source,
			PolicyID: policyID,
			Reason:   fmt.Sprintf("%s policy %s sets %s", source, policyID, field),
		}
		i.Explain = append(i.Explain, fmt.Sprintf("%s policy %s sets %s", source, policyID, field))
		return
	}
	i.Explain = append(i.Explain, fmt.Sprintf("%s overrides policy %s %s", i.ReasoningSources[field].Reason, policyID, field))
}
