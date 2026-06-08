package policyinspect

import (
	"errors"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestInspectModelReturnsResolvedPolicyWithoutSecrets(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Providers[0].Name = "DeepSeek"
	cfg.Providers[0].APIKey = "sk-secret"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].UpstreamModel = "deepseek/deepseek-v4-pro"
	cfg.Models[0].Reasoning.Mode = "auto"

	got, err := InspectModel(cfg, "deepseek-v4-pro")
	if err != nil {
		t.Fatalf("InspectModel() error = %v", err)
	}
	if got.SchemaVersion != 1 || got.ModelID != "deepseek-v4-pro" {
		t.Fatalf("inspection = %+v", got)
	}
	if got.ProviderID != "deepseek" || got.ProviderType != "openai_compatible" {
		t.Fatalf("provider fields = %+v", got)
	}
	if got.Protocol != "openai_compatible" {
		t.Fatalf("protocol = %q, want openai_compatible", got.Protocol)
	}
	for _, want := range []string{"deepseek-v4-openai-compatible", "reasoning-replay-provider-families", "reasoning-replay-model-families"} {
		if !hasInspectionPolicy(got.MatchedPolicies, want) {
			t.Fatalf("matched policies = %+v, missing %s", got.MatchedPolicies, want)
		}
	}
	if !got.ResolvedReasoning.Enabled || got.ResolvedReasoning.Effort != "max" {
		t.Fatalf("resolved reasoning = %+v, want enabled max", got.ResolvedReasoning)
	}
	if !got.ResolvedReasoning.Replay || !got.ResolvedReasoning.OmitToolChoice {
		t.Fatalf("resolved reasoning = %+v, want replay and omit_tool_choice", got.ResolvedReasoning)
	}
	if strings.Contains(got.String(), "sk-secret") {
		t.Fatalf("inspection leaked provider secret: %s", got.String())
	}
}

func hasInspectionPolicy(matches []config.CompatibilityPolicyMatch, id string) bool {
	for _, match := range matches {
		if match.ID == id {
			return true
		}
	}
	return false
}

func TestInspectModelMissingModel(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	_, err := InspectModel(cfg, "missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestInspectModelMissingProvider(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Models[0].ProviderID = "missing-provider"
	_, err := InspectModel(cfg, cfg.Models[0].ID)
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("error = %v, want ErrProviderNotFound", err)
	}
}

func TestInspectModelIncludesUserOverrideMetadata(t *testing.T) {
	falseValue := false
	cfg := config.MinimalValidConfig("local-key")
	cfg.CompatibilityPolicies = []config.CompatibilityPolicyConfig{{
		ID: "model-openrouter-sonnet-compat",
		Match: config.CompatibilityMatchConfig{
			ProviderIDs:    []string{cfg.Models[0].ProviderID},
			UpstreamModels: []string{cfg.Models[0].UpstreamModel},
		},
		Reasoning: config.CompatibilityReasoningConfig{
			Replay: &falseValue,
		},
	}}
	got, err := InspectModel(cfg, cfg.Models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UserOverride.Exists {
		t.Fatalf("user override = %+v, want exists", got.UserOverride)
	}
	if got.UserOverride.Replay == nil || *got.UserOverride.Replay {
		t.Fatalf("user override replay = %v, want false", got.UserOverride.Replay)
	}
}
