# Arkroute Setup Panel, NPM Packaging, And Uninstall Design

Date: 2026-06-04
Status: Ready for user review

## Goal

Make Arkroute installable and usable without cloning the repository, building Go, or hand-editing YAML. A new user should be able to run:

```sh
npm install -g arkroute
arkroute setup
```

Then choose a provider in a local setup panel, save the config, activate Claude Code, and start routing through Arkroute.

The same lifecycle must also support setup later, returning to the panel later, and uninstalling Arkroute integration safely.

## Design Read

Arkroute's panel is a local developer-tool setup and control surface, not a marketing page. The UI should feel like a premium utilitarian admin tool: clear forms, compact status, readable tables, restrained color, and direct actions.

Use the `minimalist-ui` direction as the primary visual language:

- light mode first
- warm off-white canvas or clean white canvas
- 8px radius for cards and panels
- low-shadow or no-shadow surfaces
- one restrained accent color
- clear inline errors and empty states
- monospace metadata for ports, paths, env var names, route aliases, and status details

Use a small amount of `industrial-brutalist-ui` only for developer-tool texture:

- compact mono labels
- route and provider status chips
- trace/log rows
- rigid table alignment

Do not use a landing-page hero, decorative gradient orbs, purple AI gradients, large marketing cards, or animation-heavy Awwwards patterns.

## Product Decisions

The selected product direction is:

1. `arkroute setup` opens a local setup panel by default.
2. The panel is embedded in the Go binary and served from the local Arkroute process.
3. The terminal setup flow exists only as a fallback for headless or browserless environments.
4. `arkroute panel` reopens the panel after initial setup.
5. The panel includes a `Setup later` action.
6. `arkroute uninstall` removes integration safely and keeps local config by default.
7. `arkroute uninstall --purge --yes` deletes local Arkroute data only after explicit confirmation.
8. NPM distribution uses prebuilt platform packages, not `postinstall` binary downloads.

## Implementation Slices

The lifecycle is one product feature, but it should be implemented in slices so each slice leaves the repo working:

1. Setup core: bootstrap config, setup presets, `arkroute setup --no-browser`, `arkroute panel --no-browser`, and serve guidance when no provider exists.
2. Panel core: embedded static panel, setup provider save, setup later, Claude activation, reload after save when the running gateway owns the panel.
3. Uninstall: Claude settings removal, default keep-config behavior, and explicit purge confirmation through the CLI. A panel uninstall page is a future control-plane enhancement, not part of the current shipped setup panel.
4. NPM packaging: launcher package, platform packages, release build outputs, local package install verification.

The panel UI can start with the Setup and System areas, then add Providers and Models/Routes management once the save/reload path is stable.

## Non-Goals

This project does not include:

- cloud sync
- remote dashboard access
- OAuth provider login
- multi-user admin
- accounts, billing, or hosted control plane
- editing arbitrary YAML in the browser
- external database storage
- background service installation
- automatic binary self-removal
- a full React or Next.js frontend app
- Codex, Gemini CLI, Cursor, OpenCode, Cline, or Continue integration beyond preserving future room for them

## NPM Distribution

Arkroute should publish an npm package that behaves like a native CLI install:

```sh
npm install -g arkroute
arkroute setup
```

The main package should be a thin JavaScript launcher. It should not contain Go source, should not compile Go during install, and should not download binaries during `postinstall`.

Recommended package layout:

```text
npm/
  arkroute/
    package.json
    bin/arkroute.js
  platform/
    darwin-arm64/package.json
    darwin-x64/package.json
    linux-arm64/package.json
    linux-x64/package.json
    win32-x64/package.json
```

Main package:

- package name: `arkroute` if available
- fallback package name: `@bloodstalk1/arkroute` if the unscoped npm name is unavailable
- `bin.arkroute` points to `bin/arkroute.js`
- `optionalDependencies` list platform packages
- no `postinstall` download script

Platform package names if the `@arkroute` scope is available:

- `@arkroute/darwin-arm64`
- `@arkroute/darwin-x64`
- `@arkroute/linux-arm64`
- `@arkroute/linux-x64`
- `@arkroute/win32-x64`

Fallback scoped names if needed:

- `@bloodstalk1/arkroute-darwin-arm64`
- `@bloodstalk1/arkroute-darwin-x64`
- `@bloodstalk1/arkroute-linux-arm64`
- `@bloodstalk1/arkroute-linux-x64`
- `@bloodstalk1/arkroute-win32-x64`

Each platform package contains only:

- package metadata
- README/license metadata
- one prebuilt Arkroute binary for that OS and architecture

The JavaScript launcher detects `process.platform` and `process.arch`, locates the installed platform package, and forwards all CLI args to the native binary. It should preserve stdio and exit with the binary's exit code.

If the platform package is missing, the launcher prints a short diagnostic:

```text
Arkroute binary for darwin-arm64 is not installed.
Try reinstalling with optional dependencies enabled:
  npm install -g arkroute
```

## Setup Command UX

`arkroute setup` is the main onboarding entry point.

Behavior:

1. Determine config path from `--config` or default `~/.arkroute/config.yaml`.
2. If no config exists, create a local bootstrap config with:
   - `server.host: 127.0.0.1`
   - default port `20128`
   - generated `server.client_key`
   - Claude client discovery enabled
   - no enabled provider, model, or route if the user has not selected one yet
3. Start a temporary local setup panel server if the main gateway is not running.
4. If the main gateway is already running, open the panel on that process.
5. Print the panel URL.
6. Try to open the browser unless `--no-browser` is set.
7. Fall back to terminal instructions when browser open fails.

If the requested setup port is unavailable, Arkroute should try the next available loopback port for the temporary setup server and print the actual URL. It should not silently change the configured gateway port unless the user saves that change from the panel.

When `arkroute setup` starts a temporary setup server, the command should keep running until the user completes setup, chooses setup later, or interrupts the command. When setup completes, the panel should show the next command to run and the CLI can shut the temporary server down cleanly. If `arkroute setup` detects and uses an already-running gateway, it can print/open the URL and exit.

Suggested terminal output:

```text
Arkroute setup panel is running:
  http://127.0.0.1:20128/setup#setup_token=<session-token>

Choose a provider, save config, then activate Claude Code from the panel.
```

The token in the URL fragment is short-lived and only authorizes panel setup mutations. It is not the Arkroute local client key.

Flags:

- `--config <path>`: use a custom config path
- `--no-browser`: print URL only
- `--host <host>`: setup server host, restricted to loopback values
- `--port <port>`: setup server port

The command must never bind to a non-loopback host.

## Panel Command UX

`arkroute panel` reopens the local panel after initial setup.

Behavior:

1. Load config.
2. If the gateway is reachable, print and open `http://host:port/panel`.
3. If the gateway is not reachable, start a temporary local panel server using the config path and print/open its URL.
4. If the config has no usable provider, redirect to `/setup`.

This command is for users who want to edit provider, model, route, or integration settings without remembering URLs.

## Setup Later

The panel's setup wizard includes a `Setup later` action.

When selected:

- ensure a local config file exists
- keep the generated local client key
- keep Claude client settings enabled in Arkroute config
- do not enable any provider, model, or route
- show a clear next-step screen with:
  - config path
  - panel URL
  - `arkroute panel`
  - `arkroute setup`

`arkroute serve` with no usable provider should still start. It should print:

```text
arkroute listening on http://127.0.0.1:20128
no provider is configured
run: arkroute setup
```

Gateway request behavior with no route remains explicit: `/v1/messages` should return a normal routing/config error instead of crashing.

## Panel Information Architecture

The panel has five main areas.

### Setup

Shown first when no usable provider exists.

Content:

- provider selector
- provider-specific fields
- model preset selector
- route alias field
- Claude activation option
- `Save & Setup`
- `Setup later`

Provider presets:

- OpenRouter
- Anthropic
- Gemini
- OpenAI-compatible
- OpenCode Go
- Custom

Provider fields:

- provider name
- base URL
- provider type
- API key
- env var name
- upstream model
- exposed alias
- Claude discovery alias
- route alias

Default secret handling:

- Arkroute must not auto-edit shell profile files in the first implementation. Shell profile mutation is cross-shell, destructive when wrong, and hard to undo reliably.
- The panel should offer two explicit storage choices:
  - `Store in Arkroute config`: easiest local setup; writes the provider key into the owner-only config file and redacts it everywhere after save.
  - `Use environment variable`: writes `env:NAME` into config and shows an export command with a copy action.
- The default generated config with no provider should contain no provider secret.
- Provider presets may default to `Use environment variable` for advanced safety, but the UI must make `Store in Arkroute config` available so a new user can complete setup without editing shell files.
- Saved config and panel API responses must not expose raw provider keys after save.

### Providers

Shows configured providers.

Rows:

- provider name
- provider type
- base URL
- key status
- enabled/disabled
- last known health
- actions: edit, disable, test, delete

States:

- configured
- missing env var
- disabled
- unreachable
- auth failed
- unknown

Editing supports:

- base URL
- type
- env var name
- headers
- enabled state

Deleting a provider must explain which models and routes will be affected.

### Models And Routes

Lets users configure routing without editing YAML.

Models view:

- model ID
- provider
- upstream model
- exposed alias
- Claude discovery alias
- capabilities
- enabled state

Routes view:

- route alias
- Claude discovery alias
- strategy: `priority` or `fallback`
- ordered targets
- enabled state

Core actions:

- add model
- edit model
- add route
- reorder fallback targets
- test route with a short prompt

The first version can use simple up/down target reorder controls. Drag and drop is optional later.

### System

Shows operational status.

Content:

- server URL
- config path
- log path
- current version
- config generation if running
- provider/model/route counts
- Claude settings status
- reload config
- write Claude settings
- doctor results
- open logs

### Uninstall

Exposes safe integration removal.

Actions:

- remove Claude integration
- purge Arkroute local data
- show binary removal instructions

The destructive purge action must require a typed confirmation such as:

```text
delete arkroute data
```

## Panel HTTP Surface

The panel should reuse the existing local auth model where possible, but browser panel authentication needs a separate session mechanism because a static browser page cannot safely discover the local client key by itself.

Read endpoints can build on existing internal admin APIs:

- `GET /internal/status`
- `GET /internal/config`
- `GET /internal/routes`
- `GET /internal/health`

New mutating setup endpoints should be local-only and authenticated with a setup session token. The CLI that starts the panel generates the token, passes it to the browser in the setup URL fragment, and the panel sends it back in an `X-Arkroute-Setup-Token` header. The setup token is separate from `server.client_key`, short-lived, and never written to config or logs.

Implemented setup endpoints:

- `GET /setup`
- `GET /panel`
- `GET /panel/assets/*`
- `GET /internal/setup/options`
- `POST /internal/setup/session`
- `POST /internal/setup/provider`
- `POST /internal/setup/later`

Claude activation is handled inline by `POST /internal/setup/provider` through the `activate_claude` request field. Panel uninstall and purge endpoints are reserved for a later control-plane iteration; the current implemented uninstall surface is the CLI.

Responses must use `schema_version: 1`.

Mutating endpoints must:

- load the config from the server's configured path
- apply the requested change
- validate the resulting config
- save atomically
- reload runtime state when the running gateway serves the panel
- return a clear structured error if validation fails

The temporary setup server used by `arkroute setup` can own the config path directly and does not need to proxy through a running gateway unless a gateway is detected.

When a running gateway serves `/panel`, the CLI should request a short-lived panel session token from `POST /internal/setup/session`, authenticated with the local client key loaded from config. Direct manual visits to `/panel` can remain read-only until authenticated.

## Embedded Panel Implementation

The panel should be embedded in the Go binary using `embed`.

Suggested package:

```text
internal/panel/
  assets/
    panel.html
    panel.css
    panel.js
  server.go
```

Use vanilla HTML, CSS, and JavaScript for the first version. This avoids adding a frontend build pipeline and keeps the npm install path focused on binary distribution.

The UI can use simple text symbols only where standard and accessible. Avoid hand-drawn decorative SVGs. If icons are needed in a later implementation, choose one bundled icon strategy and keep it consistent.

## Config Generation

The current `config.MinimalValidConfig` always creates an enabled OpenRouter provider and route. Setup later requires a second config shape.

Add a bootstrap config builder that creates a valid local runtime config without upstream routes:

```text
BootstrapLocalConfig(clientKey)
```

This config should be acceptable for:

- saving to disk
- starting `arkroute serve`
- showing setup panel
- returning clear no-route errors

Validation rules may need to distinguish between:

- runtime config with no providers yet
- invalid config with broken references

Provider preset generation should live in a focused setup/config package instead of being spread across CLI and panel handlers.

Suggested package:

```text
internal/setup/
  presets.go
  planner.go
  validate.go
```

Responsibilities:

- list setup provider presets
- build provider/model/route entries from a submitted setup form
- normalize IDs and aliases
- choose safe env var names
- validate setup-specific input before saving config

## Claude Activation

Panel activation should call the existing Claude settings writer.

Behavior:

- `Save & Activate Claude` writes Arkroute config and updates Claude settings.
- `Save Config Only` writes Arkroute config but leaves Claude settings unchanged.
- The System page can run activation later.
- Activation must preserve unrelated settings in Claude settings.
- Activation should show the settings path that changed.

## Uninstall Design

`arkroute uninstall` removes Arkroute integration safely.

Default behavior:

- remove Arkroute-managed Claude settings keys
- keep `~/.arkroute/config.yaml`
- keep logs
- print where local data remains
- print binary removal instructions

Flags:

- `--config <path>`: custom Arkroute config path
- `--settings <path>`: custom Claude settings path
- `--purge`: delete Arkroute local config/log data after confirmation
- `--yes`: skip interactive confirmation, only valid with explicit destructive flags

Current purge confirmation is explicit and non-interactive:

```text
arkroute uninstall --purge --yes
```

Claude settings removal must not blindly delete a user's unrelated Anthropic configuration. Before removing `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`, or `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY`, Arkroute must compare the settings values with the current Arkroute config. If the base URL or token does not match Arkroute, uninstall should leave those keys unchanged and print a warning. A future force flag can be added if needed, but it is not part of this design.

`--purge` should delete:

- config file at the selected config path
- default trace log at `~/.arkroute/traces.jsonl`
- empty `~/.arkroute` directory if safe

It should not delete:

- arbitrary parent directories
- user shell profiles
- npm package files
- the running binary

Binary removal instructions:

```text
Arkroute integration removed.

To remove the npm-installed binary:
  npm uninstall -g arkroute

Local config kept:
  ~/.arkroute/config.yaml
```

If the binary path can be detected with `os.Executable`, show it as diagnostic only.

## Security And Privacy

Security rules:

- panel binds only to loopback
- no CORS wildcard for mutating endpoints
- raw provider keys are accepted only over loopback
- saved config prefers `env:NAME` references
- raw keys are never returned by panel APIs after save
- config show and status stay redacted
- logs never contain prompts, responses, provider keys, local client key, or auth headers
- purge requires explicit confirmation
- browser open failure must not expose secrets in command output

If a temporary setup server is created before a config exists, it should generate an unguessable setup token and include it in the panel URL fragment. Even after a config exists, browser-based setup mutations should use setup session tokens rather than exposing the normal local client key to JavaScript by default.

Example:

```text
http://127.0.0.1:20128/setup#setup_token=<generated-token>
```

The token should not be written to logs.

## Error Handling

Panel errors should be direct and actionable.

Examples:

- `OPENROUTER_API_KEY is not set. Export it before starting Arkroute, or paste a key and choose how to store it.`
- `The provider URL must be an absolute HTTPS URL.`
- `Claude settings could not be updated: permission denied.`
- `Config saved, but reload failed because server.host changed. Restart Arkroute.`

No panel flow should require users to inspect a Go stack trace.

## Testing Strategy

Unit tests:

- setup preset generation
- env var name normalization
- bootstrap config validation
- provider form to config conversion
- setup session token accepts authorized mutations and rejects missing or invalid tokens
- provider key storage choice writes either a raw redacted config secret or an `env:NAME` reference
- uninstall keeps config by default
- uninstall leaves Claude settings unchanged when current settings do not point at Arkroute
- uninstall purge deletes only selected Arkroute files
- Claude settings removal preserves unrelated env keys
- npm launcher platform resolution logic

Integration tests:

- `arkroute setup --no-browser` creates bootstrap config and prints setup URL
- `arkroute panel --no-browser` prints panel URL
- setup provider API saves config and redacts secret values
- setup later creates no enabled provider but keeps server usable
- running gateway issues a panel session token only after local client key auth
- serve with no provider starts and reports `run: arkroute setup`
- uninstall removes Claude integration but keeps config

Manual verification:

- npm package installs on macOS arm64
- npm package installs on Linux x64
- Windows launcher resolves `.exe`
- panel works without internet
- browser open failure falls back cleanly

## Acceptance Criteria

The feature is complete when:

- a user can install Arkroute through npm without Go installed
- `arkroute setup` opens or prints a local setup panel URL
- the panel can create a provider-backed config from a preset
- the panel can activate Claude settings
- `Setup later` leaves Arkroute in a recoverable local setup state
- `arkroute panel` can reopen setup or control panel later
- `arkroute serve` gives clear guidance when no provider exists
- `arkroute uninstall` removes Claude integration and keeps config by default
- `arkroute uninstall --purge --yes` deletes Arkroute local data only after explicit confirmation
- provider keys and local client keys stay redacted in CLI output, panel responses, and logs
- tests cover setup, panel mutation, npm launcher resolution, and uninstall behavior
