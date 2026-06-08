# Multi-CLI E2E Checklist

Run this outside sandbox before release when provider flow, presets, or CLI setup changes.

## Environment

```sh
arkroute --version
arkroute validate --config ~/.arkroute/config.yaml
arkroute serve --config ~/.arkroute/config.yaml
```

Record:

- Arkroute commit:
- OS and shell:
- Config path:
- Provider route under test:
- Model alias:

## Claude Code

```sh
eval "$(arkroute activate claude)"
claude
```

Checks:

- Claude Code can see Arkroute-discovered model aliases.
- A DeepSeek V4 Pro or equivalent reasoning model returns a response.
- Routes panel policy inspector shows matched builtin and user policies.

## OpenCode

```sh
eval "$(arkroute activate opencode)"
export OPENAI_MODEL=sonnet
opencode
```

Checks:

- OpenCode reaches `http://127.0.0.1:2002/v1`.
- The selected route alias is used.

## Codex

```sh
eval "$(arkroute activate codex)"
export OPENAI_MODEL=sonnet
codex
```

Checks:

- Codex uses Arkroute's local key.
- If env-only configuration is ignored by the installed Codex version, configure its provider file with Arkroute `/v1` and record that result.

## Droid / OpenAI-Like

```sh
eval "$(arkroute activate droid)"
droidrun run --provider OpenAILike --model "$ARKROUTE_OPENAI_MODEL" --api_base "$ARKROUTE_OPENAI_BASE_URL" "Open the settings app"
```

Checks:

- DroidRun sends OpenAI-compatible requests to Arkroute.
- The route appears in traces.

## Transcript

Paste the successful command transcript here before release:

```text
date:
arkroute commit:
claude result:
opencode result:
codex result:
droid result:
policy inspector result:
```
