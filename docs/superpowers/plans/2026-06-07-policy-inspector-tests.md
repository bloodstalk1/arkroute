# Policy Inspector + Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only policy inspector that shows which compatibility policies affect a model, exposes the final request-independent reasoning behavior, and covers the flow with backend and frontend verification.

**Architecture:** Extract OpenAI-compatible reasoning resolution into a shared Go package so the runtime adapter and inspector use the same logic. Add config-level compatibility inspection with policy match sources, expose it through a setup-token protected panel endpoint, mount that endpoint through the Claude gateway, and render it in the existing Routes panel without exposing provider API keys.

**Tech Stack:** Go `net/http`, Arkroute YAML config, existing `internal/provider` protocol resolver, React single-page panel in `web-ui`, existing `npm run build:frontend` asset pipeline.

---

## File Structure

- Create: `internal/reasoning/resolve.go`
  - Shared request-independent and request-aware reasoning resolver used by OpenAI adapter and policy inspector.
- Create: `internal/reasoning/resolve_test.go`
  - Unit coverage for auto/custom/adaptive/passthrough behavior and Claude effort following.
- Modify: `internal/adapter/openai/reasoning_policy.go`
  - Replace duplicated resolver implementation with a thin wrapper around `internal/reasoning`.
- Create: `internal/config/compatibility_inspect.go`
  - Compatibility policy matching with source tracking, resolved model output, and explanation strings.
- Modify: `internal/config/compatibility.go`
  - Reuse the source-tracking implementation for runtime policy application.
- Test: `internal/config/config_test.go`
  - Add precedence and explanation tests for user policy, builtin policy, and model override.
- Create: `internal/policyinspect/inspect.go`
  - Service-level inspector that finds model/provider, resolves protocol, combines compatibility policy output, and formats API response.
- Create: `internal/policyinspect/inspect_test.go`
  - Unit coverage for deterministic output, missing references, and API key redaction.
- Create: `internal/panel/policy_inspect.go`
  - HTTP handler for `GET /internal/policy/inspect?model_id=<model_id>`.
- Modify: `internal/panel/server.go`
  - Register the setup-token protected inspector route.
- Test: `internal/panel/server_test.go`
  - Endpoint auth, success payload, missing model, and secret redaction tests.
- Modify: `internal/client/claude/server.go`
  - Mount `/internal/policy/inspect` through the gateway panel handler.
- Test: `internal/client/claude/server_test.go`
  - Verify setup session token can access the mounted endpoint through the gateway.
- Modify: `web-ui/src/App.jsx`
  - Add selected model state, inspector fetch, model/route target selection, and read-only policy panel.
- Modify: `web-ui/src/index.css`
  - Add compact inspector styles consistent with existing operator cards and route lists.

---

### Task 1: Shared Reasoning Resolver

**Files:**
- Create: `internal/reasoning/resolve.go`
- Create: `internal/reasoning/resolve_test.go`
- Modify: `internal/adapter/openai/reasoning_policy.go`
- Test: `internal/adapter/openai/openai_test.go`

- [ ] **Step 1: Write failing resolver tests**

Create `internal/reasoning/resolve_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/reasoning
```

Expected: `FAIL` because package `internal/reasoning` has no implementation.

- [ ] **Step 3: Add shared resolver implementation**

Create `internal/reasoning/resolve.go`:

```go
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
```

- [ ] **Step 4: Wire OpenAI adapter to the shared resolver**

Replace the contents of `internal/adapter/openai/reasoning_policy.go` with:

```go
package openai

import (
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/reasoning"
)

type reasoningBehavior = reasoning.Behavior

func resolveReasoning(provider config.ProviderConfig, model config.ModelConfig, req protocol.Request) reasoningBehavior {
	return reasoning.Resolve(provider, model, req)
}
```

- [ ] **Step 5: Verify shared resolver and OpenAI adapter**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/reasoning ./internal/adapter/openai
```

Expected: `ok` for both packages.

- [ ] **Step 6: Commit Task 1**

Run:

```bash
git status --short
git add internal/reasoning/resolve.go internal/reasoning/resolve_test.go internal/adapter/openai/reasoning_policy.go
git commit -m "refactor: share reasoning resolution"
```

Expected: commit succeeds. If unrelated dirty files are present, leave them unstaged.

---

### Task 2: Config Compatibility Policy Inspector

**Files:**
- Create: `internal/config/compatibility_inspect.go`
- Modify: `internal/config/compatibility.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing precedence test**

Append to `internal/config/config_test.go`:

```go
func TestInspectCompatibilityPoliciesExplainsPrecedence(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].UpstreamModel = "provider/deepseek-v4-pro"
	cfg.Models[0].Reasoning.Replay = configBoolPtr(false)
	cfg.CompatibilityPolicies = []CompatibilityPolicyConfig{{
		ID: "user-deepseek-v4",
		Match: CompatibilityMatchConfig{
			UpstreamModelPatterns: []string{"*deepseek*v4*"},
		},
		Reasoning: CompatibilityReasoningConfig{
			OmitToolChoice: configBoolPtr(false),
		},
	}}

	got := InspectCompatibilityPolicies(cfg.Providers[0], cfg.Models[0], cfg.CompatibilityPolicies)
	for _, want := range []CompatibilityPolicyMatch{
		{ID: "user-deepseek-v4", Source: "user"},
		{ID: "deepseek-v4-openai-compatible", Source: "builtin"},
		{ID: "reasoning-replay-provider-families", Source: "builtin"},
		{ID: "reasoning-replay-model-families", Source: "builtin"},
	} {
		if !hasCompatibilityPolicyMatch(got.MatchedPolicies, want.ID, want.Source) {
			t.Fatalf("matched policies = %+v, missing %+v", got.MatchedPolicies, want)
		}
	}
	if got.Model.Reasoning.Replay == nil || *got.Model.Reasoning.Replay {
		t.Fatalf("replay = %v, want model override false", got.Model.Reasoning.Replay)
	}
	if got.Model.Reasoning.OmitToolChoice == nil || *got.Model.Reasoning.OmitToolChoice {
		t.Fatalf("omit_tool_choice = %v, want user override false", got.Model.Reasoning.OmitToolChoice)
	}
	if got.Model.Reasoning.AutoEnable == nil || !*got.Model.Reasoning.AutoEnable {
		t.Fatalf("auto_enable = %v, want builtin true", got.Model.Reasoning.AutoEnable)
	}
	if got.Model.Reasoning.AutoEffort != "max" {
		t.Fatalf("auto_effort = %q, want max", got.Model.Reasoning.AutoEffort)
	}
	if got.ReasoningSources["replay"].Source != "model" {
		t.Fatalf("replay source = %+v, want model source", got.ReasoningSources["replay"])
	}
	if got.ReasoningSources["omit_tool_choice"].PolicyID != "user-deepseek-v4" {
		t.Fatalf("omit_tool_choice source = %+v, want user policy source", got.ReasoningSources["omit_tool_choice"])
	}
	if got.ReasoningSources["auto_enable"].PolicyID != "deepseek-v4-openai-compatible" {
		t.Fatalf("auto_enable source = %+v, want builtin policy source", got.ReasoningSources["auto_enable"])
	}
	explain := strings.Join(got.Explain, "\n")
	for _, want := range []string{
		"models[].reasoning.replay overrides policy deepseek-v4-openai-compatible replay",
		"user policy user-deepseek-v4 sets omit_tool_choice",
		"builtin policy deepseek-v4-openai-compatible sets auto_enable",
	} {
		if !strings.Contains(explain, want) {
			t.Fatalf("explain missing %q: %s", want, explain)
		}
	}
}

func hasCompatibilityPolicyMatch(matches []CompatibilityPolicyMatch, id, source string) bool {
	for _, match := range matches {
		if match.ID == id && match.Source == source {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/config -run TestInspectCompatibilityPoliciesExplainsPrecedence
```

Expected: `FAIL` with `undefined: InspectCompatibilityPolicies`.

- [ ] **Step 3: Add source-tracking inspector**

Create `internal/config/compatibility_inspect.go`:

```go
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
```

- [ ] **Step 4: Reuse inspector in runtime policy application**

Modify the top of `internal/config/compatibility.go` so the two exported application functions read:

```go
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
```

Keep `builtinCompatibilityPolicies`, `compatibilityPolicyMatches`, `containsAnyLower`, `matchesAnyLowerPattern`, and `wildcardPatternRegexp` in the same file. This preserves runtime order: snapshot applies user policies, the OpenAI adapter applies builtin policies, and the inspector applies both only for explanation output.

- [ ] **Step 5: Verify config tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/config
```

Expected: `ok`.

- [ ] **Step 6: Commit Task 2**

Run:

```bash
git status --short
git add internal/config/compatibility.go internal/config/compatibility_inspect.go internal/config/config_test.go
git commit -m "feat: inspect compatibility policy precedence"
```

Expected: commit succeeds with only Task 2 files staged.

---

### Task 3: Policy Inspector Service

**Files:**
- Create: `internal/policyinspect/inspect.go`
- Create: `internal/policyinspect/inspect_test.go`

- [ ] **Step 1: Write failing service tests**

Create `internal/policyinspect/inspect_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/policyinspect
```

Expected: `FAIL` because `internal/policyinspect` has no implementation.

- [ ] **Step 3: Add inspector service**

Create `internal/policyinspect/inspect.go`:

```go
package policyinspect

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bloodstalk1/arkroute/internal/config"
	providercatalog "github.com/bloodstalk1/arkroute/internal/provider"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/reasoning"
)

var (
	ErrModelNotFound    = errors.New("model not found")
	ErrProviderNotFound = errors.New("provider not found")
)

type Inspection struct {
	SchemaVersion    int                                      `json:"schema_version"`
	ModelID          string                                   `json:"model_id"`
	ProviderID       string                                   `json:"provider_id"`
	ProviderType     string                                   `json:"provider_type"`
	UpstreamModel    string                                   `json:"upstream_model"`
	Protocol         string                                   `json:"protocol"`
	MatchedPolicies  []config.CompatibilityPolicyMatch        `json:"matched_policies"`
	ResolvedReasoning ResolvedReasoning                       `json:"resolved_reasoning"`
	ReasoningSources map[string]config.CompatibilityFieldSource `json:"reasoning_sources"`
	Explain          []string                                 `json:"explain"`
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
	}, nil
}

func (i Inspection) String() string {
	data, err := json.Marshal(i)
	if err != nil {
		return fmt.Sprintf("%+v", i)
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
```

After adding the file, run `gofmt` on it:

```bash
gofmt -w internal/policyinspect/inspect.go internal/policyinspect/inspect_test.go
```

- [ ] **Step 4: Verify service tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/policyinspect
```

Expected: `ok`.

- [ ] **Step 5: Commit Task 3**

Run:

```bash
git status --short
git add internal/policyinspect/inspect.go internal/policyinspect/inspect_test.go
git commit -m "feat: add policy inspection service"
```

Expected: commit succeeds.

---

### Task 4: Panel HTTP Endpoint

**Files:**
- Create: `internal/panel/policy_inspect.go`
- Modify: `internal/panel/server.go`
- Test: `internal/panel/server_test.go`

- [ ] **Step 1: Write failing panel endpoint tests**

Append to `internal/panel/server_test.go`:

```go
func TestPolicyInspectRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=openrouter-sonnet", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPolicyInspectReturnsModelPolicyWithValidToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-secret"
	cfg.Models[0].UpstreamModel = "deepseek/deepseek-v4-pro"
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=openrouter-sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"model_id":"openrouter-sonnet"`, `"matched_policies"`, `"resolved_reasoning"`, `"deepseek-v4-openai-compatible"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("response leaked provider key: %s", rec.Body.String())
	}
}

func TestPolicyInspectMissingModelReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := savePanelConfig(path, config.MinimalValidConfig("local-key")); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=missing", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run PolicyInspect
```

Expected: `FAIL` with 404 or missing route.

- [ ] **Step 3: Add panel handler**

Create `internal/panel/policy_inspect.go`:

```go
package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/policyinspect"
)

func handlePolicyInspect(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		modelID := strings.TrimSpace(r.URL.Query().Get("model_id"))
		if modelID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "model_id is required"})
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		inspection, err := policyinspect.InspectModel(cfg, modelID)
		if errors.Is(err, policyinspect.ErrModelNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if errors.Is(err, policyinspect.ErrProviderNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, inspection)
	}
}
```

- [ ] **Step 4: Register endpoint in panel routes**

In `internal/panel/server.go`, add this route next to the other `/internal/...` panel routes:

```go
mux.HandleFunc("/internal/policy/inspect", withSetupToken(deps.Sessions, handlePolicyInspect(deps.ConfigPath)))
```

The route block should include:

```go
mux.HandleFunc("/internal/setup/logs", withSetupToken(deps.Sessions, handleGetLogs()))
mux.HandleFunc("/internal/cli-tools", withSetupToken(deps.Sessions, handleCLIToolsStatus(deps.CLITools)))
mux.HandleFunc("/internal/cli-tools/claude/launch", withSetupToken(deps.Sessions, handleClaudeLaunch(deps.CLITools)))
mux.HandleFunc("/internal/policy/inspect", withSetupToken(deps.Sessions, handlePolicyInspect(deps.ConfigPath)))
```

- [ ] **Step 5: Verify panel endpoint tests**

Run:

```bash
gofmt -w internal/panel/policy_inspect.go internal/panel/server.go internal/panel/server_test.go
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run 'PolicyInspect|CLITools|SetupOptions'
```

Expected: `ok`.

- [ ] **Step 6: Commit Task 4**

Run:

```bash
git status --short
git add internal/panel/policy_inspect.go internal/panel/server.go internal/panel/server_test.go
git commit -m "feat: expose policy inspector endpoint"
```

Expected: commit succeeds.

---

### Task 5: Gateway Mount

**Files:**
- Modify: `internal/client/claude/server.go`
- Test: `internal/client/claude/server_test.go`

- [ ] **Step 1: Write failing gateway mount test**

Append to `internal/client/claude/server_test.go`:

```go
func TestPolicyInspectMountedOnGatewayWithSetupSession(t *testing.T) {
	srv := testServer(t)
	sessionReq := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	sessionReq.Header.Set("Authorization", "Bearer local-key")
	sessionRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionRec.Code, sessionRec.Body.String())
	}
	var sessionPayload struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.Unmarshal(sessionRec.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=openrouter-sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", sessionPayload.SetupToken)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"model_id":"openrouter-sonnet"`, `"resolved_reasoning"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/client/claude -run TestPolicyInspectMountedOnGatewayWithSetupSession
```

Expected: `FAIL` because the gateway does not mount `/internal/policy/inspect`.

- [ ] **Step 3: Mount policy inspector route through the gateway**

In `internal/client/claude/server.go`, add this line near the other panel handler mounts:

```go
mux.Handle("/internal/policy/inspect", panelHandler)
```

The final panel mount block should include:

```go
mux.Handle("/internal/setup/logs", panelHandler)
mux.Handle("/internal/cli-tools", panelHandler)
mux.Handle("/internal/cli-tools/claude/launch", panelHandler)
mux.Handle("/internal/policy/inspect", panelHandler)
```

- [ ] **Step 4: Verify gateway mount test**

Run:

```bash
gofmt -w internal/client/claude/server.go internal/client/claude/server_test.go
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/client/claude -run 'PolicyInspectMounted|SetupSession|CLIToolsMounted'
```

Expected: `ok`.

- [ ] **Step 5: Commit Task 5**

Run:

```bash
git status --short
git add internal/client/claude/server.go internal/client/claude/server_test.go
git commit -m "feat: mount policy inspector in gateway"
```

Expected: commit succeeds.

---

### Task 6: Routes Panel Policy Inspector UI

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`

- [ ] **Step 1: Add model selection and inspector fetch state**

In `web-ui/src/App.jsx`, inside the main `App` component near existing state declarations, add:

```jsx
  const [selectedModelId, setSelectedModelId] = useState("");
  const [policyInspect, setPolicyInspect] = useState(null);
  const [policyInspectLoading, setPolicyInspectLoading] = useState(false);
  const [policyInspectStatus, setPolicyInspectStatus] = useState({ text: "", type: "" });
```

Add this effect after the status/config loading effects:

```jsx
  useEffect(() => {
    const models = config?.models || [];
    if (models.length === 0) {
      setSelectedModelId("");
      setPolicyInspect(null);
      return;
    }
    if (!selectedModelId || !models.some((model) => model.id === selectedModelId)) {
      setSelectedModelId(models[0].id);
    }
  }, [config, selectedModelId]);
```

Add this fetch effect after the selection effect:

```jsx
  useEffect(() => {
    if (activeTab !== "models" || !selectedModelId) {
      return;
    }
    let cancelled = false;
    setPolicyInspectLoading(true);
    setPolicyInspectStatus({ text: "", type: "" });
    fetch(`/internal/policy/inspect?model_id=${encodeURIComponent(selectedModelId)}`, { headers: apiHeaders })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((payload) => {
        if (!cancelled) {
          setPolicyInspect(payload);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setPolicyInspect(null);
          setPolicyInspectStatus({ text: err.message, type: "error" });
        }
      })
      .finally(() => {
        if (!cancelled) {
          setPolicyInspectLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeTab, selectedModelId, apiHeaders]);
```

- [ ] **Step 2: Make model and route target items selectable**

Replace `ModelItem` in `web-ui/src/App.jsx` with:

```jsx
function ModelItem({ model, active, onSelect }) {
  return (
    <button className={`list-item selectable-list-item ${active ? "active" : ""}`} type="button" onClick={() => onSelect(model.id)}>
      <div>
        <strong>{model.display_name || model.id}</strong>
        <span>{model.provider_id}</span>
      </div>
      <code>{model.upstream_model}</code>
    </button>
  );
}
```

Replace `RouteItem` with:

```jsx
function RouteItem({ route, selectedModelId, onSelectModel }) {
  return (
    <div className="list-item route-item">
      <div>
        <strong>{route.alias}</strong>
        <span>{route.strategy}</span>
      </div>
      <div className="target-list">
        {(route.targets || []).map((target, index) => (
          <button
            className={`${target.enabled ? "target enabled" : "target"} ${selectedModelId === target.model_id ? "selected" : ""}`}
            key={`${target.model_id}-${index}`}
            type="button"
            onClick={() => onSelectModel(target.model_id)}
          >
            {index + 1}. {target.model_id}
          </button>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Add read-only inspector components**

Add these helpers below `RouteItem`:

```jsx
function PolicyValue({ label, value, source }) {
  const renderedValue = typeof value === "boolean" ? (value ? "true" : "false") : (value || "unset");
  return (
    <div className="policy-value">
      <span>{label}</span>
      <strong>{renderedValue}</strong>
      {source && <small>{source.policy_id || source.source}</small>}
    </div>
  );
}

function PolicyInspector({ inspection, loading, status }) {
  if (loading) {
    return <EmptyState icon="ph-shield-checkered" title="Inspecting policy">Reading local config and policy matches.</EmptyState>;
  }
  if (status.text) {
    return <div className={`status-box ${status.type}`}>{status.text}</div>;
  }
  if (!inspection) {
    return <EmptyState icon="ph-shield-checkered" title="No model selected">Select a registered model or route target.</EmptyState>;
  }
  const reasoning = inspection.resolved_reasoning || {};
  const sources = inspection.reasoning_sources || {};
  return (
    <section className="operator-card policy-inspector-card">
      <div className="card-heading">
        <div>
          <StatusBadge tone={inspection.matched_policies?.length > 0 ? "ok" : "pending"}>
            {inspection.matched_policies?.length || 0} policies
          </StatusBadge>
          <h3><i className="ph-light ph-shield-checkered"></i>Policy Inspector</h3>
        </div>
      </div>

      <div className="policy-summary-grid">
        <DataRow label="Model">{inspection.model_id}</DataRow>
        <DataRow label="Provider">{inspection.provider_id}</DataRow>
        <DataRow label="Provider type">{inspection.provider_type || "auto"}</DataRow>
        <DataRow label="Protocol">{inspection.protocol}</DataRow>
        <DataRow label="Upstream">{inspection.upstream_model}</DataRow>
      </div>

      <div className="policy-chip-row">
        {(inspection.matched_policies || []).length > 0 ? (
          inspection.matched_policies.map((policy) => (
            <span className={`policy-chip ${policy.source}`} key={`${policy.source}-${policy.id}`}>{policy.source}: {policy.id}</span>
          ))
        ) : (
          <span className="policy-chip muted">no compatibility policy matched</span>
        )}
      </div>

      <div className="policy-value-grid">
        <PolicyValue label="enabled" value={reasoning.enabled} source={sources.enabled} />
        <PolicyValue label="effort" value={reasoning.effort} source={sources.effort} />
        <PolicyValue label="auto_enable" value={reasoning.auto_enable} source={sources.auto_enable} />
        <PolicyValue label="auto_effort" value={reasoning.auto_effort} source={sources.auto_effort} />
        <PolicyValue label="replay" value={reasoning.replay} source={sources.replay} />
        <PolicyValue label="omit_tool_choice" value={reasoning.omit_tool_choice} source={sources.omit_tool_choice} />
        <PolicyValue label="follow_claude_effort" value={reasoning.follow_claude_effort} source={sources.follow_claude_effort} />
      </div>

      {(inspection.explain || []).length > 0 && (
        <div className="policy-explain-list">
          {inspection.explain.map((line, index) => <span key={`${line}-${index}`}>{line}</span>)}
        </div>
      )}
    </section>
  );
}
```

- [ ] **Step 4: Render inspector in the Routes tab**

In the Routes tab, replace model and route item rendering with:

```jsx
{modelCount > 0 ? (
  config.models.map((model) => (
    <ModelItem key={model.id} model={model} active={selectedModelId === model.id} onSelect={setSelectedModelId} />
  ))
) : (
  <EmptyState icon="ph-cube" title="No models">Provider setup creates the first exposed model.</EmptyState>
)}
```

Replace route rendering with:

```jsx
{routeCount > 0 ? (
  config.routes.map((route) => (
    <RouteItem key={route.alias} route={route} selectedModelId={selectedModelId} onSelectModel={setSelectedModelId} />
  ))
) : (
  <EmptyState icon="ph-git-branch" title="No routes">Create a route alias during provider setup.</EmptyState>
)}
```

After the two existing operator cards in the Routes tab grid, add:

```jsx
<PolicyInspector inspection={policyInspect} loading={policyInspectLoading} status={policyInspectStatus} />
```

The Routes tab grid should have three cards: registered models, router definitions, and policy inspector.

- [ ] **Step 5: Add inspector CSS**

Append to `web-ui/src/index.css`:

```css
.selectable-list-item {
  width: 100%;
  text-align: left;
  border: 1px solid rgba(148, 163, 184, 0.18);
  cursor: pointer;
}

.selectable-list-item.active {
  border-color: rgba(34, 197, 94, 0.52);
  background: rgba(34, 197, 94, 0.08);
}

.target {
  border: 1px solid rgba(148, 163, 184, 0.18);
  cursor: pointer;
}

.target.selected {
  border-color: rgba(34, 197, 94, 0.56);
  background: rgba(34, 197, 94, 0.12);
  color: #d9f99d;
}

.policy-inspector-card {
  grid-column: 1 / -1;
}

.policy-summary-grid,
.policy-value-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
}

.policy-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 16px 0;
}

.policy-chip {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 6px;
  padding: 0 10px;
  background: rgba(15, 23, 42, 0.48);
  color: #dbeafe;
  font-size: 12px;
  letter-spacing: 0;
}

.policy-chip.user {
  color: #bbf7d0;
}

.policy-chip.builtin {
  color: #fde68a;
}

.policy-chip.muted {
  color: #94a3b8;
}

.policy-value {
  min-height: 92px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 8px;
  padding: 12px;
  background: rgba(15, 23, 42, 0.42);
}

.policy-value span,
.policy-value small {
  display: block;
  color: #94a3b8;
  font-size: 12px;
  letter-spacing: 0;
}

.policy-value strong {
  display: block;
  margin-top: 8px;
  color: #f8fafc;
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 15px;
  letter-spacing: 0;
  overflow-wrap: anywhere;
}

.policy-value small {
  margin-top: 6px;
}

.policy-explain-list {
  display: grid;
  gap: 8px;
  margin-top: 16px;
}

.policy-explain-list span {
  border-left: 2px solid rgba(34, 197, 94, 0.45);
  padding-left: 10px;
  color: #cbd5e1;
  font-size: 13px;
  line-height: 1.45;
}
```

- [ ] **Step 6: Build frontend**

Run:

```bash
npm run build:frontend
```

Expected: build completes and updates embedded panel assets under `internal/panel/assets/`.

- [ ] **Step 7: Commit Task 6**

Run:

```bash
git status --short
git add web-ui/src/App.jsx web-ui/src/index.css internal/panel/assets
git commit -m "feat: show policy inspector in panel"
```

Expected: commit succeeds with source and generated frontend assets staged.

---

### Task 7: Full Verification

**Files:**
- No new files.

- [ ] **Step 1: Run focused backend verification**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/reasoning ./internal/config ./internal/policyinspect ./internal/panel ./internal/client/claude ./internal/adapter/openai
```

Expected: all listed packages return `ok`.

- [ ] **Step 2: Run full backend suite**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./...
```

Expected: all packages return `ok`.

If this fails inside the sandbox with `operation not permitted` from `httptest` listener binding, rerun the same command outside the sandbox through the normal escalation flow. The expected outside-sandbox result is still `ok`.

- [ ] **Step 3: Run frontend build again**

Run:

```bash
npm run build:frontend
```

Expected: build completes.

- [ ] **Step 4: Check for accidental secret exposure**

Run:

```bash
rg -n "sk-secret|OPENROUTER_API_KEY.*sk-|api_key.*sk-" internal/policyinspect internal/panel web-ui/src
```

Expected: no matches except test literals that intentionally assert secrets are not leaked.

- [ ] **Step 5: Check final git scope**

Run:

```bash
git status --short
git diff --stat HEAD
```

Expected: only files from this plan are changed after the last commit. If there are pre-existing unrelated dirty files, leave them untouched and report them separately.

- [ ] **Step 6: Final commit for verification adjustments**

If verification required small fixes, commit those explicit files:

```bash
git add internal/reasoning internal/config internal/policyinspect internal/panel internal/client/claude web-ui/src internal/panel/assets
git commit -m "test: verify policy inspector flow"
```

Expected: commit succeeds only when there are verification fixes. If there are no changes, skip this commit and record that verification passed with the Task 1-6 commits.

---

## Self-Review

- Spec coverage: Plan covers Policy Inspector purpose, backend endpoint, deterministic no-network inspection, API-key redaction, model/user/builtin/capability source separation, UI display, and regression tests.
- Scope control: This plan does not implement provider-first CLI setup, editable compatibility policy UI, route presets, config export/import, or docs because those belong to Plans B and C.
- Runtime parity: Reasoning behavior is shared through `internal/reasoning`, so the OpenAI adapter and inspector use the same resolution logic.
- Security: Endpoint is setup-token protected and the response type does not include `ProviderConfig.APIKey`.
- Verification: Focused package tests, full `go test ./...`, frontend build, and secret scan are included.
