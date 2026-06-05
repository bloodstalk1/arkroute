package openai

import (
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

type reasoningBehavior struct {
	Enabled        bool
	DisableRequest bool
	Effort         string
	Replay         bool
	OmitToolChoice bool
}

func resolveReasoning(provider config.ProviderConfig, model config.ModelConfig, req protocol.Request) reasoningBehavior {
	behavior := reasoningBehavior{
		Enabled:        model.Capabilities.Reasoning,
		Replay:         model.Capabilities.Reasoning,
		OmitToolChoice: model.Capabilities.Reasoning,
	}
	reasoningHardDisabled := false
	replayExplicit := false
	omitToolChoiceExplicit := false

	if model.Reasoning.Enabled != nil {
		behavior.Enabled = *model.Reasoning.Enabled
		behavior.DisableRequest = !*model.Reasoning.Enabled
		reasoningHardDisabled = !*model.Reasoning.Enabled
	}
	if model.Reasoning.Replay != nil {
		behavior.Replay = *model.Reasoning.Replay
		replayExplicit = true
	}
	if model.Reasoning.OmitToolChoice != nil {
		behavior.OmitToolChoice = *model.Reasoning.OmitToolChoice
		omitToolChoiceExplicit = true
	}
	if !replayExplicit && requiresReasoningReplay(provider.ID, model.UpstreamModel) {
		behavior.Replay = true
	}
	if !omitToolChoiceExplicit && omitsToolChoiceByDefault(provider.ID, model.UpstreamModel) {
		behavior.OmitToolChoice = true
	}
	if reasoningHardDisabled {
		behavior.Enabled = false
		behavior.DisableRequest = true
		behavior.Effort = ""
		return behavior
	}

	switch reasoningMode(model) {
	case "auto":
		applyAutoReasoning(&behavior, provider, model)
		if model.Reasoning.Effort != "" {
			behavior.Effort = model.Reasoning.Effort
		}
		if shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, false) {
			applyClaudeReasoning(&behavior, req)
		}
	case "custom":
		if model.Reasoning.Effort != "" {
			behavior.Enabled = true
			behavior.Effort = model.Reasoning.Effort
		}
		if shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, false) {
			applyClaudeReasoning(&behavior, req)
		}
	case "adaptive":
		if model.Reasoning.Effort != "" {
			behavior.Enabled = true
			behavior.Effort = model.Reasoning.Effort
		}
		if shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, true) {
			applyClaudeReasoning(&behavior, req)
		}
	default:
		if model.Reasoning.Effort != "" {
			behavior.Enabled = true
			behavior.Effort = model.Reasoning.Effort
		}
		if shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, true) {
			applyClaudeReasoning(&behavior, req)
		}
	}

	if !behavior.Enabled {
		behavior.Effort = ""
	}
	return behavior
}

func reasoningMode(model config.ModelConfig) string {
	if model.Reasoning.Mode != "" {
		return model.Reasoning.Mode
	}
	return "passthrough"
}

func applyAutoReasoning(behavior *reasoningBehavior, provider config.ProviderConfig, model config.ModelConfig) {
	if enablesThinkingByDefault(provider.ID, model.UpstreamModel) {
		behavior.Enabled = true
	}
	if requiresReasoningReplay(provider.ID, model.UpstreamModel) {
		behavior.Replay = true
	}
	if omitsToolChoiceByDefault(provider.ID, model.UpstreamModel) {
		behavior.OmitToolChoice = true
	}
	if behavior.Enabled && behavior.Effort == "" {
		behavior.Effort = "max"
	}
}

func applyClaudeReasoning(behavior *reasoningBehavior, req protocol.Request) {
	if strings.EqualFold(req.Thinking.Type, "disabled") {
		behavior.Enabled = false
		behavior.DisableRequest = true
		behavior.Effort = ""
		return
	}
	if req.Thinking.Type != "" {
		behavior.Enabled = true
	}
	if effort := normalizeReasoningEffort(req.ReasoningEffort); effort != "" {
		behavior.Enabled = true
		behavior.Effort = effort
		return
	}
	if req.Thinking.BudgetTokens > 0 {
		behavior.Enabled = true
		behavior.Effort = effortFromThinkingBudget(req.Thinking.BudgetTokens)
	}
}

func shouldFollowClaudeEffort(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func normalizeReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high", "max":
		return strings.ToLower(strings.TrimSpace(value))
	case "xhigh", "ultracode":
		return "max"
	default:
		return ""
	}
}

func effortFromThinkingBudget(tokens int) string {
	switch {
	case tokens <= 1024:
		return "low"
	case tokens <= 8192:
		return "medium"
	case tokens <= 20000:
		return "high"
	default:
		return "max"
	}
}

func enablesThinkingByDefault(providerID string, upstreamModel string) bool {
	value := reasoningMatchValue(providerID, upstreamModel)
	return strings.Contains(value, "deepseek-v4")
}

func omitsToolChoiceByDefault(providerID string, upstreamModel string) bool {
	value := reasoningMatchValue(providerID, upstreamModel)
	return strings.Contains(value, "deepseek-v4")
}

func requiresReasoningReplay(providerID string, upstreamModel string) bool {
	value := reasoningMatchValue(providerID, upstreamModel)
	if strings.Contains(value, "deepseek") ||
		strings.Contains(value, "siliconflow") ||
		strings.Contains(value, "deepinfra") ||
		strings.Contains(value, "fireworks") ||
		strings.Contains(value, "together") ||
		strings.Contains(value, "kimi-k2") ||
		strings.Contains(value, "z.ai") ||
		strings.Contains(value, "zai") ||
		strings.Contains(value, "glm-5.1") {
		return true
	}
	if strings.Contains(value, "qwen") && strings.Contains(value, "think") {
		return true
	}
	if strings.Contains(value, "glm") && strings.Contains(value, "think") {
		return true
	}
	return false
}

func reasoningMatchValue(providerID string, upstreamModel string) string {
	return strings.ToLower(providerID + " " + upstreamModel)
}
