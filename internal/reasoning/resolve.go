package reasoning

import (
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

type Behavior struct {
	Enabled            bool
	DisableRequest     bool
	Effort             string
	Replay             bool
	OmitToolChoice     bool
	FollowClaudeEffort bool
}

func Resolve(provider config.ProviderConfig, model config.ModelConfig, req protocol.Request) Behavior {
	return ResolveMerged(config.ApplyBuiltinCompatibilityPolicy(provider, model), req)
}

func ResolveMerged(model config.ModelConfig, req protocol.Request) Behavior {
	behavior := Behavior{
		Enabled:        model.Capabilities.Reasoning,
		Replay:         model.Capabilities.Reasoning,
		OmitToolChoice: model.Capabilities.Reasoning,
	}
	reasoningHardDisabled := false

	if model.Reasoning.Enabled != nil {
		behavior.Enabled = *model.Reasoning.Enabled
		behavior.DisableRequest = !*model.Reasoning.Enabled
		reasoningHardDisabled = !*model.Reasoning.Enabled
	}
	if model.Reasoning.Replay != nil {
		behavior.Replay = *model.Reasoning.Replay
	}
	if model.Reasoning.OmitToolChoice != nil {
		behavior.OmitToolChoice = *model.Reasoning.OmitToolChoice
	}
	if reasoningHardDisabled {
		behavior.Enabled = false
		behavior.DisableRequest = true
		behavior.Effort = ""
		behavior.FollowClaudeEffort = shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, false)
		return behavior
	}

	switch reasoningMode(model) {
	case "auto":
		applyAutoReasoning(&behavior, model)
		if model.Reasoning.Effort != "" {
			behavior.Effort = model.Reasoning.Effort
		}
		behavior.FollowClaudeEffort = shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, false)
		if behavior.FollowClaudeEffort {
			applyClaudeReasoning(&behavior, req)
		}
	case "custom":
		if model.Reasoning.Effort != "" {
			behavior.Enabled = true
			behavior.Effort = model.Reasoning.Effort
		}
		behavior.FollowClaudeEffort = shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, false)
		if behavior.FollowClaudeEffort {
			applyClaudeReasoning(&behavior, req)
		}
	case "adaptive":
		if model.Reasoning.Effort != "" {
			behavior.Enabled = true
			behavior.Effort = model.Reasoning.Effort
		}
		behavior.FollowClaudeEffort = shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, true)
		if behavior.FollowClaudeEffort {
			applyClaudeReasoning(&behavior, req)
		}
	default:
		if model.Reasoning.Effort != "" {
			behavior.Enabled = true
			behavior.Effort = model.Reasoning.Effort
		}
		behavior.FollowClaudeEffort = shouldFollowClaudeEffort(model.Reasoning.FollowClaudeEffort, true)
		if behavior.FollowClaudeEffort {
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

func applyAutoReasoning(behavior *Behavior, model config.ModelConfig) {
	if model.Reasoning.AutoEnable != nil && *model.Reasoning.AutoEnable {
		behavior.Enabled = true
	}
	if behavior.Enabled && behavior.Effort == "" {
		behavior.Effort = model.Reasoning.AutoEffort
		if behavior.Effort == "" {
			behavior.Effort = "max"
		}
	}
}

func applyClaudeReasoning(behavior *Behavior, req protocol.Request) {
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
