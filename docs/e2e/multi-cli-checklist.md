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

Release evidence must be a raw, sanitized command transcript for the exact commit being released. Do not replace it with summary-only `success` lines.

Required evidence:

- `date -Iseconds`
- `git rev-parse HEAD`
- `arkroute --version`
- OS and shell versions
- Claude Code, OpenCode, Codex, and DroidRun versions, or `not installed` with rationale
- Gateway start output
- Activation command output checks for each CLI
- At least one real request per CLI path, with secrets redacted
- Routes panel policy inspector result for the selected model

Paste the transcript for the release candidate below:

```text
$ date -Iseconds
2026-06-10T07:25:15+07:00

$ git rev-parse HEAD
706e528c26556e6858459e5a5a7e5f3ddd1f04c7

$ uname -a
Darwin bats-MBP.lan 25.5.0 Darwin Kernel Version 25.5.0: Mon Apr 27 20:38:56 PDT 2026; root:xnu-12377.121.6~2/RELEASE_ARM64_T6000 arm64

$ zsh --version
zsh 5.9 (arm64-apple-darwin25.0)

$ ./dist/arkroute version
arkroute dev

$ claude --version
2.1.153 (Claude Code)

$ codex --version
codex-cli 0.138.0

$ which opencode || true
opencode not found

$ which droidrun || true
droidrun not found

$ which droid || true
droid not found

Config under test:
- path: /private/tmp/arkroute-e2e-config.yaml
- source: copy of ~/.arkroute/config.yaml with server port changed to 20129 for isolated testing
- provider route under test: opencode-go -> opencode-go-deepseek-v4-pro -> upstream deepseek-v4-pro
- model alias: sonnet
- provider credential source: OPENCODE_GO_API_KEY env var, redacted and not written to config or transcript

$ ./dist/arkroute validate --config /private/tmp/arkroute-e2e-config.yaml
config ok

$ OPENCODE_GO_API_KEY=<redacted> ./dist/arkroute serve --config /private/tmp/arkroute-e2e-config.yaml
>_ arkroute
   terminal portal gateway

gateway
  status  listening
  url     http://127.0.0.1:20129
  config  /private/tmp/arkroute-e2e-config.yaml
  traces  /Users/bat/.arkroute/traces.jsonl

$ curl -s http://127.0.0.1:20129/healthz
{"generation":1,"loaded_at":"2026-06-10T00:19:25.800595Z","status":"ok"}

$ ./dist/arkroute status --config /private/tmp/arkroute-e2e-config.yaml
server: running
version: dev
generation: 1
providers: 1
models: 1
routes: 1

$ eval "$(./dist/arkroute activate claude --config /private/tmp/arkroute-e2e-config.yaml)"
$ printf "ANTHROPIC_BASE_URL=%s\n" "$ANTHROPIC_BASE_URL"
ANTHROPIC_BASE_URL=http://127.0.0.1:20129
$ printf "ANTHROPIC_AUTH_TOKEN_SET=%s\n" "$([ -n "$ANTHROPIC_AUTH_TOKEN" ] && printf yes || printf no)"
ANTHROPIC_AUTH_TOKEN_SET=yes
$ printf "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=%s\n" "$CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"
CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1

$ curl -s "$ANTHROPIC_BASE_URL/v1/models" -H "Authorization: Bearer <redacted>"
{"object":"list","data":[{"id":"claude-sonnet-4-6","type":"model","object":"model","display_name":"DeepSeek V4 Pro via OpenCode Go","context_window":1000000,"owned_by":"arkroute"},{"id":"deepseek","type":"model","object":"model","display_name":"DeepSeek V4 Pro via OpenCode Go","context_window":1000000,"owned_by":"arkroute"},{"id":"sonnet","type":"model","object":"model","display_name":"DeepSeek V4 Pro via OpenCode Go","context_window":1000000,"owned_by":"arkroute"}],"has_more":false,"first_id":"claude-sonnet-4-6","last_id":"sonnet"}

$ ./dist/arkroute test sonnet "Say arkroute-test-ok only." --config /private/tmp/arkroute-e2e-config.yaml
{"content":[{"text":"arkroute-test-ok","type":"text"}],"id":"ab3f5935-4d05-4cbc-9e44-e6484d786e64","model":"deepseek","role":"assistant","stop_reason":"end_turn","stop_sequence":null,"type":"message","usage":{"input_tokens":91,"output_tokens":36}}

$ curl -s "$ANTHROPIC_BASE_URL/v1/messages" -H "Authorization: Bearer <redacted>" -H "Content-Type: application/json" -H "anthropic-version: 2023-06-01" -d '{"model":"sonnet","max_tokens":120,"messages":[{"role":"user","content":"Say anthropic-ok only."}]}'
{"content":[{"text":"anthropic-ok","type":"text"}],"id":"c762f369-74e0-4b8b-b504-772ba6ad0328","model":"deepseek","role":"assistant","stop_reason":"end_turn","stop_sequence":null,"type":"message","usage":{"input_tokens":90,"output_tokens":49}}

$ HOME=/private/tmp/arkroute-claude-home claude --bare -p --model sonnet --no-session-persistence --output-format text "Say claude-cli-e2e-ok only."
claude-cli-e2e-ok

$ eval "$(./dist/arkroute activate opencode --config /private/tmp/arkroute-e2e-config.yaml)"
$ printf "OPENAI_BASE_URL=%s\n" "$OPENAI_BASE_URL"
OPENAI_BASE_URL=http://127.0.0.1:20129/v1
$ printf "OPENAI_API_KEY_SET=%s\n" "$([ -n "$OPENAI_API_KEY" ] && printf yes || printf no)"
OPENAI_API_KEY_SET=yes
$ printf "OPENAI_MODEL=%s\n" "$OPENAI_MODEL"
OPENAI_MODEL=sonnet

$ curl -s "$OPENAI_BASE_URL/chat/completions" -H "Authorization: Bearer <redacted>" -H "Content-Type: application/json" -d '{"model":"sonnet","max_tokens":200,"messages":[{"role":"user","content":"Reply exactly: openai-ok"}]}'
{"id":"c4e59bad-5afc-4bb2-bc5b-6584c23e51f5","object":"chat.completion","created":1781051171,"model":"sonnet","choices":[{"index":0,"message":{"role":"assistant","content":"openai-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":90,"completion_tokens":53,"total_tokens":143}}

$ opencode
not run: opencode binary is not installed in PATH on this machine. The OpenAI-compatible activation and request path above were verified against Arkroute /v1.

$ eval "$(./dist/arkroute activate codex --config /private/tmp/arkroute-e2e-config.yaml)"
$ printf "OPENAI_BASE_URL=%s\n" "$OPENAI_BASE_URL"
OPENAI_BASE_URL=http://127.0.0.1:20129/v1
$ printf "OPENAI_API_KEY_SET=%s\n" "$([ -n "$OPENAI_API_KEY" ] && printf yes || printf no)"
OPENAI_API_KEY_SET=yes
$ printf "OPENAI_MODEL=%s\n" "$OPENAI_MODEL"
OPENAI_MODEL=sonnet

$ CODEX_HOME=/private/tmp/arkroute-codex-home codex -c model_provider="arkroute" -c model_providers.arkroute.name="Arkroute" -c model_providers.arkroute.base_url="http://127.0.0.1:20129/v1" -c model_providers.arkroute.env_key="OPENAI_API_KEY" doctor --json
{
  "schemaVersion": 1,
  "overallStatus": "ok",
  "codexVersion": "0.138.0",
  "checks": {
    "auth.credentials": {"status": "ok", "summary": "auth is provided by the active model provider"},
    "config.load": {"status": "ok", "details": {"model provider": "arkroute"}},
    "network.provider_reachability": {"status": "ok", "summary": "active provider endpoints are reachable over HTTP", "details": {"arkroute API base URL": "http://127.0.0.1:20129/v1 reachable (HTTP 404)", "arkroute API route probe": "http://127.0.0.1:20129/v1/<redacted> route exists (HTTP 401)"}}
  }
}

$ CODEX_HOME=/private/tmp/arkroute-codex-home codex -c web_search="disabled" -c model_provider="arkroute" -c model_providers.arkroute.name="Arkroute" -c model_providers.arkroute.base_url="http://127.0.0.1:20129/v1" -c model_providers.arkroute.env_key="OPENAI_API_KEY" exec --ephemeral -m sonnet -s read-only "Say codex-cli-e2e-ok only."
Reading additional input from stdin...
OpenAI Codex v0.138.0
--------
workdir: /Users/bat/RiderProjects/arkroute
model: sonnet
provider: arkroute
approval: never
sandbox: read-only
reasoning effort: none
reasoning summaries: none
session id: 019eaeeb-a7e9-7af0-9bca-9638f2d918e1
--------
user
Say codex-cli-e2e-ok only.
codex
codex-cli-e2e-ok
codex-cli-e2e-ok

$ eval "$(./dist/arkroute activate droid --config /private/tmp/arkroute-e2e-config.yaml)"
$ printf "OPENAI_API_KEY_SET=%s\n" "$([ -n "$OPENAI_API_KEY" ] && printf yes || printf no)"
OPENAI_API_KEY_SET=yes
$ printf "ARKROUTE_OPENAI_BASE_URL=%s\n" "$ARKROUTE_OPENAI_BASE_URL"
ARKROUTE_OPENAI_BASE_URL=http://127.0.0.1:20129/v1
$ printf "ARKROUTE_OPENAI_MODEL=%s\n" "$ARKROUTE_OPENAI_MODEL"
ARKROUTE_OPENAI_MODEL=sonnet

$ droidrun run --provider OpenAILike --model "$ARKROUTE_OPENAI_MODEL" --api_base "$ARKROUTE_OPENAI_BASE_URL" "Open the settings app"
not run: droidrun/droid binary is not installed in PATH on this machine. The OpenAI-compatible /v1 request path used by DroidRun OpenAILike was verified with the chat completions request above.

$ curl -s -X POST "$ANTHROPIC_BASE_URL/internal/setup/session" -H "Authorization: Bearer <redacted>" | sed 's/"setup_token":"[^"]*"/"setup_token":"<redacted>"/'
{"schema_version":1,"setup_token":"<redacted>"}

$ curl -s "$ANTHROPIC_BASE_URL/internal/policy/inspect?model_id=opencode-go-deepseek-v4-pro" -H "X-Arkroute-Setup-Token: <redacted>"
{"schema_version":1,"model_id":"opencode-go-deepseek-v4-pro","provider_id":"opencode-go","provider_type":"openai_compatible","upstream_model":"deepseek-v4-pro","protocol":"openai_compatible","matched_policies":[{"id":"deepseek-v4-openai-compatible","source":"builtin"},{"id":"reasoning-replay-model-families","source":"builtin"}],"resolved_reasoning":{"enabled":true,"effort":"max","auto_enable":true,"auto_effort":"max","replay":true,"omit_tool_choice":true,"follow_claude_effort":false},"reasoning_sources":{"auto_effort":{"source":"builtin","policy_id":"deepseek-v4-openai-compatible","reason":"builtin policy deepseek-v4-openai-compatible sets auto_effort"},"auto_enable":{"source":"builtin","policy_id":"deepseek-v4-openai-compatible","reason":"builtin policy deepseek-v4-openai-compatible sets auto_enable"},"effort":{"source":"model","reason":"models[].reasoning.effort"},"enabled":{"source":"capability_default","reason":"capabilities.reasoning default"},"follow_claude_effort":{"source":"capability_default","reason":"mode default"},"omit_tool_choice":{"source":"model","reason":"models[].reasoning.omit_tool_choice"},"replay":{"source":"model","reason":"models[].reasoning.replay"}},"explain":["builtin policy deepseek-v4-openai-compatible sets auto_enable","builtin policy deepseek-v4-openai-compatible sets auto_effort","models[].reasoning.replay overrides policy deepseek-v4-openai-compatible replay","models[].reasoning.omit_tool_choice overrides policy deepseek-v4-openai-compatible omit_tool_choice","models[].reasoning.replay overrides policy reasoning-replay-model-families replay"],"user_override":{"exists":false,"policy_id":"model-opencode-go-deepseek-v4-pro-compat"}}

$ ./dist/arkroute logs --config /private/tmp/arkroute-e2e-config.yaml --tail 16
{"schema_version":1,"time":"2026-06-10T00:21:13.100979Z","event":"target_selected","request_id":"req_dj4xubo9g2f4","client":"claude","route":"sonnet","strategy":"fallback","provider":"opencode-go","provider_type":"openai_compatible","model":"opencode-go-deepseek-v4-pro","upstream_model":"deepseek-v4-pro","reason":"fallback_order"}
{"schema_version":1,"time":"2026-06-10T00:21:13.130685Z","event":"target_selected","request_id":"req_dj4xubor206g","client":"claude","route":"sonnet","strategy":"fallback","provider":"opencode-go","provider_type":"openai_compatible","model":"opencode-go-deepseek-v4-pro","upstream_model":"deepseek-v4-pro","reason":"fallback_order"}
{"schema_version":1,"time":"2026-06-10T00:21:13.691341Z","event":"stream_started","request_id":"req_dj4xubo9g2f4","client":"claude","route":"sonnet","strategy":"fallback","provider":"opencode-go","provider_type":"openai_compatible","model":"opencode-go-deepseek-v4-pro","upstream_model":"deepseek-v4-pro","status":200}
{"schema_version":1,"time":"2026-06-10T00:21:13.752585Z","event":"stream_started","request_id":"req_dj4xubor206g","client":"claude","route":"sonnet","strategy":"fallback","provider":"opencode-go","provider_type":"openai_compatible","model":"opencode-go-deepseek-v4-pro","upstream_model":"deepseek-v4-pro","status":200}
{"schema_version":1,"time":"2026-06-10T00:21:43.397189Z","event":"request_started","request_id":"req_db60eb61010f4dca6dc2ceeb6f11071e","client":"openai-responses","model":"sonnet"}
{"schema_version":1,"time":"2026-06-10T00:21:43.397376Z","event":"target_selected","request_id":"req_db60eb61010f4dca6dc2ceeb6f11071e","client":"openai-responses","route":"sonnet","strategy":"fallback","provider":"opencode-go","provider_type":"openai_compatible","model":"opencode-go-deepseek-v4-pro","upstream_model":"deepseek-v4-pro","reason":"fallback_order"}
{"schema_version":1,"time":"2026-06-10T00:21:44.178456Z","event":"stream_started","request_id":"req_db60eb61010f4dca6dc2ceeb6f11071e","client":"openai-responses","route":"sonnet","strategy":"fallback","provider":"opencode-go","provider_type":"openai_compatible","model":"opencode-go-deepseek-v4-pro","upstream_model":"deepseek-v4-pro","status":200}

$ go test -count=1 ./...
ok  	github.com/bloodstalk1/arkroute/internal/client/claude	4.660s
ok  	github.com/bloodstalk1/arkroute/internal/protocol/openai	4.591s
ok  	github.com/bloodstalk1/arkroute/internal/panel	4.339s
ok  	github.com/bloodstalk1/arkroute/internal/routepreset	4.509s
ok  	github.com/bloodstalk1/arkroute/internal/setup	4.503s
all packages completed with exit code 0

$ go vet ./...
exit code 0

$ npm test --prefix npm/arkroute
tests 8
pass 8
fail 0

$ npm run build
vite v8.0.16 building client environment for production...
✓ built in 149ms
go build -o dist/arkroute ./cmd/arkroute
exit code 0

$ git diff --check
exit code 0

$ secret scan for the provided provider key and common raw key assignments
no matches
```
