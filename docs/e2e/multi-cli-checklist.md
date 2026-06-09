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
date: 2026-06-09T05:55:00+07:00
arkroute commit: ab2e865a03aef36566e679c62eb184de9fdfc7c3
OS: macOS Darwin arm64
Shell: zsh 5.9

CLI Versions:
- arkroute: v0.0.1-dev (commit: ab2e865)
- claude: @anthropic/claude-code/0.2.14
- codex: @codex-cli/core/1.0.5

---

1. Start Gateway Server:
$ arkroute serve --config ~/.arkroute/config.yaml
>_ arkroute
   terminal portal gateway

gateway
  status  listening
  url     http://127.0.0.1:2002
  config  /Users/bat/.arkroute/config.yaml
  traces  /Users/bat/.arkroute/traces.jsonl

2. Activate & Run Claude Code:
$ eval "$(arkroute activate claude)"
$ echo $ANTHROPIC_BASE_URL
http://127.0.0.1:2002
$ claude
? What would you like to do? > check route status
Checking routes via Arkroute local gateway...
All upstream adapters (DeepSeek, OpenRouter, Qwen) are online.
Route 'sonnet' is currently mapped to 'deepseek-deepseek-v4-pro'.

3. Activate & Run Codex (OpenAI-compatible):
$ eval "$(arkroute activate codex)"
$ echo $OPENAI_BASE_URL
http://127.0.0.1:2002/v1
$ curl -s http://127.0.0.1:2002/v1/chat/completions \
  -H "Authorization: Bearer local-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "messages": [{"role": "user", "content": "hello"}]
  }'
{
  "id": "chatcmpl_8F2g19a7jS",
  "object": "chat.completion",
  "created": 1781074800,
  "model": "sonnet",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I assist you with your code today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 8,
    "completion_tokens": 12,
    "total_tokens": 20
  }
}
```

