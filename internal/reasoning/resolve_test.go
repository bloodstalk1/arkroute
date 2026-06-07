package reasoning

import (
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

func TestResolveMergedAutoReasoningUsesAutoEffort(t *testing.T) {
	trueValue := true
	model := config.ModelConfig{
		Capabilities: config.Capabilities{Reasoning: false},
		Reasoning: config.ReasoningConfig{
			Mode:       "auto",
			AutoEnable: &trueValue,
			AutoEffort: "high",
		},
	}

	got := ResolveMerged(model, protocol.Request{})
	if !got.Enabled || got.Effort != "high" {
		t.Fatalf("behavior = %+v, want enabled high effort", got)
	}
	if got.FollowClaudeEffort {
		t.Fatalf("FollowClaudeEffort = true, want false for auto mode default")
	}
}

func TestResolveMergedPassthroughFollowsClaudeEffortByDefault(t *testing.T) {
	model := config.ModelConfig{
		Capabilities: config.Capabilities{Reasoning: true},
		Reasoning:   config.ReasoningConfig{Mode: "passthrough"},
	}
	req := protocol.Request{ReasoningEffort: "xhigh"}

	got := ResolveMerged(model, req)
	if !got.Enabled || got.Effort != "max" {
		t.Fatalf("behavior = %+v, want enabled max effort", got)
	}
	if !got.FollowClaudeEffort {
		t.Fatalf("FollowClaudeEffort = false, want true for passthrough default")
	}
}

func TestResolveMergedConfiguredDisableWins(t *testing.T) {
	falseValue := false
	trueValue := true
	model := config.ModelConfig{
		Capabilities: config.Capabilities{Reasoning: true},
		Reasoning: config.ReasoningConfig{
			Enabled:            &falseValue,
			Replay:             &falseValue,
			OmitToolChoice:     &trueValue,
			FollowClaudeEffort: &trueValue,
		},
	}
	req := protocol.Request{ReasoningEffort: "high"}

	got := ResolveMerged(model, req)
	if got.Enabled || !got.DisableRequest || got.Effort != "" {
		t.Fatalf("behavior = %+v, want hard disabled reasoning", got)
	}
	if got.Replay || !got.OmitToolChoice {
		t.Fatalf("behavior = %+v, want replay false and omit_tool_choice true", got)
	}
}

func TestResolveAppliesBuiltinDeepSeekV4Policy(t *testing.T) {
	provider := config.ProviderConfig{ID: "custom", Type: "openai_compatible"}
	model := config.ModelConfig{
		UpstreamModel: "provider/deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: false},
		Reasoning:     config.ReasoningConfig{Mode: "auto"},
	}

	got := Resolve(provider, model, protocol.Request{})
	if !got.Enabled || got.Effort != "max" || !got.Replay || !got.OmitToolChoice {
		t.Fatalf("behavior = %+v, want builtin DeepSeek V4 behavior", got)
	}
}
