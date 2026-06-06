# CLI Profiles, OpenCode Zen, And Droid Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class Arkroute activation profiles for OpenCode, Codex, and Droid, plus an OpenCode Zen setup preset and docs/tests for this workflow.

**Architecture:** Keep provider setup separate from client activation. Add a small generic client activation layer in `internal/app` that validates local gateway config and delegates shell rendering to the existing OS-specific activation files. Keep OpenCode Zen as a conservative OpenAI-compatible preset with a known `/zen/v1` base URL and leave mixed-protocol Zen catalog support out of this slice.

**Tech Stack:** Go 1.23, existing Arkroute CLI/app/config/setup packages, existing OpenAI-compatible ingress tests, Markdown docs, npm wrapper tests.

---

## File Structure

- Modify `internal/setup/presets.go`: add the `opencode-zen` provider preset.
- Modify `internal/setup/presets_test.go`: assert core preset presence and exact OpenCode Zen preset metadata.
- Modify `internal/setup/planner_test.go`: assert OpenCode Zen setup produces a valid config.
- Modify `internal/adapter/openai/openai_test.go`: protect OpenCode Zen and OpenCode Go chat-completions URL behavior.
- Create `internal/app/client_activation.go`: validate activation config, resolve profile names, build local gateway URLs, and route profile rendering.
- Modify `internal/app/activate_unix.go`: add Unix shell snippets for OpenAI-compatible and Droid profiles.
- Modify `internal/app/activate_windows.go`: add Windows `cmd.exe` snippets for OpenAI-compatible and Droid profiles.
- Modify `internal/cli/cli.go`: route `arkroute activate PROFILE` through the new profile activation API while preserving Claude settings writes.
- Modify `internal/cli/cli_test.go`: cover OpenCode, Codex, Droid, unknown profile, missing key, and unsafe host activation behavior.
- Modify `internal/client/openai/fixtures_test.go`: add Codex and Droid client payload fixtures.
- Modify `README.md`: add daily usage snippets for Warp/OpenCode/Codex/Droid and OpenCode Zen setup notes.
- Modify `docs/openai-compatibility.md`: add client-specific OpenAI-compatible setup notes.

Do not modify the existing OpenCode Go preset fields except tests that lock its URL behavior. Do not write client config files for Codex/OpenCode/Droid in this slice.

---

### Task 1: Add OpenCode Zen Setup Preset

**Files:**
- Modify: `internal/setup/presets_test.go`
- Modify: `internal/setup/planner_test.go`
- Modify: `internal/setup/presets.go`

- [ ] **Step 1: Write failing preset metadata tests**

Add `opencode-zen` to `wantIDs` in `TestPresetsIncludeCoreProviders`, then add this test to `internal/setup/presets_test.go`:

```go
func TestOpenCodeZenPresetMetadata(t *testing.T) {
	var got ProviderPreset
	found := false
	for _, preset := range Presets() {
		if preset.ID == "opencode-zen" {
			got = preset
			found = true
			break
		}
	}
	if !found {
		t.Fatal("opencode-zen preset not found")
	}
	if got.Name != "OpenCode Zen" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Type != "openai_compatible" {
		t.Fatalf("Type = %q", got.Type)
	}
	if got.BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("BaseURL = %q", got.BaseURL)
	}
	if got.DefaultModel != "kimi-k2.6" {
		t.Fatalf("DefaultModel = %q", got.DefaultModel)
	}
	if got.DefaultAlias != "opencode-zen-kimi" {
		t.Fatalf("DefaultAlias = %q", got.DefaultAlias)
	}
	if got.DefaultRoute != "sonnet" {
		t.Fatalf("DefaultRoute = %q", got.DefaultRoute)
	}
	if got.DiscoveryAlias != "claude-sonnet-4-20250514" {
		t.Fatalf("DiscoveryAlias = %q", got.DiscoveryAlias)
	}
}
```

- [ ] **Step 2: Write failing setup planner test**

Add this test to `internal/setup/planner_test.go`:

```go
func TestApplyProviderSetupBuildsOpenCodeZenConfig(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "opencode-zen",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENCODE_API_KEY",
		UpstreamModel: "kimi-k2.6",
		ExposedAlias:  "opencode-zen-kimi",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("ApplyProviderSetup() error = %v", err)
	}
	if out.Providers[0].ID != "opencode-zen" {
		t.Fatalf("provider ID = %q", out.Providers[0].ID)
	}
	if out.Providers[0].Type != "openai_compatible" {
		t.Fatalf("provider type = %q", out.Providers[0].Type)
	}
	if out.Providers[0].BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("provider base URL = %q", out.Providers[0].BaseURL)
	}
	if out.Providers[0].APIKey != "env:OPENCODE_API_KEY" {
		t.Fatalf("provider API key = %q", out.Providers[0].APIKey)
	}
	if out.Models[0].UpstreamModel != "kimi-k2.6" || out.Models[0].ExposedAlias != "opencode-zen-kimi" {
		t.Fatalf("unexpected model config: %+v", out.Models[0])
	}
	if out.Routes[0].Alias != "sonnet" || out.Routes[0].Targets[0].ModelID != "opencode-zen-kimi" {
		t.Fatalf("unexpected route config: %+v", out.Routes[0])
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
```

- [ ] **Step 3: Run tests and verify they fail**

Run:

```sh
go test -count=1 ./internal/setup
```

Expected: FAIL. The failure should mention missing `opencode-zen` or `unknown preset "opencode-zen"`.

- [ ] **Step 4: Add OpenCode Zen preset**

In `internal/setup/presets.go`, insert this preset immediately after the existing `opencode-go` preset:

```go
{
	ID: "opencode-zen", Name: "OpenCode Zen", Type: "openai_compatible",
	BaseURL: "https://opencode.ai/zen/v1", DefaultModel: "kimi-k2.6",
	DefaultAlias: "opencode-zen-kimi", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
	Capabilities: defaultClaudeCapabilities(),
},
```

- [ ] **Step 5: Run tests and verify they pass**

Run:

```sh
go test -count=1 ./internal/setup
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```sh
git add internal/setup/presets.go internal/setup/presets_test.go internal/setup/planner_test.go
git commit -m "feat: add opencode zen preset"
```

---

### Task 2: Lock OpenCode URL Behavior

**Files:**
- Modify: `internal/adapter/openai/openai_test.go`
- Modify only if the test fails: `internal/adapter/openai/url.go`

- [ ] **Step 1: Add URL regression cases**

Update the `tests` map in `TestChatCompletionsURL`:

```go
tests := map[string]string{
	"https://openrouter.ai/api/v1":  "https://openrouter.ai/api/v1/chat/completions",
	"https://example.test":          "https://example.test/v1/chat/completions",
	"https://example.test/v1/":      "https://example.test/v1/chat/completions",
	"https://opencode.ai/zen/v1":    "https://opencode.ai/zen/v1/chat/completions",
	"https://opencode.ai/zen/go":    "https://opencode.ai/zen/go/v1/chat/completions",
	"https://opencode.ai/zen/go/v1": "https://opencode.ai/zen/go/v1/chat/completions",
}
```

- [ ] **Step 2: Run URL test**

Run:

```sh
go test -count=1 ./internal/adapter/openai -run TestChatCompletionsURL
```

Expected: PASS. If it fails with `/v1/v1/` or a missing `/v1`, continue to Step 3. If it passes, skip Step 3.

- [ ] **Step 3: Fix URL builder only when the test fails**

If Step 2 fails, update `ChatCompletionsURL` in `internal/adapter/openai/url.go` so paths ending in `/v1` append only `/chat/completions` and paths ending in `/zen/go` append `/v1/chat/completions`:

```go
func ChatCompletionsURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	if strings.HasSuffix(path, "/chat/completions") {
		parsed.Path = path
		return parsed.String(), nil
	}
	if isOpenCodeGoBase(parsed.Host, path) {
		parsed.Path = path + "/v1/chat/completions"
		return parsed.String(), nil
	}
	parsed.Path = path + "/chat/completions"
	return parsed.String(), nil
}
```

- [ ] **Step 4: Run package tests**

Run:

```sh
go test -count=1 ./internal/adapter/openai
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```sh
git add internal/adapter/openai/openai_test.go internal/adapter/openai/url.go
git commit -m "test: lock opencode chat completions urls"
```

If `internal/adapter/openai/url.go` was not changed, omit it from `git add`.

---

### Task 3: Add Client Activation Rendering

**Files:**
- Create: `internal/app/client_activation.go`
- Modify: `internal/app/activate_unix.go`
- Modify: `internal/app/activate_windows.go`

- [ ] **Step 1: Create generic activation API**

Create `internal/app/client_activation.go`:

```go
package app

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

const (
	ClientProfileClaude   = "claude"
	ClientProfileOpenCode = "opencode"
	ClientProfileCodex   = "codex"
	ClientProfileDroid   = "droid"
)

func PrintClientActivation(w io.Writer, cfg config.Config, profile string) error {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		return fmt.Errorf("client profile is required")
	}
	if !isKnownClientProfile(profile) {
		return fmt.Errorf("unknown client profile %q", profile)
	}
	if err := validateActivationConfig(cfg); err != nil {
		return err
	}
	switch profile {
	case ClientProfileClaude:
		PrintClaudeActivation(w, cfg)
	case ClientProfileOpenCode, ClientProfileCodex:
		printOpenAIClientActivation(w, cfg)
	case ClientProfileDroid:
		printDroidClientActivation(w, cfg)
	}
	return nil
}

func isKnownClientProfile(profile string) bool {
	switch profile {
	case ClientProfileClaude, ClientProfileOpenCode, ClientProfileCodex, ClientProfileDroid:
		return true
	default:
		return false
	}
}

func validateActivationConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		return fmt.Errorf("server.client_key is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if !isLoopbackHost(cfg.Server.Host) {
		return fmt.Errorf("server.host must be loopback for activation")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func localGatewayBaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

func localOpenAIBaseURL(cfg config.Config) string {
	return localGatewayBaseURL(cfg) + "/v1"
}
```

- [ ] **Step 2: Add Unix renderers**

Append these functions to `internal/app/activate_unix.go`:

```go
func printOpenAIClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "export OPENAI_BASE_URL=%s\n", security.ShellQuote(localOpenAIBaseURL(cfg)))
	fmt.Fprintf(w, "export OPENAI_API_KEY=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export OPENAI_MODEL=%s\n", security.ShellQuote("sonnet"))
}

func printDroidClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "export OPENAI_API_KEY=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export ARKROUTE_OPENAI_BASE_URL=%s\n", security.ShellQuote(localOpenAIBaseURL(cfg)))
	fmt.Fprintf(w, "export ARKROUTE_OPENAI_MODEL=%s\n", security.ShellQuote("sonnet"))
	fmt.Fprintln(w, "# droidrun run --provider OpenAILike --model \"$ARKROUTE_OPENAI_MODEL\" --api_base \"$ARKROUTE_OPENAI_BASE_URL\" \"Open the settings app\"")
}
```

- [ ] **Step 3: Add Windows renderers**

Append these functions to `internal/app/activate_windows.go`:

```go
func printOpenAIClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "set OPENAI_BASE_URL=%s\n", localOpenAIBaseURL(cfg))
	fmt.Fprintf(w, "set OPENAI_API_KEY=%s\n", cfg.Server.ClientKey)
	fmt.Fprintf(w, "set OPENAI_MODEL=sonnet\n")
}

func printDroidClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "set OPENAI_API_KEY=%s\n", cfg.Server.ClientKey)
	fmt.Fprintf(w, "set ARKROUTE_OPENAI_BASE_URL=%s\n", localOpenAIBaseURL(cfg))
	fmt.Fprintf(w, "set ARKROUTE_OPENAI_MODEL=sonnet\n")
	fmt.Fprintln(w, "REM droidrun run --provider OpenAILike --model \"%ARKROUTE_OPENAI_MODEL%\" --api_base \"%ARKROUTE_OPENAI_BASE_URL%\" \"Open the settings app\"")
}
```

- [ ] **Step 4: Run app package tests**

Run:

```sh
go test -count=1 ./internal/app
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```sh
git add internal/app/client_activation.go internal/app/activate_unix.go internal/app/activate_windows.go
git commit -m "feat: add client activation renderers"
```

---

### Task 4: Wire CLI Activation Profiles

**Files:**
- Modify: `internal/cli/cli_test.go`
- Modify: `internal/cli/cli.go`

- [ ] **Step 1: Add CLI tests**

Add this helper and tests near the existing activation tests in `internal/cli/cli_test.go`:

```go
func writeActivationConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunActivateOpenAICompatibleProfilesPrintExports(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20128
	path := writeActivationConfig(t, cfg)

	for _, profile := range []string{"opencode", "codex"} {
		t.Run(profile, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"arkroute", "activate", profile, "--config", path}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
			}
			out := stdout.String()
			for _, want := range []string{"OPENAI_BASE_URL", "http://127.0.0.1:20128/v1", "OPENAI_API_KEY", "local-key", "OPENAI_MODEL", "sonnet"} {
				if !strings.Contains(out, want) {
					t.Fatalf("activate %s output missing %q: %q", profile, want, out)
				}
			}
		})
	}
}

func TestRunActivateDroidPrintsDroidRunGuidance(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20128
	path := writeActivationConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "droid", "--config", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"OPENAI_API_KEY", "ARKROUTE_OPENAI_BASE_URL", "http://127.0.0.1:20128/v1", "ARKROUTE_OPENAI_MODEL", "--provider OpenAILike", "--api_base"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activate droid output missing %q: %q", want, out)
		}
	}
}

func TestRunActivateUnknownProfile(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	cfg := config.MinimalValidConfig("local-key")
	path := writeActivationConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "missing", "--config", path}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown client profile") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunActivateRejectsUnsafeHost(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Host = "0.0.0.0"
	path := writeActivationConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "opencode", "--config", path}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "server.host must be loopback") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunActivateRejectsMissingClientKey(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.ClientKey = ""
	path := writeActivationConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "codex", "--config", path}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "server.client_key is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run CLI tests and verify they fail**

Run:

```sh
go test -count=1 ./internal/cli -run 'TestRunActivate(OpenAICompatibleProfiles|Droid|UnknownProfile|Rejects)'
```

Expected: FAIL. The OpenCode/Codex/Droid tests should fail because `arkroute activate` only accepts `claude`.

- [ ] **Step 3: Refactor activate command**

In `internal/cli/cli.go`, replace the current `"activate"` case with:

```go
case "activate":
	return runActivate(args[2:], stdout, stderr)
```

Then add this helper near the other `run*` helpers:

```go
func runActivate(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: arkroute activate PROFILE")
		return 2
	}
	profile := strings.ToLower(strings.TrimSpace(args[0]))
	flags := args[1:]
	path := flagValue(flags, "--config")
	if path == "" {
		path = app.DefaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "activate failed: %v\n", err)
		return 1
	}
	if key := flagValue(flags, "--client-key"); key != "" {
		cfg.Server.ClientKey = key
	}
	settingsPath := flagValue(flags, "--settings")
	if hasFlag(flags, "--write-settings") {
		if profile != app.ClientProfileClaude {
			fmt.Fprintln(stderr, "activate failed: --write-settings is only supported for claude")
			return 1
		}
		if err := app.WriteClaudeSettings(settingsPath, cfg); err != nil {
			fmt.Fprintf(stderr, "activate failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "updated Claude settings: %s\n", app.ClaudeSettingsPath(settingsPath))
		return 0
	}
	if err := app.PrintClientActivation(stdout, cfg, profile); err != nil {
		if strings.Contains(err.Error(), "unknown client profile") {
			fmt.Fprintf(stderr, "activate failed: %v\n", err)
			return 2
		}
		fmt.Fprintf(stderr, "activate failed: %v\n", err)
		return 1
	}
	if profile == app.ClientProfileClaude {
		app.PrintClaudeActivationSettingsWarning(stdout, cfg, settingsPath)
	}
	return 0
}
```

- [ ] **Step 4: Update help text**

In `printHelp`, replace the activate line with:

```go
fmt.Fprintln(w, "  activate          Print client environment exports for claude, opencode, codex, or droid")
```

- [ ] **Step 5: Run CLI tests**

Run:

```sh
go test -count=1 ./internal/cli
```

Expected: PASS.

- [ ] **Step 6: Run app and CLI packages together**

Run:

```sh
go test -count=1 ./internal/app ./internal/cli
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```sh
git add internal/app/client_activation.go internal/app/activate_unix.go internal/app/activate_windows.go internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: add activation profiles for coding clients"
```

---

### Task 5: Add Codex And Droid Compatibility Fixtures

**Files:**
- Modify: `internal/client/openai/fixtures_test.go`

- [ ] **Step 1: Add documented client payload cases**

In `TestChatCompletionsAcceptsDocumentedClientPayloads`, add these two cases to the `tests` slice:

```go
{
	name: "codex cli style",
	body: `{
		"model": "sonnet",
		"messages": [
			{"role": "developer", "content": "You are working inside a local repository."},
			{"role": "user", "content": "Inspect the failing test and propose a fix."}
		],
		"max_completion_tokens": 1024,
		"reasoning_effort": "medium",
		"parallel_tool_calls": true,
		"metadata": {"client": "codex-cli"}
	}`,
},
{
	name: "droidrun openailike style",
	body: `{
		"model": "sonnet",
		"messages": [
			{"role": "system", "content": "Control Android through concise steps."},
			{"role": "user", "content": "Open settings and report the current screen."}
		],
		"max_tokens": 512,
		"temperature": 0.2,
		"top_p": 0.95,
		"user": "droidrun-local"
	}`,
},
```

- [ ] **Step 2: Run fixture tests**

Run:

```sh
go test -count=1 ./internal/client/openai -run TestChatCompletionsAcceptsDocumentedClientPayloads
```

Expected: PASS. If this fails, inspect the exact unsupported field in the failure body and remove only that field from the fixture; keep a client-shaped payload that Arkroute supports.

- [ ] **Step 3: Commit**

Run:

```sh
git add internal/client/openai/fixtures_test.go
git commit -m "test: add codex and droid openai fixtures"
```

---

### Task 6: Document Daily Workflow

**Files:**
- Modify: `README.md`
- Modify: `docs/openai-compatibility.md`

- [ ] **Step 1: Update README daily usage**

In `README.md`, replace the generic OpenAI-compatible env snippet under "Daily Usage" with:

````md
Use OpenCode or OpenAI-compatible Codex sessions through Arkroute:

```sh
eval "$(arkroute activate opencode)"
opencode
```

```sh
eval "$(arkroute activate codex)"
codex
```

Use DroidRun with its OpenAILike provider:

```sh
eval "$(arkroute activate droid)"
droidrun run --provider OpenAILike --model "$ARKROUTE_OPENAI_MODEL" --api_base "$ARKROUTE_OPENAI_BASE_URL" "Open the settings app"
```

All OpenAI-compatible profiles use Arkroute's local `/v1` gateway and the local `server.client_key`, not an upstream provider API key.
````

- [ ] **Step 2: Update README OpenCode section**

Rename `## OpenCode Go And Auto Protocol Detection` to:

```md
## OpenCode Zen, OpenCode Go, And Protocol Detection
```

Add this paragraph immediately after the heading:

```md
OpenCode Zen is available as a setup preset with `base_url: https://opencode.ai/zen/v1`, `type: openai_compatible`, and default model `kimi-k2.6`. Zen contains mixed endpoint families; this preset intentionally starts with a known OpenAI-compatible path through `/chat/completions`.
```

- [ ] **Step 3: Update OpenAI compatibility client setup**

In `docs/openai-compatibility.md`, replace the paragraph beginning with `This pattern applies to clients such as Cursor` with:

````md
This pattern applies to clients such as Cursor, OpenCode, Cline, Continue, Codex-style OpenAI SDK users, and direct OpenAI SDK integrations when they support a custom OpenAI base URL.

Arkroute can print client-specific snippets:

```sh
eval "$(arkroute activate opencode)"
eval "$(arkroute activate codex)"
eval "$(arkroute activate droid)"
```

`opencode` and `codex` print `OPENAI_BASE_URL`, `OPENAI_API_KEY`, and `OPENAI_MODEL`. Codex custom gateway behavior can vary by installed Codex CLI version; if env-only setup is ignored, configure the Codex provider/base URL in Codex's config file and keep Arkroute's `/v1` base URL and local client key.

`droid` prints `OPENAI_API_KEY`, `ARKROUTE_OPENAI_BASE_URL`, and `ARKROUTE_OPENAI_MODEL`, plus a commented DroidRun command using `--provider OpenAILike` and `--api_base`.
````

- [ ] **Step 4: Check docs formatting**

Run:

```sh
rg -n "arkroute activate (opencode|codex|droid)|OpenCode Zen|OpenAILike|server.client_key" README.md docs/openai-compatibility.md
```

Expected: output includes all new profile names, `OpenCode Zen`, `OpenAILike`, and `server.client_key`.

- [ ] **Step 5: Commit**

Run:

```sh
git add README.md docs/openai-compatibility.md
git commit -m "docs: add coding client activation workflow"
```

---

### Task 7: Final Verification

**Files:**
- No direct edits unless verification exposes a bug in this plan's changes.

- [ ] **Step 1: Run focused packages**

Run:

```sh
go test -count=1 ./internal/setup ./internal/adapter/openai ./internal/app ./internal/cli ./internal/client/openai
```

Expected: PASS.

- [ ] **Step 2: Run full Go suite**

Run:

```sh
go test -count=1 ./...
```

Expected: PASS.

- [ ] **Step 3: Run npm wrapper tests**

Run:

```sh
npm test --prefix npm/arkroute
```

Expected: PASS.

- [ ] **Step 4: Inspect final diff**

Run:

```sh
git status --short
git log --oneline -6
```

Expected: `git status --short` contains no uncommitted files from this plan. Existing unrelated user changes that were present before execution can remain, but files touched by this plan should be committed.

- [ ] **Step 5: Final commit if verification changed files**

If Step 1-3 required code or docs fixes after the task commits, make one small verification commit:

```sh
git add internal/setup/presets.go internal/setup/presets_test.go internal/setup/planner_test.go internal/adapter/openai/openai_test.go internal/adapter/openai/url.go internal/app/client_activation.go internal/app/activate_unix.go internal/app/activate_windows.go internal/cli/cli.go internal/cli/cli_test.go internal/client/openai/fixtures_test.go README.md docs/openai-compatibility.md
git commit -m "fix: complete cli profile verification"
```

Do not stage unrelated files.
