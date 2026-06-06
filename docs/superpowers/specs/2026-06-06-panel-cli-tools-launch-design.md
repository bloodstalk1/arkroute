# Arkroute Panel CLI Tools Launch Design

Date: 2026-06-06
Status: Ready for implementation-plan review

## Goal

Add a CLI Tools module to the local Arkroute panel so users can start supported coding CLIs from the same place they configure providers and routes.

The first supported tool is Claude Code CLI. When selected, Arkroute should verify that the local gateway is reachable, then launch the real `claude` command with environment variables that route Claude Code through Arkroute:

```sh
ANTHROPIC_BASE_URL=http://127.0.0.1:<server.port>
ANTHROPIC_AUTH_TOKEN=<server.client_key>
ANTHROPIC_API_KEY=<server.client_key>
CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1
```

This should make the happy path panel driven: configure provider, verify gateway, launch Claude Code, and use the Claude model picker against Arkroute's `/v1/models`.

## Reference Read

The `Alishahryar1/free-claude-code` project uses a dedicated `fcc-claude` launcher. Its important behavior is:

- Load current proxy settings.
- Preflight the local proxy with a short health request.
- Find the real `claude` binary.
- Spawn Claude Code with `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, and `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY`.
- Remove stale `ANTHROPIC_*` variables from the child environment before adding proxy-specific values.

Arkroute already has route configuration, Anthropic-compatible ingress, model discovery aliases, and `arkroute activate claude`. This design adapts the launcher idea into the panel instead of copying the whole Free Claude Code env-file model.

## Current State

Arkroute already supports:

- `arkroute activate claude`, which prints shell exports for Claude Code.
- Claude settings writes from setup flows.
- A local panel with Setup, Providers, Models/Routes, Logs, and System views.
- Authenticated local admin endpoints using `server.client_key`.
- `/v1/messages`, `/v1/models`, and setup session endpoints on the running gateway.

Gaps:

- The panel does not have a dedicated client/tool launcher area.
- Users must leave the panel and manually run activation commands.
- Stale shell or Claude settings can still point Claude Code at the wrong base URL.
- There is no UI preflight that says whether Claude Code can be launched against the current Arkroute gateway.

## Non-Goals

This slice does not include:

- Launching Codex, OpenCode, DroidRun, Cursor, Cline, Continue, VS Code, JetBrains, or terminal apps.
- Writing config files for third-party clients beyond the existing Claude settings behavior.
- Managing long-lived background services.
- Running remote commands from the browser.
- Browser terminal emulation or streaming Claude Code output into the panel.
- Provider OAuth import or credential discovery.
- Replacing `arkroute activate claude`.
- Changing Arkroute's route config model to `MODEL_OPUS`, `MODEL_SONNET`, or `MODEL_HAIKU`.

## User-Facing Design

Add a new panel navigation item:

```text
CLI Tools
```

The view shows tool rows or compact cards. The first row is Claude Code CLI.

Claude Code row:

- Tool name: `Claude Code`
- Command: `claude`
- Status fields:
  - CLI binary found or not found.
  - Gateway reachable or unreachable.
  - Model discovery enabled.
  - Active base URL.
- Primary action when supported: `Launch`
- Fallback action: `Copy Env`

`Launch` starts Claude Code through Arkroute. `Copy Env` shows or copies the existing activation snippet as a fallback for users who prefer launching manually from a terminal.

`Launch` must not be shown as available unless the backend reports that launching an interactive child process is usable in the current Arkroute process. In particular, a browser click cannot create an interactive terminal by itself. If Arkroute is running without a usable terminal, the panel should keep the preflight/status experience but make `Copy Env` the available action.

The UI should not expose upstream provider API keys. Showing the local loopback URL and local Arkroute client key in copyable env output is acceptable because those are required for local client activation.

## Backend Design

Add a small CLI tools layer under `internal/app` or a focused internal package.

Responsibilities:

- Inspect configured tool support.
- Resolve the current Arkroute base URL from config.
- Check whether the configured gateway is reachable.
- Find the target CLI binary with `exec.LookPath`.
- Build a sanitized child environment.
- Spawn the child process without blocking the HTTP request.
- Return a compact launch result to the panel.

Suggested endpoints on the gateway-hosted panel:

```text
GET  /internal/cli-tools
POST /internal/cli-tools/claude/launch
```

Both endpoints are local admin endpoints and must require the same bearer auth or setup session protection pattern used by existing internal panel APIs. The implementation should reuse existing auth/session conventions instead of introducing a second auth scheme.

The implementation must mount these endpoints in the same path family used by the React panel:

- Add handlers to `internal/panel.Routes` so temporary panel tests and panel-local behavior are covered.
- Mount those paths from `internal/client/claude.Server.Routes`, the same way `/internal/setup/options`, `/internal/setup/status`, and related panel endpoints are mounted today.

The React panel currently authenticates panel requests with `X-Arkroute-Setup-Token`, not `Authorization`. CLI Tools endpoints used by the panel should follow that setup-session token path. The existing `/internal/setup/session` bearer-auth endpoint remains the way a running gateway issues the short-lived token to the panel.

`GET /internal/cli-tools` returns:

```json
{
  "schema_version": 1,
  "tools": [
    {
      "id": "claude",
      "name": "Claude Code",
      "command": "claude",
      "installed": true,
      "gateway_reachable": true,
      "base_url": "http://127.0.0.1:20128",
      "model_discovery": true,
      "launch_supported": true,
      "launch_blocked_reason": "",
      "activation_command": "eval \"$(arkroute activate claude)\""
    }
  ]
}
```

`POST /internal/cli-tools/claude/launch` returns quickly:

```json
{
  "schema_version": 1,
  "launched": true,
  "pid": 12345,
  "command": "claude"
}
```

If launch fails, it returns a stable JSON error message with a helpful remediation:

- gateway unreachable: start `arkroute serve`
- missing binary: install Claude Code
- interactive launch unsupported: copy the activation command and run Claude Code in a terminal
- invalid config: run `arkroute setup`
- spawn failure: use `Copy Env` and launch manually

## Claude Launch Environment

The child environment should start from the current process environment, then remove existing `ANTHROPIC_*` variables before adding Arkroute-specific values. This avoids stale Anthropic credentials or base URLs taking precedence.

Required child env:

```text
ANTHROPIC_BASE_URL=http://<server.host>:<server.port>
ANTHROPIC_AUTH_TOKEN=<server.client_key>
ANTHROPIC_API_KEY=<server.client_key>
CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1
```

Optional child env:

```text
CLAUDE_CODE_AUTO_COMPACT_WINDOW=190000
```

Do not add the auto-compact env in the first implementation unless tests show it is harmless with the currently supported Claude Code CLI. The first slice should keep behavior close to Arkroute's existing `activate claude` command.

## Process Behavior

The panel request must not wait for Claude Code to exit.

Implementation behavior:

1. Validate config and gateway reachability.
2. Resolve `claude` with `exec.LookPath`.
3. Verify that interactive launch is supported in this Arkroute process.
4. Build sanitized env.
5. Start the process.
6. Return `pid` and command metadata.

Interactive launch support is intentionally strict in the first slice:

- The panel must be hosted by the running gateway, not only by a temporary setup server.
- Arkroute must have usable standard input, output, and error streams for a child CLI.
- If those conditions are not met, `POST /internal/cli-tools/claude/launch` must not spawn `claude`; it should return a stable unsupported-launch error and the panel should direct the user to `Copy Env`.

When launch is supported, the child process should inherit the Arkroute process standard IO so Claude Code remains interactive in the same terminal. Do not redirect Claude Code to null devices; that would make a successful launch unusable.

Stopping or supervising the Claude child process is future work. This slice only launches it.

## Panel UX States

Claude Code should show clear states:

- `Ready`: CLI found and gateway reachable.
- `Gateway offline`: config is valid but gateway preflight failed.
- `Not installed`: `claude` is not found on PATH.
- `Config issue`: config cannot be loaded or local key is missing.
- `Launch unavailable`: gateway and config are valid, but this Arkroute process cannot host an interactive CLI child.
- `Launched`: process start succeeded and a pid is available.

The `Launch` button is enabled only when the row is ready and `launch_supported` is true. `Copy Env` remains available when config can be loaded, even if `claude` is not installed or interactive launch is unavailable.

## Error Handling

Fail clearly when:

- Config cannot be loaded.
- `server.host` is not loopback.
- `server.port` is invalid.
- `server.client_key` is empty.
- Gateway preflight fails.
- Claude Code binary cannot be found.
- Interactive launch is unavailable.
- Process spawn fails.

Errors returned to the panel must not include upstream provider API keys. Local client keys may be present only in explicit copy-env output, not in generic error text.

## Testing

Add focused tests for:

- CLI tool status reports Claude installed or missing through an injectable lookup function.
- Gateway preflight success and failure.
- Claude launch env removes stale `ANTHROPIC_*` values.
- Claude launch env sets Arkroute base URL, auth token, API key, and model discovery.
- Launch returns a pid and does not wait for child exit when process start succeeds.
- Launch is blocked, without spawning a process, when no interactive terminal is available.
- Temporary setup-panel mode reports launch unavailable while still exposing copy-env guidance.
- Missing binary returns a stable error.
- Gateway unreachable returns a stable error.
- Panel route handlers require the same setup-session protection as existing panel endpoints.
- CLI Tools routes are mounted through both `panel.Routes` and `client/claude.Server.Routes`.
- The CLI Tools panel renders the Claude Code row and disables Launch when not ready.

Run before completion:

```sh
go test -count=1 ./...
npm run build:frontend
```

If frontend tests are added later, include them in the verification command list.

## Future Work

Useful later, but not part of this slice:

- Add Codex, OpenCode, DroidRun, and other tool rows.
- Add per-tool command arguments and profiles.
- Add process stop/restart controls.
- Stream child process logs or last launch diagnostics into the panel.
- Add a dedicated `arkroute claude [args...]` CLI wrapper for terminal-first users.
- Add model-tier convenience routing inspired by `MODEL_OPUS`, `MODEL_SONNET`, and `MODEL_HAIKU`.
