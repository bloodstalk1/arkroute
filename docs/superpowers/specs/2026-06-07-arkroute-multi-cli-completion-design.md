# Arkroute Multi-CLI Completion Design

Date: 2026-06-07
Status: Draft for review

## Goal

Finish Arkroute as a practical local routing control plane for using many models across many coding CLIs. The system should let a user configure providers, routes, compatibility behavior, and CLI activation from one local panel while keeping runtime behavior config-driven and debuggable.

This spec covers the seven follow-up workstreams after the initial backend compatibility policy work:

1. Policy Inspector
2. Provider + Model + CLI panel flow
3. Compatibility Policy UI
4. Route Presets
5. Config Import/Export + Backup
6. Multi-CLI Docs
7. Regression Tests

## Current State

Arkroute already has:

- Route/model/provider config in `config.yaml`.
- Claude-compatible ingress for Claude Code.
- OpenAI-compatible ingress for OpenCode, Codex-style clients, Droid, and SDK users.
- CLI activation commands for Claude, OpenCode, Codex, and Droid-style OpenAI clients.
- A panel with provider/route/system surfaces.
- Compatibility policy schema and builtin reasoning quirks for DeepSeek/reasoning replay.

Remaining gaps:

- Users cannot easily see which compatibility policy affected a model.
- CLI setup is still separated from provider/model selection.
- Compatibility policy edits require manual YAML.
- Common route/model presets are not first-class enough for multi-provider fallback.
- Panel config writes need backup/export safety.
- Docs explain individual client setup, but not the complete multi-CLI mental model.
- Regression coverage is spread across backend tests and needs explicit multi-CLI/policy test cases.

## Non-Goals

This work does not include:

- Cloud sync or remote control.
- OAuth import for every provider.
- Provider quota/billing analytics beyond what is already present.
- Full OmniRoute-style payload rule engine.
- MITM/browser automation behavior.
- Replacing `config.yaml` as the source of truth.
- Making Arkroute a hosted SaaS router.

## Architecture Principles

Arkroute should preserve four separate layers:

```text
CLI profile -> route/model alias -> provider/protocol resolver -> compatibility policy
```

CLI profiles should never contain provider/model quirks. A CLI profile only tells a client how to reach Arkroute and which model alias to request.

Routes and models decide where traffic goes. Provider/protocol resolution decides which adapter is used. Compatibility policies decide which quirks are applied for a provider/model pair.

The panel should make those layers visible without forcing the user to learn the internal code structure.

## 1. Policy Inspector

### Purpose

Show why a route/model behaves the way it does. This is critical now that quirks live in policies instead of inline adapter code.

### User-Facing Behavior

For a selected model or route target, the panel shows:

- Provider ID and provider type.
- Upstream model.
- Resolved protocol.
- Matched user policies.
- Matched builtin policies.
- Final resolved reasoning behavior:
  - `auto_enable`
  - `auto_effort`
  - `replay`
  - `omit_tool_choice`
  - `enabled`
  - `effort`
  - `follow_claude_effort`

The inspector should clearly distinguish:

- Explicit model-level override
- User compatibility policy
- Builtin compatibility policy
- Capability default

### Backend Shape

Add a read-only internal endpoint:

```text
GET /internal/policy/inspect?model_id=<model_id>
```

Response shape:

```json
{
  "schema_version": 1,
  "model_id": "deepseek-v4-pro",
  "provider_id": "deepseek",
  "upstream_model": "deepseek-v4-pro",
  "protocol": "openai_compatible",
  "matched_policies": [
    {"id": "deepseek-v4-custom", "source": "user"},
    {"id": "deepseek-v4-openai-compatible", "source": "builtin"}
  ],
  "resolved_reasoning": {
    "enabled": true,
    "effort": "max",
    "auto_enable": true,
    "auto_effort": "max",
    "replay": true,
    "omit_tool_choice": true,
    "follow_claude_effort": false
  },
  "explain": [
    "models[].reasoning.replay overrides builtin replay",
    "user policy deepseek-v4-custom sets omit_tool_choice"
  ]
}
```

### Acceptance Criteria

- Inspector works without upstream network calls.
- Inspector never exposes upstream provider API keys.
- Output is deterministic for a fixed config.
- Unit tests cover precedence explanation.

## 2. Provider + Model + CLI Panel Flow

### Purpose

Move the user workflow from "configure setup separately, then configure CLI separately" to "choose a provider/model, then activate the CLI from that context."

### User-Facing Behavior

The panel should have a provider-first flow:

```text
Providers -> Provider Detail -> Model/Route Detail -> CLI Setup
```

On a provider or model detail screen, the user can choose:

- Claude Code
- OpenCode
- Codex
- Droid/OpenAI-compatible

The selected CLI setup should show:

- Base URL
- Local Arkroute key
- Suggested route/model alias
- Copy activation command
- Launch status where launching is supported
- Model discovery status where relevant

The old standalone Setup tab should not be the main workflow. Setup can remain as a fallback/system action, but provider/model details should be the normal path.

### Acceptance Criteria

- Selecting a provider/model can produce correct CLI activation snippets.
- The same model alias can be used by multiple CLI profiles.
- CLI setup UI does not duplicate provider-specific quirk logic.
- No upstream provider key is shown in UI copy blocks.

## 3. Compatibility Policy UI

### Purpose

Allow users to override model/provider quirks without hand-editing YAML.

### Initial Scope

Only expose the current reasoning policy fields:

- `auto_enable`
- `auto_effort`
- `replay`
- `omit_tool_choice`

Do not add generic `request.filter/default/override` UI in this slice.

### User-Facing Behavior

On a model detail page:

- Show current resolved policy state.
- Show builtin policy matches.
- Allow creating a user override policy for that model/provider.
- Allow toggling the four supported reasoning fields.
- Allow deleting the user override.
- Provide "Reset to builtin" by removing the user override.

### Config Behavior

Panel writes should edit `compatibility_policies` in `config.yaml`.

Generated policy IDs should be stable and readable, for example:

```text
model-<model_id>-compat
```

If the model ID contains unsafe characters, sanitize it to lowercase alphanumeric plus `-`.

### Acceptance Criteria

- User can disable builtin DeepSeek V4 auto thinking from UI.
- User can disable/enable reasoning replay from UI.
- Saving policy reloads runtime config through existing config reload path.
- Invalid values are rejected before writing config.

## 4. Route Presets

### Purpose

Make common multi-model/multi-provider route setups fast and consistent.

### Preset Categories

Add presets for:

- DeepSeek V4 Pro
- Qwen coder/thinking
- GLM
- Kimi K2/K2.6
- MiniMax
- Claude via OpenRouter or Anthropic-compatible provider
- Generic OpenAI-compatible model

### Preset Output

A preset may create:

- Provider entry
- Model entry
- Route entry
- Profile alias
- Compatibility policy override if needed

Presets must not hide generated config. The user should be able to inspect and edit all generated entries.

### Default Route Pattern

Use fallback routes for multi-provider setups:

```yaml
routes:
  - alias: sonnet
    strategy: fallback
    targets:
      - model_id: deepseek-v4-pro
      - model_id: qwen-coder
      - model_id: openrouter-sonnet
```

### Acceptance Criteria

- Presets produce configs that pass validation.
- Presets do not overwrite existing IDs without confirmation.
- Presets can be applied incrementally.
- Preset tests protect provider base URL, provider type, model alias, route alias, and compatibility defaults.

## 5. Config Import/Export + Backup

### Purpose

Make panel-driven config edits safe. Users should be able to recover from bad edits and share config for debugging.

### Backup Behavior

Before any panel write to `config.yaml`, Arkroute should create a timestamped backup:

```text
~/.arkroute/backups/config-YYYYMMDD-HHMMSS.yaml
```

Keep a bounded number of backups, default 20.

### Export Behavior

Panel provides:

- Export full config
- Export redacted config
- Copy redacted config to clipboard

Redacted export must hide:

- `server.client_key`
- provider `api_key`
- provider headers

### Import Behavior

Import flow:

1. User selects/pastes YAML.
2. Arkroute parses and validates it.
3. Panel shows validation errors or summary of changes.
4. User confirms apply.
5. Existing config is backed up.
6. New config is written and reloaded.

### Acceptance Criteria

- Every panel write creates a backup.
- Invalid import cannot overwrite current config.
- Redacted export never leaks provider API keys.
- Backup pruning is deterministic and tested.

## 6. Multi-CLI Docs

### Purpose

Document the complete mental model for using many models across many CLIs.

### Required Docs

Add or update docs for:

- Claude Code through Anthropic-compatible Arkroute endpoint.
- OpenCode through OpenAI-compatible `/v1`.
- Codex through OpenAI-compatible `/v1`.
- Droid/OpenAI-like clients through `/v1`.
- Model discovery behavior.
- Route aliases vs exposed model aliases.
- Compatibility policies and override precedence.
- Common DeepSeek V4 Pro troubleshooting.

### Required Examples

Include examples for:

```sh
eval "$(arkroute activate claude)"
eval "$(arkroute activate opencode)"
eval "$(arkroute activate codex)"
eval "$(arkroute activate droid)"
```

Include a sample multi-provider fallback route and a sample compatibility override.

### Acceptance Criteria

- A new user can configure one provider and activate one CLI from docs.
- A power user can understand how to route multiple CLIs to the same alias.
- Docs state that provider quirks belong in compatibility policies, not CLI profiles.

## 7. Regression Tests

### Purpose

Protect the multi-CLI routing contract as Arkroute grows.

### Test Areas

Add focused tests for:

- Compatibility policy precedence:
  - model override
  - user policy
  - builtin policy
  - capability default
- Policy inspector response shape and explanation.
- CLI activation output for Claude, OpenCode, Codex, and Droid.
- Provider/model preset generation and validation.
- Config backup/export/import redaction.
- Model discovery includes route aliases and exposed model aliases.
- OpenAI-compatible request fixtures shaped like OpenCode/Codex/Droid.
- Claude-compatible request fixtures shaped like Claude Code.

### E2E Gate

Manual E2E outside sandbox should remain the release gate for CLI behavior:

- Start Arkroute.
- Activate Claude Code.
- Run a DeepSeek V4 Pro route.
- Run the same or equivalent route through OpenCode/Codex/OpenAI-compatible client.
- Confirm route/model policy is visible in the inspector.

### Acceptance Criteria

- Backend test suite passes with `go test ./...`.
- Frontend builds after panel changes.
- At least one manual E2E transcript is recorded in docs or release notes for major CLI-flow changes.

## Recommended Implementation Order

1. Policy Inspector backend and tests.
2. Policy Inspector panel display.
3. Config backup/export before any edit UI.
4. Compatibility Policy UI.
5. Route Presets.
6. Provider + Model + CLI panel flow.
7. Multi-CLI docs and broad regression pass.

This order makes the system observable before adding more write paths. The panel should first explain current behavior, then safely edit behavior, then streamline setup.

## Risks

- Policy behavior can become confusing if inspector output is weak.
- Presets can accidentally overwrite user config if ID handling is careless.
- CLI config snippets can drift as third-party CLI behavior changes.
- Too much policy UI can make the panel feel like a YAML editor.

Mitigation:

- Keep the first policy UI narrow.
- Require backup before writes.
- Prefer route/model aliases over CLI-specific special cases.
- Keep tests centered on behavior rather than exact UI copy.

