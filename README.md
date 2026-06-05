# Arkroute

Arkroute is a local AI model router for coding tools. It exposes an Anthropic-compatible local gateway for supported client CLIs, including Claude Code today, and routes requests to OpenAI-compatible, Anthropic-compatible, and Gemini upstream providers.

## Requirements

- A provider API key, usually referenced from config as `env:NAME`
- A client CLI is optional during setup. Claude Code CLI is supported today via `arkroute activate claude`; other clients can be added without changing provider setup.
- **For source builds only:** Go 1.23+ and Node.js

If you want to use Claude Code, install it separately and check it with:

```sh
claude --version
```

## Install With NPM

Install the prebuilt CLI from the npm registry:

```sh
npm install -g arkroute
```

This installs the `arkroute` command globally and automatically selects the correct prebuilt binary for your OS and CPU architecture. Go is not required for npm installs.

Supported npm binary packages:

- macOS arm64: `@arkroute/darwin-arm64`
- macOS x64: `@arkroute/darwin-x64`
- Linux arm64: `@arkroute/linux-arm64`
- Linux x64: `@arkroute/linux-x64`
- Windows x64: `@arkroute/win32-x64`

Verify the install:

```sh
arkroute version
arkroute help
```

If npm installs without optional dependencies, reinstall with optional dependencies enabled:

```sh
npm install -g arkroute --include=optional
```

## First Run

Start the guided setup:

```sh
arkroute setup
```

You can also run `arkroute` with no command; it starts the same guided setup flow.

`arkroute` creates a bootstrap config if needed, starts a loopback-only setup panel, and opens a short-lived URL. The terminal output looks like:

```text
>_ arkroute
   terminal portal gateway

setup panel
  status  running
  panel   http://127.0.0.1:2002/setup#setup_token=<session-token>
  config  ~/.arkroute/config.yaml
```

Choose a provider preset, select whether to store the provider key in config or reference an environment variable, and save the config. Claude Code activation from the panel is optional and only needed if you use Claude Code.

For headless environments:

```sh
arkroute setup --no-browser
```

The printed setup token only authorizes browser setup mutations. It is not the Arkroute local client key and is not written to config.

After setup, start the local gateway:

```sh
arkroute serve
```

When the gateway starts, `arkroute serve` prints the listening URL, config path, and trace log path. If the config has no provider yet, it still starts and prints the next setup command:

```text
>_ arkroute
   terminal portal gateway

gateway
  status  listening
  url     http://127.0.0.1:2002
  config  ~/.arkroute/config.yaml
  traces  ~/.arkroute/traces.jsonl

provider
  status  not configured
  next    arkroute setup
```

You can reopen the setup/control panel later:

```sh
arkroute panel
```

## Daily Usage

1. Start Arkroute in one terminal:

```sh
arkroute serve
```

2. Activate Claude Code in another terminal.

macOS/Linux:

```sh
eval "$(arkroute activate claude)"
claude
```

Windows PowerShell:

```powershell
arkroute activate claude | Invoke-Expression
claude
```

Windows cmd.exe:

```bat
for /f "delims=" %i in ('arkroute activate claude') do %i
claude
```

For persistent Claude Code activation, let Arkroute write Claude settings:

```sh
arkroute activate claude --write-settings
claude
```

Custom Claude settings file:

```sh
arkroute activate claude --write-settings --settings /path/to/settings.json
claude --settings /path/to/settings.json
```

Check status and routing:

```sh
arkroute status
arkroute provider list
arkroute model list
arkroute route list
```

Test a model alias:

```sh
arkroute test sonnet "hello"
```

To create the default example config manually instead of using the setup panel:

```sh
arkroute init
arkroute validate
```

Export any provider keys referenced by your config before starting the server. Example:

```sh
export OPENROUTER_API_KEY="..."
```

## Install From Source

Source installs require Go 1.23+ and Node.js.

Clone the repo and install globally via npm:

```sh
git clone https://github.com/bloodstalk1/arkroute.git
cd arkroute
npm install
npm run local-install
```

This builds the frontend, compiles the Go binary for your platform, and registers the CLI via `npm install -g` with no manual PATH changes.

## OpenCode Go And Auto Protocol Detection

`providers[].type` is optional. When it is omitted or set to `auto`, Arkroute resolves the upstream protocol from the provider catalog, model name, and endpoint shape.

For OpenCode Go, Arkroute defaults to OpenAI-compatible mode, but routes Qwen 3.x and MiniMax/MiMax models through the Anthropic-compatible adapter. DeepSeek, GLM, Kimi, and other OpenAI-compatible models keep using the OpenAI-compatible adapter.

Use `models[].protocol` only as an explicit per-model override:

```yaml
models:
  - id: qwen37
    provider_id: opencode-go
    upstream_model: qwen3.7-max
    protocol: anthropic
    exposed_alias: qwen37
```

## Operator Commands

```sh
arkroute setup
arkroute setup --no-browser
arkroute panel
arkroute panel --no-browser
arkroute config path
arkroute config show
arkroute provider list
arkroute model list
arkroute route list
arkroute status
arkroute reload
arkroute reload --addr http://127.0.0.1:2002
arkroute reload --client-key <current-running-key>
arkroute doctor
arkroute doctor --claude-settings /path/to/settings.json
arkroute test sonnet "hello"
arkroute logs --tail 50
arkroute uninstall
arkroute version --debug
```

## Config

Default config path:

```text
~/.arkroute/config.yaml
```

Generated provider keys use `env:NAME` references. Export provider keys in your shell before starting `arkroute serve`.

## Safety

Arkroute binds to `127.0.0.1` by default and does not log prompt or response bodies. Status, config output, and panel save responses redact provider API keys and the local client key.

Browser setup mutations require a short-lived setup session token sent in the `X-Arkroute-Setup-Token` header. When the panel is served by a running gateway, `arkroute panel` obtains that setup token through the authenticated local admin endpoint.

## Troubleshooting

If Claude keeps connecting to an old Arkroute port after activation, Claude Code settings may be overriding shell environment variables. Check and fix it:

```sh
arkroute doctor
arkroute activate claude --write-settings
```

For a custom settings file:

```sh
arkroute doctor --claude-settings /path/to/settings.json
arkroute activate claude --write-settings --settings /path/to/settings.json
```

`arkroute reload` asks the running server to reload the config path it started with. If you edit `server.host` or `server.port`, the running listener cannot move without restart. Use `arkroute reload --addr http://127.0.0.1:2002` to contact the old listener and get the explicit restart-required error, then restart `arkroute serve`.

If you changed `server.client_key`, use `arkroute reload --client-key <current-running-key>` once so the running server can authenticate the reload request. After reload, new requests must use the new key.

Provider saves from the gateway-hosted panel trigger an internal reload automatically. Provider saves from a temporary setup panel write the config; start or restart `arkroute serve` afterward.

On Unix, sending SIGHUP to the `arkroute serve` process triggers the same reload path as `arkroute reload`.

Some tests use `httptest` and bind a local loopback port. If a restricted sandbox blocks local port binding, run the same `go test -count=1 ./...` command with permission to bind loopback.

## Uninstall

Remove Arkroute from Claude Code settings while keeping local config:

```sh
arkroute uninstall
```

Delete Arkroute local config and logs with explicit non-interactive confirmation:

```sh
arkroute uninstall --purge --yes
```

Remove the npm-installed binary:

```sh
npm uninstall -g arkroute
```

## Development

```sh
go test ./...
go run ./cmd/arkroute help
```

## License

Arkroute is released under the MIT License. See [LICENSE](LICENSE).
