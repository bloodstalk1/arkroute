package config

import (
	"regexp"
	"strings"
)

func ApplyCompatibilityPolicies(provider ProviderConfig, model ModelConfig, policies []CompatibilityPolicyConfig) ModelConfig {
	inspection := CompatibilityInspection{
		Model:            model,
		ReasoningSources: initialReasoningSources(model),
	}
	inspection.applyPolicies(provider, policies, CompatibilityPolicySourceUser)
	return inspection.Model
}

func ApplyBuiltinCompatibilityPolicy(provider ProviderConfig, model ModelConfig) ModelConfig {
	inspection := CompatibilityInspection{
		Model:            model,
		ReasoningSources: initialReasoningSources(model),
	}
	inspection.applyPolicies(provider, builtinCompatibilityPolicies(), CompatibilityPolicySourceBuiltin)
	return inspection.Model
}

func builtinCompatibilityPolicies() []CompatibilityPolicyConfig {
	trueValue := true
	return []CompatibilityPolicyConfig{
		{
			ID: "deepseek-v4-openai-compatible",
			Match: CompatibilityMatchConfig{
				UpstreamModelPatterns: []string{"*deepseek*v4*"},
			},
			Reasoning: CompatibilityReasoningConfig{
				AutoEnable:     &trueValue,
				AutoEffort:     "max",
				Replay:         &trueValue,
				OmitToolChoice: &trueValue,
			},
		},
		{
			ID: "reasoning-replay-provider-families",
			Match: CompatibilityMatchConfig{
				ProviderIDContains: []string{
					"deepseek",
					"siliconflow",
					"deepinfra",
					"fireworks",
					"together",
					"z.ai",
					"zai",
				},
			},
			Reasoning: CompatibilityReasoningConfig{Replay: &trueValue},
		},
		{
			ID: "reasoning-replay-model-families",
			Match: CompatibilityMatchConfig{
				UpstreamModelContains: []string{
					"deepseek",
					"kimi-k2",
					"glm-5.1",
				},
			},
			Reasoning: CompatibilityReasoningConfig{Replay: &trueValue},
		},
		{
			ID: "reasoning-replay-thinking-models",
			Match: CompatibilityMatchConfig{
				UpstreamModelPatterns: []string{
					"*qwen*think*",
					"*glm*think*",
				},
			},
			Reasoning: CompatibilityReasoningConfig{Replay: &trueValue},
		},
	}
}

func compatibilityPolicyMatches(provider ProviderConfig, model ModelConfig, match CompatibilityMatchConfig) bool {
	hasMatcher := len(match.ProviderIDs) > 0 ||
		len(match.ProviderIDContains) > 0 ||
		len(match.ProviderTypeContains) > 0 ||
		len(match.UpstreamModels) > 0 ||
		len(match.UpstreamModelContains) > 0 ||
		len(match.UpstreamModelPatterns) > 0
	if !hasMatcher {
		return false
	}
	if !equalsAnyLower(provider.ID, match.ProviderIDs) {
		return false
	}
	if !containsAnyLower(provider.ID, match.ProviderIDContains) {
		return false
	}
	if !containsAnyLower(provider.Type, match.ProviderTypeContains) {
		return false
	}
	if !equalsAnyLower(model.UpstreamModel, match.UpstreamModels) {
		return false
	}
	if !containsAnyLower(model.UpstreamModel, match.UpstreamModelContains) {
		return false
	}
	if !matchesAnyLowerPattern(model.UpstreamModel, match.UpstreamModelPatterns) {
		return false
	}
	return true
}

func equalsAnyLower(value string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && value == needle {
			return true
		}
	}
	return false
}

func containsAnyLower(value string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	value = strings.ToLower(value)
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func matchesAnyLowerPattern(value string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		re, err := wildcardPatternRegexp(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func wildcardPatternRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
