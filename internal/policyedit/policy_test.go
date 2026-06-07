package policyedit_test

import (
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/policyedit"
	"github.com/bloodstalk1/arkroute/internal/policyinspect"
)

func TestStableModelPolicyIDSanitizesModelID(t *testing.T) {
	got := policyedit.StableModelPolicyID("DeepSeek/V4 Pro++")
	want := "model-deepseek-v4-pro-compat"
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
}

func TestUpsertModelOverrideDisablesBuiltinDeepSeekV4AutoThinking(t *testing.T) {
	falseValue := false
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].UpstreamModel = "deepseek-v4-pro"
	cfg.Models[0].Capabilities.Reasoning = false
	cfg.Routes[0].Targets[0].ModelID = "deepseek-v4-pro"

	updated, policy, err := policyedit.UpsertModelOverride(cfg, policyedit.OverrideInput{
		ModelID:        "deepseek-v4-pro",
		AutoEnable:    &falseValue,
		Replay:        &falseValue,
		OmitToolChoice: &falseValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.ID != "model-deepseek-v4-pro-compat" {
		t.Fatalf("policy id = %q", policy.ID)
	}
	inspection, err := policyinspect.InspectModel(updated, "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ResolvedReasoning.AutoEnable || inspection.ResolvedReasoning.Enabled {
		t.Fatalf("resolved reasoning = %+v, want auto thinking disabled", inspection.ResolvedReasoning)
	}
	if inspection.ResolvedReasoning.Replay || inspection.ResolvedReasoning.OmitToolChoice {
		t.Fatalf("resolved reasoning = %+v, want replay and omit_tool_choice disabled", inspection.ResolvedReasoning)
	}
}

func TestUpsertModelOverrideRejectsInvalidAutoEffort(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	if _, _, err := policyedit.UpsertModelOverride(cfg, policyedit.OverrideInput{
		ModelID:    cfg.Models[0].ID,
		AutoEffort: "ultracode",
	}); err == nil {
		t.Fatal("UpsertModelOverride error = nil, want invalid effort error")
	}
}

func TestDeleteModelOverrideRemovesGeneratedPolicy(t *testing.T) {
	trueValue := true
	cfg := config.MinimalValidConfig("local-key")
	updated, _, err := policyedit.UpsertModelOverride(cfg, policyedit.OverrideInput{
		ModelID:    cfg.Models[0].ID,
		AutoEnable: &trueValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, policyID, err := policyedit.DeleteModelOverride(updated, cfg.Models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if policyID != policyedit.StableModelPolicyID(cfg.Models[0].ID) {
		t.Fatalf("policy id = %q", policyID)
	}
	if len(deleted.CompatibilityPolicies) != 0 {
		t.Fatalf("compatibility policies = %+v, want empty", deleted.CompatibilityPolicies)
	}
}
