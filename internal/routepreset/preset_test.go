package routepreset

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/compatpolicy"
	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestPresetsCoverRequiredFamilies(t *testing.T) {
	want := map[string]bool{
		"deepseek-v4-pro": false,
		"qwen-coder": false,
		"glm": false,
		"kimi-k2": false,
		"minimax": false,
		"claude-openrouter": false,
		"generic-openai-compatible": false,
	}
	for _, preset := range Presets() {
		if _, ok := want[preset.ID]; ok {
			want[preset.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("preset %q missing from %+v", id, Presets())
		}
	}
}

func TestApplyPresetAddsProviderModelRouteAndProfile(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, summary, err := Apply(cfg, ApplyRequest{
		PresetID: "deepseek-v4-pro",
		ProviderID: "deepseek",
		APIKeyMode: "env",
		EnvName: "DEEPSEEK_API_KEY",
		RouteAlias: "sonnet",
		ProfileName: "deepseek",
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.ProviderID != "deepseek" || summary.ModelID == "" || summary.RouteAlias != "sonnet" {
		t.Fatalf("summary = %+v", summary)
	}
	if len(out.Providers) != 1 || out.Providers[0].APIKey != "env:DEEPSEEK_API_KEY" {
		t.Fatalf("providers = %+v", out.Providers)
	}
	if len(out.Models) != 1 || out.Models[0].ProviderID != "deepseek" {
		t.Fatalf("models = %+v", out.Models)
	}
	if len(out.Routes) != 1 || out.Routes[0].Targets[0].ModelID != out.Models[0].ID {
		t.Fatalf("routes = %+v", out.Routes)
	}
	if out.Profiles["deepseek"] != "sonnet" {
		t.Fatalf("profiles = %+v, want deepseek -> sonnet", out.Profiles)
	}
	policyID := compatpolicy.StableModelPolicyID(out.Models[0].ID)
	found := false
	for _, p := range out.CompatibilityPolicies {
		if p.ID == policyID {
			found = true
			if p.Reasoning.AutoEnable == nil || !*p.Reasoning.AutoEnable {
				t.Fatalf("CompatibilityPolicy.Reasoning.AutoEnable not set")
			}
			if p.Reasoning.Replay == nil || !*p.Reasoning.Replay {
				t.Fatalf("CompatibilityPolicy.Reasoning.Replay not set")
			}
		}
	}
	if !found {
		t.Fatalf("compatibility policy %q not found in %+v", policyID, out.CompatibilityPolicies)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyPresetDoesNotOverwriteWithoutConfirmation(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	_, _, err := Apply(cfg, ApplyRequest{
		PresetID: "claude-openrouter",
		ProviderID: cfg.Providers[0].ID,
		RouteAlias: "sonnet",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}
}

func TestApplyPresetCanAppendFallbackTarget(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	out, _, err := Apply(cfg, ApplyRequest{
		PresetID: "qwen-coder",
		ProviderID: "qwen",
		APIKeyMode: "env",
		EnvName: "QWEN_API_KEY",
		RouteAlias: "sonnet",
		AppendToRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Routes) != 1 || len(out.Routes[0].Targets) != 2 {
		t.Fatalf("routes = %+v, want appended fallback target", out.Routes)
	}
	if out.Routes[0].Strategy != "fallback" {
		t.Fatalf("strategy = %q, want fallback", out.Routes[0].Strategy)
	}
}

func TestNextDiscoveryAliasTakesCandidate(t *testing.T) {
	if got := nextDiscoveryAlias("claude-x", nil); got != "claude-x" {
		t.Errorf("nil existing: got %q, want claude-x", got)
	}
	if got := nextDiscoveryAlias("claude-x", []config.ModelConfig{{ClaudeDiscoveryAlias: "claude-y"}}); got != "claude-x" {
		t.Errorf("non-conflicting existing: got %q, want claude-x", got)
	}
	if got := nextDiscoveryAlias("claude-x", []config.ModelConfig{{ClaudeDiscoveryAlias: "claude-x"}}); got != "" {
		t.Errorf("conflicting existing: got %q, want empty", got)
	}
}

func TestAppendModelEntryUsesExplicitDiscovery(t *testing.T) {
	cfg := config.MinimalValidConfig("k")
	in := applyInputs{providerID: "p", providerName: "P", modelAlias: "m", modelID: "p-m"}
	out := appendModelEntry(cfg, Preset{UpstreamModel: "u", Capabilities: config.Capabilities{}}, in, "claude-explicit")
	if got := out.Models[len(out.Models)-1].ClaudeDiscoveryAlias; got != "claude-explicit" {
		t.Errorf("discovery = %q, want claude-explicit", got)
	}
}
