# OpenAI Compatibility

Arkroute exposes a local OpenAI-compatible ingress for clients that can point at a custom `/v1` base URL. This compatibility layer maps OpenAI-shaped requests into Arkroute routing and provider adapters; it does not proxy the full OpenAI Platform yet.

## Base URL And Auth

Use Arkroute's local server URL with the `/v1` prefix:

```sh
export OPENAI_BASE_URL="http://127.0.0.1:2002/v1"
export OPENAI_API_KEY="<server.client_key from ~/.arkroute/config.yaml>"
```

All OpenAI-compatible endpoints require:

```text
Authorization: Bearer <server.client_key>
```

Model names are Arkroute route aliases or exposed model aliases, for example `sonnet` or `sonnet-or`.

## Endpoint Matrix

| Endpoint | Status | Notes |
| --- | --- | --- |
| `GET /v1/models` | Supported | Returns OpenAI list shape with enabled Arkroute route/model aliases. |
| `POST /v1/chat/completions` | Supported | Text chat, streaming, tool calls, tool results, system/developer/user/assistant/tool roles. |
| `POST /v1/responses` | Supported subset | Text input/output, streaming text events, function tools, `instructions`, `output_text`. |
| `POST /v1/embeddings` | Not implemented | Planned for a later OpenAI Platform phase. |
| `POST /v1/completions` | Not implemented | Planned as a legacy adapter phase. |
| `POST /v1/moderations` | Not implemented | Planned where providers support moderation. |
| Files, batches, vector stores | Not implemented | Planned local storage phases. |
| Images, audio, realtime | Not implemented | Planned provider-backed or realtime phases. |

Unsupported endpoints return OpenAI-style JSON errors when they are routed through the OpenAI ingress.

## Chat Completions

Supported request features:

- `model`
- `messages` with `system`, `developer`, `user`, `assistant`, and `tool` roles
- string content or text content parts with `type: "text"` or `type: "input_text"`
- function tools with `type: "function"`
- assistant `tool_calls`
- tool result messages via `tool_call_id`
- `tool_choice`
- `max_tokens` and `max_completion_tokens`
- `temperature`
- `stream`
- `reasoning_effort`

Accepted but currently ignored compatibility fields:

- `metadata`
- `user`
- `top_p`
- `presence_penalty`
- `frequency_penalty`
- `parallel_tool_calls`
- `stream_options`
- `response_format: {"type":"text"}`

Explicit unsupported errors:

- `n > 1`
- structured `response_format` such as `json_object` or `json_schema`
- multimodal content parts such as `image_url` or `input_audio`
- `logprobs` and `top_logprobs`
- audio output options and non-text `modalities`
- non-function tool types

Example:

```sh
curl http://127.0.0.1:2002/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "messages": [{"role": "user", "content": "hello"}],
    "max_completion_tokens": 256
  }'
```

## Responses

Supported request features:

- `model`
- `input` as a string
- `input` as message items with `system`, `developer`, `user`, or `assistant` roles
- content parts with `type: "input_text"`, `type: "output_text"`, or `type: "text"`
- `instructions`
- function tools with `type: "function"`
- function call outputs with `type: "function_call_output"`
- `tool_choice`
- `max_output_tokens`
- `temperature`
- `stream`
- `reasoning.effort`

Accepted but currently ignored compatibility fields:

- `metadata`
- `user`
- `parallel_tool_calls`
- `truncation`

Explicit unsupported errors:

- `previous_response_id`
- `store: true`
- non-empty `include`
- hosted tools such as `web_search_preview` or `file_search`
- multimodal input parts such as `input_image`
- structured `text.format` such as `json_schema`
- non-text `modalities`

Example:

```sh
curl http://127.0.0.1:2002/v1/responses \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "instructions": "Be concise.",
    "input": "hello",
    "max_output_tokens": 256
  }'
```

## Client Setup

For any OpenAI-compatible client, use:

- Base URL: `http://127.0.0.1:2002/v1`
- API key: the Arkroute `server.client_key`
- Model: an Arkroute route alias, usually `sonnet`

This pattern applies to clients such as Cursor, OpenCode, Cline, Continue, Codex-style OpenAI SDK users, and direct OpenAI SDK integrations when they support a custom OpenAI base URL.

Arkroute can print client-specific snippets:

```sh
eval "$(arkroute activate opencode)"
eval "$(arkroute activate codex)"
eval "$(arkroute activate droid)"
```

`opencode` and `codex` print `OPENAI_BASE_URL`, `OPENAI_API_KEY`, and `OPENAI_MODEL`. Codex custom gateway behavior can vary by installed Codex CLI version; if env-only setup is ignored, configure the Codex provider/base URL in Codex's config file and keep Arkroute's `/v1` base URL and local client key.

`droid` prints `OPENAI_API_KEY`, `ARKROUTE_OPENAI_BASE_URL`, and `ARKROUTE_OPENAI_MODEL`, plus a commented DroidRun command using `--provider OpenAILike` and `--api_base`.

JavaScript SDK example:

```js
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "http://127.0.0.1:2002/v1",
  apiKey: process.env.OPENAI_API_KEY,
});

const response = await client.chat.completions.create({
  model: "sonnet",
  messages: [{ role: "user", content: "hello" }],
});
```

Python SDK example:

```py
from openai import OpenAI
import os

client = OpenAI(
    base_url="http://127.0.0.1:2002/v1",
    api_key=os.environ["OPENAI_API_KEY"],
)

response = client.responses.create(
    model="sonnet",
    input="hello",
)
```

## Error Shape

OpenAI-compatible ingress errors use this shape:

```json
{
  "error": {
    "message": "unsupported Responses feature: previous_response_id",
    "type": "invalid_request_error",
    "param": "",
    "code": "unsupported_feature"
  }
}
```

The OpenAI ingress should not return Anthropic-style `{"type":"error"}` payloads.
