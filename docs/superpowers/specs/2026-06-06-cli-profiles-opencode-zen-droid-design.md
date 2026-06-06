# Arkroute CLI Profiles, OpenCode Zen, And Droid Compatibility Design

Date: 2026-06-06
Status: Ready for implementation-plan review

## Goal

Make Arkroute easier to use with the user's real daily toolchain:

- Warp Terminal as the shell surface.
- Claude CLI through Arkroute's Anthropic-compatible local gateway.
- OpenCode, Codex, and Droid CLI through Arkroute's OpenAI-compatible `/v1` gateway.
- Providers commonly used by the user: OpenCode Go, OpenCode Zen, Kiro, Claude, Codex, and OpenRouter.

The first implementation slice should improve setup, presets, docs, and compatibility tests. It should not add a large dashboard, SQLite persistence, OAuth import flows, or OmniRoute-style broad provider management.

## Current State

Arkroute already has:

- `arkroute activate claude` for Claude CLI environment/setup.
- OpenAI-compatible local ingress at `/v1/models`, `/v1/chat/completions`, and `/v1/responses`.
- Setup presets for OpenRouter, Anthropic, Gemini, OpenAI-compatible, OpenCode Go, and Custom.
- A provider protocol resolver that can auto-detect OpenCode Go quirks.

Gaps for this user's workflow:

- OpenCode Zen is not a first-class setup preset.
- OpenCode, Codex, and Droid do not have first-class activation/profile commands.
- README mentions generic OpenAI-compatible clients but does not provide concrete CLI setup snippets for this exact toolchain.
- Compatibility tests cover some OpenCode-style payloads, but the intended toolchain should be encoded as explicit fixtures and regression tests.

## Non-Goals

This slice does not include:

- OAuth import for Kiro, Claude Code OAuth, Codex OAuth, or any provider's local credential store.
- Browser automation, MITM, TLS fingerprinting, or stealth behavior.
- SQLite, quota tracking, budget management, usage dashboards, MCP, A2A, Electron, or cloud sync.
- Mutating Claude/Codex/OpenCode/Droid config files by default.
- New provider adapters beyond the existing `openai_compatible`, `anthropic`, and `gemini` adapters.
- Advanced routing such as weighted, cost-aware, quota-aware, latency-aware, or health-aware selection. Those remain separate future work.

## User-Facing Design

Add client profile commands that print shell-safe setup snippets. The canonical command surface for this slice is `activate <profile>` because Arkroute already has `arkroute activate claude`:

```sh
arkroute activate opencode
arkroute activate codex
arkroute activate droid
arkroute activate claude
```

Reserve the more explicit command form below for a later CLI cleanup, not this slice:

```sh
arkroute client env opencode
arkroute client env codex
arkroute client env droid
arkroute client env claude
```

`claude` keeps the existing Anthropic-compatible behavior.

`opencode` and `codex` print OpenAI-compatible environment exports:

```sh
export OPENAI_BASE_URL="http://127.0.0.1:2002/v1"
export OPENAI_API_KEY="<server.client_key>"
export OPENAI_MODEL="sonnet"
```

Codex custom-provider behavior is version-sensitive. The env output is still useful, but docs must state that some Codex versions require `~/.codex/config.toml` provider configuration or a top-level base URL setting. This slice does not write Codex config files.

`droid` prints exports plus a command-shaped comment because DroidRun's OpenAILike provider takes the base URL as `--api_base`:

```sh
export OPENAI_API_KEY="<server.client_key>"
export ARKROUTE_OPENAI_BASE_URL="http://127.0.0.1:2002/v1"
export ARKROUTE_OPENAI_MODEL="sonnet"
# droidrun run --provider OpenAILike --model "$ARKROUTE_OPENAI_MODEL" --api_base "$ARKROUTE_OPENAI_BASE_URL" "<task>"
```

For Warp Terminal, the command output should be copy/paste friendly, with no interactive prompts required. Users can run:

```sh
eval "$(arkroute activate opencode)"
```

## Provider Presets

Add an OpenCode Zen preset distinct from OpenCode Go:

- `id`: `opencode-zen`
- `name`: `OpenCode Zen`
- `type`: `openai_compatible`
- `base_url`: `https://opencode.ai/zen/v1`
- `default_model`: `kimi-k2.6`
- `default_alias`: `opencode-zen-kimi`
- `default_route`: `sonnet`
- `claude_discovery_alias`: `claude-sonnet-4-20250514`

Keep OpenCode Go as its own preset. Do not merge Go and Zen into one generic OpenCode preset because they differ in base URL, model set, pricing/quota semantics, and endpoint shape.

OpenCode Zen is a mixed-protocol catalog: current docs list some Zen models under `/responses`, some under `/messages`, some under `/chat/completions`, and Gemini-style model endpoints. The first slice should only make the preset reliable for a known OpenAI-compatible default model through `/zen/v1/chat/completions`. A generated Zen model catalog or resolver table for Anthropic/Gemini/Responses variants is future work.

OpenRouter remains the recommended fallback preset for the user's route chain.

## Routing Recommendation

The default generated config for this workflow should support this simple route shape when the user manually configures multiple providers:

```yaml
routes:
  - alias: sonnet
    strategy: fallback
    targets:
      - model_id: opencode-go-default
      - model_id: opencode-zen-default
      - model_id: openrouter-sonnet
```

The implementation should not force this order globally. This slice should document the shape only. It should not add config-merge or multi-provider setup behavior unless that is already available in existing setup code.

## Internal Components

### Client Profiles

Add a small client profile package or module under `internal/app` or `internal/clientprofile`.

Responsibilities:

- Resolve current config path and client key.
- Build the correct base URL for each client profile.
- Render shell snippets for Unix shells, PowerShell, and cmd where existing activation code already supports platform-specific output.
- Keep Claude-specific settings logic separate from OpenAI-compatible client profiles.
- Keep command output valid for `eval "$(arkroute activate <profile>)"` on Unix shells: executable examples should be comments, not bare commands.

### Preset Catalog

Extend `internal/setup/presets.go` with OpenCode Zen. If this file starts to duplicate provider resolver metadata, follow-up work should consolidate provider preset and resolver metadata into a single small Go catalog.

Do not change the existing OpenCode Go preset in this slice except to add tests that protect its current URL behavior. OpenCode Go can remain `type: auto` because Arkroute already has OpenCode Go protocol rules.

### Docs

Update README and `docs/openai-compatibility.md` with concrete setup sections for:

- Warp Terminal
- OpenCode
- Codex
- Droid
- Claude CLI
- OpenRouter fallback route example

Docs should state clearly:

- Claude CLI uses Anthropic-compatible env vars.
- OpenCode/Codex/Droid use OpenAI-compatible `/v1`.
- DroidRun additionally needs `--provider OpenAILike` and `--api_base`.
- Codex custom gateway setup may need config-file steps depending on the installed Codex CLI version; env-only should be presented as the lightweight path, not guaranteed global config.
- Kiro OAuth/local credential import is not part of this slice.

## Error Handling

Client profile commands should fail clearly when:

- Config cannot be loaded.
- `server.client_key` is missing.
- `server.host` is not loopback.
- `server.port` is invalid.
- The requested client profile is unknown.

Outputs must never print provider API keys. Printing the local Arkroute `server.client_key` is acceptable because activation already needs to hand it to local clients, but docs must describe it as the local gateway key, not an upstream provider key.

## Testing

Add focused tests for:

- OpenCode Zen preset appears in setup presets and produces a valid config.
- `arkroute activate opencode` and `codex` render OpenAI-compatible `OPENAI_BASE_URL`, `OPENAI_API_KEY`, and `OPENAI_MODEL` snippets.
- `arkroute activate droid` renders the local gateway key, base URL, and model in variables suitable for DroidRun.
- `arkroute activate droid` includes the DroidRun `--provider OpenAILike` and `--api_base` guidance as shell comments.
- `arkroute activate claude` continues to render Anthropic-compatible settings unchanged.
- OpenAI-compatible fixture requests shaped like OpenCode, Codex, and Droid normalize successfully.
- Unknown client profile returns a stable error.
- OpenCode Zen URL generation maps `https://opencode.ai/zen/v1` to `https://opencode.ai/zen/v1/chat/completions`.
- OpenCode Go URL generation remains unchanged for `https://opencode.ai/zen/go` and `https://opencode.ai/zen/go/v1`.
- Preset tests protect OpenCode Zen's `base_url`, `type`, `default_model`, `default_alias`, and route alias.

Run before completion:

```sh
go test -count=1 ./...
npm test --prefix npm/arkroute
```

## Future Work

Useful later, but not part of this slice:

- Health-aware fallback and provider cooldowns.
- Kiro/Codex/Claude OAuth import.
- Provider quota/status display.
- Writing config files for OpenCode/Codex/Droid after the env-only profiles prove stable.
- Generated OpenCode Zen model catalog covering `/responses`, `/messages`, `/chat/completions`, and Gemini-style model endpoints.
- A generated provider catalog shared by setup presets, protocol resolver, docs, and tests.
