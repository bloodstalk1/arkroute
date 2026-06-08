# Multi-CLI Routing With Arkroute

Arkroute keeps four layers separate:

```text
CLI profile -> route/model alias -> provider/protocol resolver -> compatibility policy
```

CLI profiles only point clients at Arkroute and choose a route or exposed model alias. Provider-specific quirks belong in compatibility policies, not in CLI snippets.

## Activation Commands

```sh
eval "$(arkroute activate claude)"
eval "$(arkroute activate opencode)"
eval "$(arkroute activate codex)"
eval "$(arkroute activate droid)"
```

Claude Code uses Arkroute's Anthropic-compatible endpoint and gateway model discovery. OpenCode, Codex, Droid, and OpenAI-like clients use Arkroute's `/v1` endpoint with `server.client_key`.

## Choosing Aliases

Use route aliases for shared workflows and exposed model aliases for direct targeting.

```yaml
routes:
  - alias: sonnet
    strategy: fallback
    targets:
      - model_id: deepseek-deepseek-v4-pro
        enabled: true
      - model_id: qwen-qwen-coder
        enabled: true
      - model_id: openrouter-sonnet-or
        enabled: true
```

The same alias can be used by Claude Code, OpenCode, Codex, Droid, and SDK clients.

## Compatibility Policy Precedence

Reasoning compatibility is resolved in this order:

1. Model-level `models[].reasoning`
2. User `compatibility_policies`
3. Builtin compatibility policies
4. Capability defaults

Example override:

```yaml
compatibility_policies:
  - id: model-deepseek-v4-pro-compat
    match:
      provider_ids: [deepseek]
      upstream_models: [deepseek-v4-pro]
    reasoning:
      auto_enable: false
      replay: false
      omit_tool_choice: false
```

## DeepSeek V4 Pro Troubleshooting

If Claude Code fails on a DeepSeek V4 Pro route, inspect the model in the Routes panel. Check `auto_enable`, `auto_effort`, `replay`, and `omit_tool_choice`. Disable the generated user override only when the upstream provider behaves correctly with Claude-style reasoning controls.
