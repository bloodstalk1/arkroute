# Arkroute Setup Panel NPM Uninstall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the install/setup lifecycle from `npm install -g arkroute` through provider panel setup, setup later, Claude activation, and safe uninstall.

**Architecture:** Keep Arkroute as a single Go binary with an embedded vanilla HTML/CSS/JS panel. Add focused setup planning code for provider presets and config mutation, add short-lived panel session tokens for browser mutations, and add npm wrapper packages that launch prebuilt platform binaries without install-time downloads.

**Tech Stack:** Go 1.23 standard library, `gopkg.in/yaml.v3`, `embed`, vanilla HTML/CSS/JS, npm package metadata and Node.js launcher tests.

---

## Current Implementation Note

This plan records the task-by-task implementation path. The current shipped behavior is:

- `arkroute setup` and fallback `arkroute panel` start a temporary loopback panel with a short-lived setup token.
- A gateway-hosted panel obtains setup tokens through authenticated `/internal/setup/session`.
- Provider save and setup-later mutations are token-authenticated.
- Provider save can activate Claude settings inline through `activate_claude`.
- Gateway-hosted saves trigger runtime reload after writing config.
- `arkroute serve` with no provider prints `run: arkroute setup`.
- Uninstall is currently CLI-based: `arkroute uninstall` keeps config, and destructive purge requires `arkroute uninstall --purge --yes`.

## Scope Map

This plan implements the approved spec in four slices:

- Setup core: bootstrap config, setup presets, no-browser setup/panel commands, serve guidance with no provider.
- Panel core: embedded panel, setup session token, provider save, setup later, Claude activation, reload after save.
- Uninstall: Claude settings cleanup, keep-config default, and explicit purge confirmation through the CLI.
- NPM packaging: JS launcher, platform package templates, release build output layout.

## File Structure

Create:

- `internal/setup/presets.go`: provider preset catalog and setup form types.
- `internal/setup/planner.go`: converts setup form input into a validated `config.Config`.
- `internal/setup/presets_test.go`: preset catalog and env var normalization tests.
- `internal/setup/planner_test.go`: setup form to config behavior tests.
- `internal/panel/session.go`: short-lived panel setup token store.
- `internal/panel/session_test.go`: token accept/reject tests.
- `internal/panel/assets/panel.html`: embedded static panel shell.
- `internal/panel/assets/panel.css`: minimalist utilitarian panel styling.
- `internal/panel/assets/panel.js`: setup form behavior and API calls.
- `internal/panel/server.go`: panel routes and setup mutation endpoints.
- `internal/panel/server_test.go`: panel route and mutation tests.
- `internal/app/setup.go`: CLI-facing setup/panel command orchestration.
- `internal/app/setup_test.go`: setup command behavior tests.
- `internal/app/browser.go`: cross-platform browser opener used by setup and panel commands.
- `internal/app/uninstall.go`: uninstall and purge orchestration.
- `internal/app/uninstall_test.go`: uninstall safety tests.
- `npm/arkroute/package.json`: main npm package metadata.
- `npm/arkroute/bin/arkroute.js`: native binary launcher.
- `npm/arkroute/test/launcher.test.js`: launcher platform resolution tests.
- `npm/platform/darwin-arm64/package.json`
- `npm/platform/darwin-x64/package.json`
- `npm/platform/linux-arm64/package.json`
- `npm/platform/linux-x64/package.json`
- `npm/platform/win32-x64/package.json`

Modify:

- `internal/config/load.go`: add `BootstrapLocalConfig`.
- `internal/config/validate.go`: allow empty provider/model/route bootstrap configs while preserving broken-reference validation.
- `internal/config/config_test.go`: cover bootstrap config validation.
- `internal/app/init.go`: optionally reuse bootstrap helpers without changing current `arkroute init` default behavior.
- `internal/app/serve.go`: print setup guidance when no provider is configured and mount panel routes.
- `internal/app/claude_settings.go`: add safe removal helper for Arkroute-managed Claude env keys.
- `internal/app/claude_settings_test.go`: cover safe removal preserving unrelated settings.
- `internal/cli/cli.go`: dispatch `setup`, `panel`, and `uninstall`.
- `internal/cli/cli_test.go`: CLI parse and output tests.
- `internal/client/claude/server.go`: mount panel/static/setup routes on the gateway server.
- `internal/client/claude/admin.go`: add session endpoint if implemented inside Claude server rather than `internal/panel`.
- `Makefile`: add cross-platform build and npm package staging targets.
- `README.md`: document npm install, setup, panel, uninstall.

## Task 1: Bootstrap Config And Validation

**Files:**
- Modify: `internal/config/load.go:44-89`
- Modify: `internal/config/validate.go:44-138`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests for bootstrap config**

Add to `internal/config/config_test.go`:

```go
func TestValidateAcceptsBootstrapLocalConfig(t *testing.T) {
	cfg := BootstrapLocalConfig("ark-local-key")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() bootstrap error = %v", err)
	}
	if len(cfg.Providers) != 0 || len(cfg.Models) != 0 || len(cfg.Routes) != 0 {
		t.Fatalf("bootstrap config should not create providers/models/routes: %+v", cfg)
	}
	if !cfg.Clients.Claude.Enabled || !cfg.Clients.Claude.ModelDiscovery {
		t.Fatalf("bootstrap Claude settings = %+v", cfg.Clients.Claude)
	}
}

func TestValidateRejectsBrokenReferencesWhenPartialSetupExists(t *testing.T) {
	cfg := BootstrapLocalConfig("ark-local-key")
	cfg.Models = []ModelConfig{{
		ID:            "broken-model",
		ProviderID:    "missing",
		UpstreamModel: "provider/model",
		ExposedAlias:  "broken",
		Enabled:       true,
	}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want missing provider reference")
	}
	if !strings.Contains(err.Error(), "models[0].provider_id") {
		t.Fatalf("error = %q, want models[0].provider_id", err.Error())
	}
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/config -run 'TestValidateAcceptsBootstrapLocalConfig|TestValidateRejectsBrokenReferencesWhenPartialSetupExists'
```

Expected: FAIL because `BootstrapLocalConfig` is undefined.

- [ ] **Step 3: Implement bootstrap config**

Add to `internal/config/load.go` after `MinimalValidConfig`:

```go
func BootstrapLocalConfig(clientKey string) Config {
	return Config{
		Version: CurrentVersion,
		Server: ServerConfig{
			Host:                   "127.0.0.1",
			Port:                   20128,
			ClientKey:              clientKey,
			UpstreamTimeoutSeconds: 600,
		},
		Clients: ClientsConfig{Claude: ClaudeClientConfig{Enabled: true, ModelDiscovery: true}},
		Providers: []ProviderConfig{},
		Models:    []ModelConfig{},
		Routes:    []RouteConfig{},
		Profiles:  map[string]string{},
	}
}
```

`Validate` already accepts empty provider/model/route slices because it only validates entries that exist. Keep the broken-reference checks unchanged.

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```sh
go test -count=1 ./internal/config -run 'TestValidateAcceptsBootstrapLocalConfig|TestValidateRejectsBrokenReferencesWhenPartialSetupExists'
```

Expected: PASS.

- [ ] **Step 5: Run package tests**

Run:

```sh
go test -count=1 ./internal/config
```

Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add internal/config/load.go internal/config/config_test.go
git commit -m "feat: add bootstrap setup config"
```

## Task 2: Setup Presets And Config Planner

**Files:**
- Create: `internal/setup/presets.go`
- Create: `internal/setup/planner.go`
- Create: `internal/setup/presets_test.go`
- Create: `internal/setup/planner_test.go`

- [ ] **Step 1: Write failing preset tests**

Create `internal/setup/presets_test.go`:

```go
package setup

import "testing"

func TestPresetsIncludeCoreProviders(t *testing.T) {
	presets := Presets()
	wantIDs := map[string]bool{
		"openrouter":         false,
		"anthropic":          false,
		"gemini":             false,
		"openai-compatible":  false,
		"opencode-go":        false,
		"custom":             false,
	}
	for _, preset := range presets {
		if _, ok := wantIDs[preset.ID]; ok {
			wantIDs[preset.ID] = true
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Fatalf("preset %q not found in %+v", id, presets)
		}
	}
}

func TestEnvNameForProvider(t *testing.T) {
	tests := []struct {
		providerID string
		want       string
	}{
		{providerID: "openrouter", want: "OPENROUTER_API_KEY"},
		{providerID: "OpenCode Go", want: "OPENCODE_GO_API_KEY"},
		{providerID: "my-provider.io", want: "MY_PROVIDER_IO_API_KEY"},
	}
	for _, tt := range tests {
		if got := EnvNameForProvider(tt.providerID); got != tt.want {
			t.Fatalf("EnvNameForProvider(%q) = %q, want %q", tt.providerID, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Write failing planner tests**

Create `internal/setup/planner_test.go`:

```go
package setup

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestApplyProviderSetupStoresEnvReference(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:       "openrouter",
		APIKeyMode:     APIKeyModeEnv,
		APIKey:         "sk-or-secret",
		EnvName:        "OPENROUTER_API_KEY",
		UpstreamModel:  "anthropic/claude-sonnet-4.5",
		ExposedAlias:   "sonnet-or",
		RouteAlias:     "sonnet",
		ActivateClaude: true,
	})
	if err != nil {
		t.Fatalf("ApplyProviderSetup() error = %v", err)
	}
	if out.Providers[0].APIKey != "env:OPENROUTER_API_KEY" {
		t.Fatalf("provider api key = %q", out.Providers[0].APIKey)
	}
	if out.Models[0].ProviderID != "openrouter" || out.Routes[0].Alias != "sonnet" {
		t.Fatalf("unexpected config: %+v", out)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupCanStoreRawConfigSecret(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    APIKeyModeConfig,
		APIKey:        "sk-ant-secret",
		UpstreamModel: "claude-sonnet-4-20250514",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("ApplyProviderSetup() error = %v", err)
	}
	if out.Providers[0].APIKey != "sk-ant-secret" {
		t.Fatalf("provider api key = %q", out.Providers[0].APIKey)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupRejectsUnknownPreset(t *testing.T) {
	_, err := ApplyProviderSetup(config.BootstrapLocalConfig("local-key"), ProviderSetup{PresetID: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unknown preset") {
		t.Fatalf("error = %v, want unknown preset", err)
	}
}
```

- [ ] **Step 3: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/setup
```

Expected: FAIL because `internal/setup` package functions are undefined.

- [ ] **Step 4: Implement preset catalog**

Create `internal/setup/presets.go`:

```go
package setup

import (
	"strings"
	"unicode"

	"github.com/bloodstalk1/arkroute/internal/config"
)

const (
	APIKeyModeEnv    = "env"
	APIKeyModeConfig = "config"
)

type ProviderPreset struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Type           string              `json:"type"`
	BaseURL        string              `json:"base_url"`
	DefaultModel   string              `json:"default_model"`
	DefaultAlias   string              `json:"default_alias"`
	DefaultRoute   string              `json:"default_route"`
	Headers        map[string]string   `json:"headers,omitempty"`
	Capabilities   config.Capabilities `json:"capabilities"`
	DiscoveryAlias string              `json:"claude_discovery_alias"`
}

func Presets() []ProviderPreset {
	return []ProviderPreset{
		{
			ID: "openrouter", Name: "OpenRouter", Type: "openai_compatible",
			BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "anthropic/claude-sonnet-4.5",
			DefaultAlias: "sonnet-or", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Headers: map[string]string{"X-OpenRouter-Title": "Arkroute"},
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "anthropic", Name: "Anthropic", Type: "anthropic",
			BaseURL: "https://api.anthropic.com", DefaultModel: "claude-sonnet-4-20250514",
			DefaultAlias: "sonnet-anthropic", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "gemini", Name: "Gemini", Type: "gemini",
			BaseURL: "https://generativelanguage.googleapis.com/v1beta", DefaultModel: "gemini-2.5-pro",
			DefaultAlias: "gemini-pro", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "openai-compatible", Name: "OpenAI-compatible", Type: "openai_compatible",
			BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-5.1",
			DefaultAlias: "openai-model", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "opencode-go", Name: "OpenCode Go", Type: "auto",
			BaseURL: "https://opencode.ai/zen/go", DefaultModel: "qwen3.7-max",
			DefaultAlias: "qwen37", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
		{
			ID: "custom", Name: "Custom", Type: "auto",
			BaseURL: "https://example.com/v1", DefaultModel: "provider/model",
			DefaultAlias: "custom-model", DefaultRoute: "sonnet", DiscoveryAlias: "claude-sonnet-4-20250514",
			Capabilities: defaultClaudeCapabilities(),
		},
	}
}

func EnvNameForProvider(providerID string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range providerID {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "PROVIDER"
	}
	return out + "_API_KEY"
}

func defaultClaudeCapabilities() config.Capabilities {
	return config.Capabilities{
		Streaming:       true,
		Tools:           true,
		ToolResults:     true,
		SystemMessages:  true,
		ContextWindow:   200000,
		MaxOutputTokens: 8192,
	}
}
```

- [ ] **Step 5: Implement planner**

Create `internal/setup/planner.go`:

```go
package setup

import (
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type ProviderSetup struct {
	PresetID       string `json:"preset_id"`
	ProviderName   string `json:"provider_name"`
	BaseURL        string `json:"base_url"`
	Type           string `json:"type"`
	APIKeyMode     string `json:"api_key_mode"`
	APIKey         string `json:"api_key"`
	EnvName        string `json:"env_name"`
	UpstreamModel  string `json:"upstream_model"`
	ExposedAlias   string `json:"exposed_alias"`
	RouteAlias     string `json:"route_alias"`
	ActivateClaude bool   `json:"activate_claude"`
}

func ApplyProviderSetup(cfg config.Config, input ProviderSetup) (config.Config, error) {
	preset, ok := findPreset(input.PresetID)
	if !ok {
		return config.Config{}, fmt.Errorf("unknown preset %q", input.PresetID)
	}
	providerID := preset.ID
	providerName := firstNonEmpty(input.ProviderName, preset.Name)
	baseURL := firstNonEmpty(input.BaseURL, preset.BaseURL)
	providerType := firstNonEmpty(input.Type, preset.Type)
	upstreamModel := firstNonEmpty(input.UpstreamModel, preset.DefaultModel)
	exposedAlias := firstNonEmpty(input.ExposedAlias, preset.DefaultAlias)
	routeAlias := firstNonEmpty(input.RouteAlias, preset.DefaultRoute)
	envName := firstNonEmpty(input.EnvName, EnvNameForProvider(providerID))
	apiKey := providerAPIKey(input.APIKeyMode, input.APIKey, envName)

	modelID := providerID + "-" + normalizeID(exposedAlias)
	cfg.Providers = []config.ProviderConfig{{
		ID: providerID, Name: providerName, Type: providerType, BaseURL: baseURL,
		APIKey: apiKey, Headers: cloneStringMap(preset.Headers), Enabled: true,
	}}
	cfg.Models = []config.ModelConfig{{
		ID: modelID, ProviderID: providerID, UpstreamModel: upstreamModel, ExposedAlias: exposedAlias,
		ClaudeDiscoveryAlias: preset.DiscoveryAlias, DisplayName: providerName + " " + upstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	}}
	cfg.Routes = []config.RouteConfig{{
		Alias: routeAlias, ClaudeDiscoveryAlias: preset.DiscoveryAlias, Strategy: "fallback",
		Targets: []config.RouteTarget{{ModelID: modelID, Enabled: true}}, Enabled: true,
	}}
	cfg.Profiles = map[string]string{"default": routeAlias, "best": routeAlias}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func findPreset(id string) (ProviderPreset, bool) {
	for _, preset := range Presets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return ProviderPreset{}, false
}

func providerAPIKey(mode string, raw string, envName string) string {
	if mode == APIKeyModeConfig {
		return raw
	}
	return "env:" + envName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 6: Run setup package tests**

Run:

```sh
go test -count=1 ./internal/setup
```

Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add internal/setup
git commit -m "feat: add setup provider presets"
```

## Task 3: Setup And Panel CLI Core

**Files:**
- Create: `internal/app/setup.go`
- Create: `internal/app/setup_test.go`
- Create: `internal/app/browser.go`
- Modify: `internal/app/init.go:13-40`
- Modify: `internal/cli/cli.go:19-150`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Write failing app setup tests**

Create `internal/app/setup_test.go`:

```go
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestSetupNoBrowserCreatesBootstrapConfigAndPrintsURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var out bytes.Buffer
	err := Setup(SetupOptions{ConfigPath: path, NoBrowser: true, Host: "127.0.0.1", Port: 0, ExitAfterPrint: true}, &out)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "/setup#setup_token=") {
		t.Fatalf("output missing setup token URL: %q", got)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 0 || cfg.Server.ClientKey == "" {
		t.Fatalf("bootstrap config = %+v", cfg)
	}
}

func TestPanelNoBrowserRequiresExistingConfig(t *testing.T) {
	var out bytes.Buffer
	err := Panel(PanelOptions{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), NoBrowser: true, ExitAfterPrint: true}, &out)
	if err == nil {
		t.Fatal("Panel() error = nil, want missing config error")
	}
}

func TestSetupRejectsNonLoopbackHost(t *testing.T) {
	err := Setup(SetupOptions{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), NoBrowser: true, Host: "0.0.0.0", ExitAfterPrint: true}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "host must be loopback") {
		t.Fatalf("error = %v, want loopback validation", err)
	}
}

func TestSetupDoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if _, err := InitConfig(path, false); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Setup(SetupOptions{ConfigPath: path, NoBrowser: true, Port: 0, ExitAfterPrint: true}, &out); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("Setup() overwrote existing config")
	}
}

func TestSetupOpensBrowserWhenAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var opened string
	err := Setup(SetupOptions{
		ConfigPath: path, Host: "127.0.0.1", Port: 0, ExitAfterPrint: true,
		OpenBrowser: func(url string) error {
			opened = url
			return nil
		},
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !strings.Contains(opened, "/setup#setup_token=") {
		t.Fatalf("opened URL = %q", opened)
	}
}
```

- [ ] **Step 2: Write failing CLI tests**

Add to `internal/cli/cli_test.go`:

```go
func TestRunSetupParsesNoBrowser(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "setup", "--config", configPath, "--no-browser", "--port", "0", "--exit-after-print"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "/setup#setup_token=") {
		t.Fatalf("stdout missing setup URL: %q", stdout.String())
	}
}

func TestRunPanelCommandMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "panel", "--config", filepath.Join(t.TempDir(), "missing.yaml"), "--no-browser", "--exit-after-print"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "panel failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

`--exit-after-print` is a hidden test flag so tests do not block on the temporary server lifecycle.

- [ ] **Step 3: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/app ./internal/cli -run 'TestSetup|TestPanel|TestRunSetup|TestRunPanel'
```

Expected: FAIL because `Setup`, `Panel`, and CLI dispatch do not exist.

- [ ] **Step 4: Implement app setup orchestration**

Create `internal/app/setup.go`:

```go
package app

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
	"gopkg.in/yaml.v3"
)

type SetupOptions struct {
	ConfigPath      string
	NoBrowser      bool
	Host           string
	Port           int
	ExitAfterPrint bool
	OpenBrowser    func(string) error
}

type PanelOptions struct {
	ConfigPath      string
	NoBrowser      bool
	ExitAfterPrint bool
	OpenBrowser    func(string) error
}

func Setup(options SetupOptions, w io.Writer) error {
	path := pathOrDefault(options.ConfigPath)
	host := options.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("host must be loopback")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		key, err := security.GenerateClientKey()
		if err != nil {
			return err
		}
		if err := saveConfig(path, config.BootstrapLocalConfig(key)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	port := options.Port
	if port == 0 {
		port = 20128
	}
	actualPort, err := findAvailableSetupPort(host, port)
	if err != nil {
		return err
	}
	token, err := security.GenerateClientKey()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:%d/setup#setup_token=%s", host, actualPort, token)
	fmt.Fprintf(w, "Arkroute setup panel is running:\n  %s\n\nChoose a provider, save config, then activate Claude Code from the panel.\n", url)
	if !options.NoBrowser {
		open := options.OpenBrowser
		if open == nil {
			open = openBrowserURL
		}
		if err := open(url); err != nil {
			fmt.Fprintf(w, "browser open failed: %v\n", err)
		}
	}
	if options.ExitAfterPrint {
		return nil
	}
	return runTemporaryPanelServer(path, host, actualPort, token)
}

func Panel(options PanelOptions, w io.Writer) error {
	path := pathOrDefault(options.ConfigPath)
	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	token, err := security.GenerateClientKey()
	if err != nil {
		return err
	}
	page := "panel"
	if !HasUsableProvider(cfg) {
		page = "setup"
	}
	url := fmt.Sprintf("http://%s:%d/%s#setup_token=%s", cfg.Server.Host, cfg.Server.Port, page, token)
	fmt.Fprintf(w, "Arkroute panel:\n  %s\n", url)
	if !options.NoBrowser {
		open := options.OpenBrowser
		if open == nil {
			open = openBrowserURL
		}
		if err := open(url); err != nil {
			fmt.Fprintf(w, "browser open failed: %v\n", err)
		}
	}
	if options.ExitAfterPrint {
		return nil
	}
	return nil
}

func HasUsableProvider(cfg config.Config) bool {
	for _, provider := range cfg.Providers {
		if provider.Enabled {
			return true
		}
	}
	return false
}

func saveConfig(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func findAvailableSetupPort(host string, preferred int) (int, error) {
	for port := preferred; port < preferred+20; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no available loopback setup port starting at %d", preferred)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
```

Add a temporary stub in the same file, replaced by Task 5:

```go
func runTemporaryPanelServer(path string, host string, port int, token string) error {
	return fmt.Errorf("temporary panel server not available; rerun with --no-browser")
}
```

Create `internal/app/browser.go`:

```go
package app

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openBrowserURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Update CLI dispatch**

Modify `internal/cli/cli.go`:

```go
	case "setup":
		options := app.SetupOptions{
			ConfigPath:      flagValue(args[2:], "--config"),
			NoBrowser:      hasFlag(args[2:], "--no-browser"),
			Host:           flagValue(args[2:], "--host"),
			Port:           intFlagValue(args[2:], "--port", 20128),
			ExitAfterPrint: hasFlag(args[2:], "--exit-after-print"),
		}
		if err := app.Setup(options, stdout); err != nil {
			fmt.Fprintf(stderr, "setup failed: %v\n", err)
			return 1
		}
		return 0
	case "panel":
		options := app.PanelOptions{
			ConfigPath:      flagValue(args[2:], "--config"),
			NoBrowser:      hasFlag(args[2:], "--no-browser"),
			ExitAfterPrint: hasFlag(args[2:], "--exit-after-print"),
		}
		if err := app.Panel(options, stdout); err != nil {
			fmt.Fprintf(stderr, "panel failed: %v\n", err)
			return 1
		}
		return 0
```

Add help lines:

```go
	fmt.Fprintln(w, "  setup             Open local setup panel")
	fmt.Fprintln(w, "  panel             Open local control panel")
```

- [ ] **Step 6: Run tests to verify GREEN**

Run:

```sh
go test -count=1 ./internal/app ./internal/cli -run 'TestSetup|TestPanel|TestRunSetup|TestRunPanel'
```

Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add internal/app/setup.go internal/app/setup_test.go internal/app/browser.go internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: add setup panel CLI entrypoints"
```

## Task 4: Serve Guidance For No Provider

**Files:**
- Modify: `internal/app/serve.go:22-95`
- Modify: `internal/app/commands_test.go`

- [ ] **Step 1: Write failing test for setup guidance**

Because `Serve` writes to process stdout today, first add a testable helper in `internal/app/commands_test.go`:

```go
func TestServeSetupGuidanceForNoProvider(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	got := ServeSetupGuidance(cfg)
	want := "no provider is configured\nrun: arkroute setup\n"
	if got != want {
		t.Fatalf("ServeSetupGuidance() = %q, want %q", got, want)
	}
}

func TestServeSetupGuidanceEmptyWhenProviderConfigured(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	if got := ServeSetupGuidance(cfg); got != "" {
		t.Fatalf("ServeSetupGuidance() = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/app -run TestServeSetupGuidance
```

Expected: FAIL because `ServeSetupGuidance` is undefined.

- [ ] **Step 3: Implement guidance helper and call it from serve**

Add to `internal/app/serve.go`:

```go
func ServeSetupGuidance(cfg config.Config) string {
	if HasUsableProvider(cfg) {
		return ""
	}
	return "no provider is configured\nrun: arkroute setup\n"
}
```

After line that prints `arkroute listening`:

```go
	if guidance := ServeSetupGuidance(cfg); guidance != "" {
		fmt.Print(guidance)
	}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```sh
go test -count=1 ./internal/app -run TestServeSetupGuidance
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add internal/app/serve.go internal/app/commands_test.go
git commit -m "feat: guide setup when no provider exists"
```

## Task 5: Embedded Panel And Setup Session Tokens

**Files:**
- Create: `internal/panel/session.go`
- Create: `internal/panel/session_test.go`
- Create: `internal/panel/server.go`
- Create: `internal/panel/server_test.go`
- Create: `internal/panel/assets/panel.html`
- Create: `internal/panel/assets/panel.css`
- Create: `internal/panel/assets/panel.js`
- Modify: `internal/app/setup.go`
- Modify: `internal/client/claude/server.go:22-33`
- Modify: `internal/client/claude/server_test.go`

- [ ] **Step 1: Write failing token tests**

Create `internal/panel/session_test.go`:

```go
package panel

import (
	"testing"
	"time"
)

func TestSessionStoreAcceptsIssuedTokenOnceBeforeExpiry(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	if !store.Valid(token) {
		t.Fatal("issued token should be valid")
	}
	if !store.Valid(token) {
		t.Fatal("issued token should remain valid until expiry")
	}
}

func TestSessionStoreRejectsMissingAndExpiredTokens(t *testing.T) {
	store := NewSessionStore(time.Nanosecond)
	if store.Valid("") {
		t.Fatal("empty token should be invalid")
	}
	token := store.Issue()
	time.Sleep(time.Millisecond)
	if store.Valid(token) {
		t.Fatal("expired token should be invalid")
	}
}
```

- [ ] **Step 2: Write failing panel server tests**

Create `internal/panel/server_test.go`:

```go
package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRoutesServeSetupHTML(t *testing.T) {
	handler := Routes(Deps{Sessions: NewSessionStore(time.Minute)})
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Arkroute Setup") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestSetupOptionsRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/setup/options", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSetupOptionsReturnsPresetsWithValidToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/setup/options", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "openrouter") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
```

- [ ] **Step 3: Write failing gateway session endpoint tests**

Add to `internal/client/claude/server_test.go`:

```go
func TestInternalSetupSessionRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestInternalSetupSessionIssuesTokenForPanelOptions(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		SetupToken    string `json:"setup_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 1 || payload.SetupToken == "" {
		t.Fatalf("payload = %+v", payload)
	}
	optionsReq := httptest.NewRequest(http.MethodGet, "/internal/setup/options", nil)
	optionsReq.Header.Set("X-Arkroute-Setup-Token", payload.SetupToken)
	optionsRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("options status = %d, body = %s", optionsRec.Code, optionsRec.Body.String())
	}
}
```

Add `encoding/json` to the test imports if it is not present.

- [ ] **Step 4: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/panel ./internal/client/claude -run 'TestSessionStore|TestRoutesServeSetupHTML|TestSetupOptions|TestInternalSetupSession'
```

Expected: FAIL because `internal/panel` package and `/internal/setup/session` do not exist.

- [ ] **Step 5: Implement session store**

Create `internal/panel/session.go`:

```go
package panel

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type SessionStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]time.Time
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{ttl: ttl, tokens: map[string]time.Time{}}
}

func (s *SessionStore) Issue() string {
	var raw [32]byte
	_, _ = rand.Read(raw[:])
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = time.Now().Add(s.ttl)
	return token
}

func (s *SessionStore) Valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(s.tokens, token)
		return false
	}
	return true
}
```

- [ ] **Step 6: Implement embedded panel routes**

Create `internal/panel/server.go`:

```go
package panel

import (
	"embed"
	"encoding/json"
	"net/http"
	"time"

	setupcore "github.com/bloodstalk1/arkroute/internal/setup"
)

//go:embed assets/*
var assets embed.FS

type Deps struct {
	Sessions *SessionStore
}

func Routes(deps Deps) http.Handler {
	if deps.Sessions == nil {
		deps.Sessions = NewSessionStore(15 * time.Minute)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/setup", servePanelHTML)
	mux.HandleFunc("/panel", servePanelHTML)
	mux.Handle("/panel/assets/", http.StripPrefix("/panel/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/internal/setup/options", withSetupToken(deps.Sessions, handleOptions))
	return mux
}

func servePanelHTML(w http.ResponseWriter, r *http.Request) {
	data, err := assets.ReadFile("assets/panel.html")
	if err != nil {
		http.Error(w, "panel asset missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func withSetupToken(store *SessionStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !store.Valid(r.Header.Get("X-Arkroute-Setup-Token")) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"schema_version": 1, "error": "invalid setup token"})
			return
		}
		next(w, r)
	}
}

func handleOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "presets": setupcore.Presets()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
```

Create `internal/panel/assets/panel.html`:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Arkroute Setup</title>
  <link rel="stylesheet" href="/panel/assets/panel.css">
</head>
<body>
  <main class="shell">
    <aside class="sidebar">
      <div class="brand">Arkroute</div>
      <nav>
        <a href="#setup" class="active">Setup</a>
        <a href="#system">System</a>
      </nav>
    </aside>
    <section class="content">
      <header class="page-header">
        <p class="eyebrow">local gateway</p>
        <h1>Arkroute Setup</h1>
        <p class="muted">Choose a provider, save config, and activate Claude Code.</p>
      </header>
      <section class="panel">
        <label for="provider">Provider</label>
        <select id="provider"></select>
        <button id="load-options" type="button">Load providers</button>
        <output id="status" role="status"></output>
      </section>
    </section>
  </main>
  <script src="/panel/assets/panel.js"></script>
</body>
</html>
```

Create `internal/panel/assets/panel.css`:

```css
:root {
  color-scheme: light;
  --bg: #f7f6f3;
  --surface: #ffffff;
  --text: #171717;
  --muted: #6f6f68;
  --line: #e6e2da;
  --accent: #1f6c9f;
}

* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font: 14px/1.5 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
.shell {
  min-height: 100dvh;
  display: grid;
  grid-template-columns: 220px 1fr;
}
.sidebar {
  border-right: 1px solid var(--line);
  padding: 24px;
  background: #fbfbfa;
}
.brand {
  font-weight: 700;
  margin-bottom: 32px;
}
nav a {
  display: block;
  color: var(--muted);
  text-decoration: none;
  padding: 8px 0;
}
nav a.active { color: var(--text); }
.content { padding: 40px; max-width: 980px; }
.page-header { margin-bottom: 24px; }
.eyebrow {
  margin: 0 0 8px;
  color: var(--accent);
  font: 11px/1.2 ui-monospace, SFMono-Regular, Menlo, monospace;
  text-transform: uppercase;
  letter-spacing: .08em;
}
h1 { margin: 0 0 8px; font-size: 32px; line-height: 1.1; }
.muted { color: var(--muted); margin: 0; }
.panel {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 24px;
  display: grid;
  gap: 12px;
}
label { font-weight: 600; }
select, button {
  min-height: 36px;
  border-radius: 6px;
  border: 1px solid var(--line);
  background: #fff;
  color: var(--text);
}
button {
  background: var(--text);
  color: #fff;
  cursor: pointer;
}
output {
  font: 12px/1.4 ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--muted);
}
@media (max-width: 760px) {
  .shell { grid-template-columns: 1fr; }
  .sidebar { border-right: 0; border-bottom: 1px solid var(--line); }
  .content { padding: 24px; }
}
```

Create `internal/panel/assets/panel.js`:

```js
const token = new URLSearchParams(window.location.hash.slice(1)).get("setup_token") || "";
const provider = document.querySelector("#provider");
const status = document.querySelector("#status");
const load = document.querySelector("#load-options");

load.addEventListener("click", async () => {
  status.textContent = "Loading provider presets";
  const response = await fetch("/internal/setup/options", {
    headers: {"X-Arkroute-Setup-Token": token}
  });
  if (!response.ok) {
    status.textContent = "Setup session expired. Run arkroute setup again.";
    return;
  }
  const payload = await response.json();
  provider.replaceChildren(...payload.presets.map((preset) => {
    const option = document.createElement("option");
    option.value = preset.id;
    option.textContent = preset.name;
    return option;
  }));
  status.textContent = "Provider presets loaded";
});
```

- [ ] **Step 7: Wire temporary setup server**

Modify `internal/app/setup.go` imports to include:

```go
	"net/http"
	"time"

	"github.com/bloodstalk1/arkroute/internal/panel"
```

Replace `runTemporaryPanelServer`:

```go
func runTemporaryPanelServer(path string, host string, port int, store *panel.SessionStore) error {
	handler := panel.Routes(panel.Deps{Sessions: store})
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	return http.ListenAndServe(addr, handler)
}
```

Replace the token generation block in `Setup` with:

```go
store := panel.NewSessionStore(15 * time.Minute)
token := store.Issue()
fmt.Fprintf(w, "Arkroute setup panel is running:\n  http://%s:%d/setup#setup_token=%s\n\nChoose a provider, save config, then activate Claude Code from the panel.\n", host, actualPort, token)
if options.ExitAfterPrint {
	return nil
}
return runTemporaryPanelServer(path, host, actualPort, store)
```

- [ ] **Step 8: Mount panel routes and session endpoint on gateway**

Modify `internal/client/claude/server.go`:

```go
import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bloodstalk1/arkroute/internal/panel"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)
```

Inside `Routes()` before `return mux`:

```go
	sessions := panel.NewSessionStore(15 * time.Minute)
	panelHandler := panel.Routes(panel.Deps{Sessions: sessions})
	mux.Handle("/setup", panelHandler)
	mux.Handle("/panel", panelHandler)
	mux.Handle("/panel/assets/", panelHandler)
	mux.Handle("/internal/setup/options", panelHandler)
	mux.HandleFunc("/internal/setup/session", s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, anthropicError("method_not_allowed", "method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": adminSchemaVersion,
			"setup_token":    sessions.Issue(),
		})
	}))
```

- [ ] **Step 9: Run tests to verify GREEN**

Run:

```sh
go test -count=1 ./internal/panel ./internal/app ./internal/client/claude
```

Expected: PASS.

- [ ] **Step 10: Commit**

```sh
git add internal/panel internal/app/setup.go internal/client/claude/server.go
git commit -m "feat: embed local setup panel"
```

## Task 6: Setup Mutations And Claude Activation

**Files:**
- Modify: `internal/panel/server.go`
- Modify: `internal/panel/server_test.go`
- Modify: `internal/app/setup.go`
- Modify: `internal/app/setup_test.go`

- [ ] **Step 1: Write failing setup mutation tests**

Add to `internal/panel/server_test.go`:

```go
func TestSetupLaterWritesBootstrapConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/later", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 0 || cfg.Server.ClientKey == "" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestSetupProviderSavesRedactedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	body := strings.NewReader(`{"preset_id":"openrouter","api_key_mode":"config","api_key":"sk-secret","upstream_model":"anthropic/claude-sonnet-4.5","exposed_alias":"sonnet-or","route_alias":"sonnet"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/provider", body)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("response leaked provider key: %s", rec.Body.String())
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].APIKey != "sk-secret" {
		t.Fatalf("stored key = %q", cfg.Providers[0].APIKey)
	}
}
```

Add required imports: `path/filepath`, `github.com/bloodstalk1/arkroute/internal/config`.

Add to `internal/app/setup_test.go`:

```go
func TestPanelUsesRunningGatewaySession(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/setup/session" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"setup_token":"issued-token"}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	if err := Panel(PanelOptions{ConfigPath: path, NoBrowser: true, ExitAfterPrint: true}, &out); err != nil {
		t.Fatalf("Panel() error = %v", err)
	}
	if !strings.Contains(out.String(), "#setup_token=issued-token") {
		t.Fatalf("output = %q", out.String())
	}
}
```

Add `net/http` and `net/http/httptest` to `internal/app/setup_test.go` imports if they are not present.

- [ ] **Step 2: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/panel ./internal/app -run 'TestSetupLater|TestSetupProvider|TestPanelUsesRunningGatewaySession'
```

Expected: FAIL because endpoints, `Deps.ConfigPath`, and `requestPanelSession` behavior are missing.

- [ ] **Step 3: Implement setup mutation endpoints**

Modify `internal/panel/server.go`:

```go
type Deps struct {
	Sessions   *SessionStore
	ConfigPath string
}
```

Add routes:

```go
	mux.HandleFunc("/internal/setup/provider", withSetupToken(deps.Sessions, handleProvider(deps.ConfigPath)))
	mux.HandleFunc("/internal/setup/later", withSetupToken(deps.Sessions, handleLater(deps.ConfigPath)))
```

Add handlers:

```go
func handleLater(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		key, err := security.GenerateClientKey()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg := config.BootstrapLocalConfig(key)
		if err := savePanelConfig(path, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "status": "saved", "config": config.Redacted(cfg)})
	}
}

func handleProvider(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		var input setupcore.ProviderSetup
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid setup payload"})
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg, err = setupcore.ApplyProviderSetup(cfg, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if err := savePanelConfig(path, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "status": "saved", "config": config.Redacted(cfg)})
	}
}
```

Add helpers:

```go
func loadOrBootstrapConfig(path string) (config.Config, error) {
	cfg, err := config.LoadFile(path)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, err
	}
	key, err := security.GenerateClientKey()
	if err != nil {
		return config.Config{}, err
	}
	return config.BootstrapLocalConfig(key), nil
}

func savePanelConfig(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
```

Required imports include `errors`, `os`, `path/filepath`, `github.com/bloodstalk1/arkroute/internal/config`, `github.com/bloodstalk1/arkroute/internal/security`, `gopkg.in/yaml.v3`.

- [ ] **Step 4: Update panel command to request a gateway session**

Modify `internal/app/setup.go` imports to include:

```go
	"encoding/json"
	"net/http"
	"time"
```

Add:

```go
func requestPanelSession(cfg config.Config) (string, error) {
	url := fmt.Sprintf("http://%s:%d/internal/setup/session", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("setup session failed: status %d", resp.StatusCode)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		SetupToken    string `json:"setup_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.SchemaVersion != 1 || payload.SetupToken == "" {
		return "", fmt.Errorf("setup session malformed")
	}
	return payload.SetupToken, nil
}
```

Replace the token generation in `Panel` with:

```go
token, err := requestPanelSession(cfg)
if err != nil {
	store := panel.NewSessionStore(15 * time.Minute)
	token = store.Issue()
	actualPort, portErr := findAvailableSetupPort(cfg.Server.Host, cfg.Server.Port)
	if portErr != nil {
		return portErr
	}
	page := "panel"
	if !HasUsableProvider(cfg) {
		page = "setup"
	}
	url := fmt.Sprintf("http://%s:%d/%s#setup_token=%s", cfg.Server.Host, actualPort, page, token)
	fmt.Fprintf(w, "Arkroute panel:\n  %s\n", url)
	if !options.NoBrowser {
		open := options.OpenBrowser
		if open == nil {
			open = openBrowserURL
		}
		if err := open(url); err != nil {
			fmt.Fprintf(w, "browser open failed: %v\n", err)
		}
	}
	if options.ExitAfterPrint {
		return nil
	}
	return runTemporaryPanelServer(path, cfg.Server.Host, actualPort, store)
}
```

Then keep the existing print/open flow using the token returned by `requestPanelSession` when the running gateway is reachable.

- [ ] **Step 5: Pass config path from CLI temporary server**

In `internal/app/setup.go`, pass `ConfigPath: path` when constructing `panel.Routes`.

- [ ] **Step 6: Run tests**

Run:

```sh
go test -count=1 ./internal/panel ./internal/setup ./internal/app
```

Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add internal/panel internal/app/setup.go internal/app/setup_test.go
git commit -m "feat: save setup panel config"
```

## Task 7: Safe Claude Settings Uninstall

**Files:**
- Modify: `internal/app/claude_settings.go`
- Modify: `internal/app/claude_settings_test.go`
- Create: `internal/app/uninstall.go`
- Create: `internal/app/uninstall_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Write failing Claude settings removal tests**

Add to `internal/app/claude_settings_test.go`:

```go
func TestRemoveClaudeSettingsRemovesOnlyMatchingArkrouteValues(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20128
	if err := WriteClaudeSettings(settingsPath, cfg); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveClaudeSettings(settingsPath, cfg)
	if err != nil {
		t.Fatalf("RemoveClaudeSettings() error = %v", err)
	}
	if !removed {
		t.Fatal("RemoveClaudeSettings() removed = false, want true")
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("settings still contain %s: %s", secret, data)
		}
	}
}

func TestRemoveClaudeSettingsPreservesUnrelatedAnthropicConfig(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://api.anthropic.com","ANTHROPIC_AUTH_TOKEN":"real-user-token","OTHER":"kept"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveClaudeSettings(settingsPath, config.MinimalValidConfig("local-key"))
	if err != nil {
		t.Fatalf("RemoveClaudeSettings() error = %v", err)
	}
	if removed {
		t.Fatal("RemoveClaudeSettings() removed unrelated settings")
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "real-user-token") || !strings.Contains(string(data), "OTHER") {
		t.Fatalf("settings not preserved: %s", data)
	}
}
```

- [ ] **Step 2: Write failing uninstall CLI tests**

Create `internal/app/uninstall_test.go`:

```go
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

func TestUninstallKeepsConfigByDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	settingsPath := filepath.Join(dir, "settings.json")
	cfg := config.MinimalValidConfig("local-key")
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteClaudeSettings(settingsPath, cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Uninstall(UninstallOptions{ConfigPath: configPath, SettingsPath: settingsPath, Yes: true}, &out); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config should be kept: %v", err)
	}
	if !strings.Contains(out.String(), "Local config kept") {
		t.Fatalf("output = %q", out.String())
	}
}
```

Add to `internal/cli/cli_test.go`:

```go
func TestRunUninstallMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "uninstall", "--config", filepath.Join(t.TempDir(), "missing.yaml"), "--yes"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "uninstall failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

- [ ] **Step 3: Run tests to verify RED**

Run:

```sh
go test -count=1 ./internal/app ./internal/cli -run 'RemoveClaudeSettings|Uninstall'
```

Expected: FAIL because `RemoveClaudeSettings` and `Uninstall` are undefined.

- [ ] **Step 4: Implement safe Claude settings removal**

Add to `internal/app/claude_settings.go`:

```go
func RemoveClaudeSettings(path string, cfg config.Config) (bool, error) {
	path = ClaudeSettingsPath(path)
	settings, err := readClaudeSettings(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	env := mapFromAny(settings["env"])
	if env[claudeEnvBaseURL] != claudeBaseURL(cfg) || env[claudeEnvAuthToken] != cfg.Server.ClientKey {
		return false, nil
	}
	delete(env, claudeEnvBaseURL)
	delete(env, claudeEnvAuthToken)
	delete(env, claudeEnvAPIKey)
	delete(env, claudeEnvModelDiscovery)
	settings["env"] = env
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	return true, os.WriteFile(path, data, 0o600)
}
```

- [ ] **Step 5: Implement uninstall command**

Create `internal/app/uninstall.go`:

```go
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type UninstallOptions struct {
	ConfigPath   string
	SettingsPath string
	Purge        bool
	Yes          bool
}

func Uninstall(options UninstallOptions, w io.Writer) error {
	path := pathOrDefault(options.ConfigPath)
	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	removed, err := RemoveClaudeSettings(options.SettingsPath, cfg)
	if err != nil {
		return err
	}
	if removed {
		fmt.Fprintf(w, "Claude settings integration removed: %s\n", ClaudeSettingsPath(options.SettingsPath))
	} else {
		fmt.Fprintln(w, "Claude settings were not changed; current values do not point at Arkroute")
	}
	if options.Purge {
		if !options.Yes {
			return fmt.Errorf("purge requires --yes for non-interactive use")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		logPath := DefaultLogPath()
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		removeDefaultArkrouteDirIfEmpty(path)
		fmt.Fprintln(w, "Arkroute config and logs deleted")
		return nil
	}
	fmt.Fprintf(w, "\nTo remove the npm-installed binary:\n  npm uninstall -g arkroute\n\nLocal config kept:\n  %s\n", path)
	return nil
}

func removeDefaultArkrouteDirIfEmpty(configPath string) {
	defaultPath := DefaultConfigPath()
	if filepath.Clean(configPath) != filepath.Clean(defaultPath) {
		return
	}
	_ = os.Remove(filepath.Dir(defaultPath))
}
```

- [ ] **Step 6: Add CLI dispatch**

Modify `internal/cli/cli.go`:

```go
	case "uninstall":
		options := app.UninstallOptions{
			ConfigPath:   flagValue(args[2:], "--config"),
			SettingsPath: flagValue(args[2:], "--settings"),
			Purge:        hasFlag(args[2:], "--purge"),
			Yes:          hasFlag(args[2:], "--yes"),
		}
		if err := app.Uninstall(options, stdout); err != nil {
			fmt.Fprintf(stderr, "uninstall failed: %v\n", err)
			return 1
		}
		return 0
```

Add help line:

```go
	fmt.Fprintln(w, "  uninstall         Remove Arkroute integration safely")
```

- [ ] **Step 7: Run tests**

Run:

```sh
go test -count=1 ./internal/app ./internal/cli -run 'RemoveClaudeSettings|Uninstall'
```

Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add internal/app/claude_settings.go internal/app/claude_settings_test.go internal/app/uninstall.go internal/app/uninstall_test.go internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: add safe uninstall"
```

## Task 8: NPM Platform Package Launcher

**Files:**
- Create: `npm/arkroute/package.json`
- Create: `npm/arkroute/bin/arkroute.js`
- Create: `npm/arkroute/test/launcher.test.js`
- Create: platform package `package.json` files under `npm/platform/*`
- Modify: `Makefile`

- [ ] **Step 1: Write failing launcher test**

Create `npm/arkroute/test/launcher.test.js`:

```js
import assert from "node:assert/strict";
import test from "node:test";
import { platformPackageName, binaryName } from "../bin/arkroute.js";

test("platformPackageName resolves supported packages", () => {
  assert.equal(platformPackageName("darwin", "arm64"), "@arkroute/darwin-arm64");
  assert.equal(platformPackageName("linux", "x64"), "@arkroute/linux-x64");
  assert.equal(platformPackageName("win32", "x64"), "@arkroute/win32-x64");
});

test("platformPackageName rejects unsupported targets", () => {
  assert.throws(() => platformPackageName("freebsd", "x64"), /unsupported platform/);
});

test("binaryName appends exe on windows", () => {
  assert.equal(binaryName("win32"), "arkroute.exe");
  assert.equal(binaryName("linux"), "arkroute");
});
```

- [ ] **Step 2: Run test to verify RED**

Run:

```sh
node --test npm/arkroute/test/launcher.test.js
```

Expected: FAIL because launcher does not exist.

- [ ] **Step 3: Create npm package metadata**

Create `npm/arkroute/package.json`:

```json
{
  "name": "arkroute",
  "version": "0.0.0-dev",
  "description": "Local AI model router for Claude Code and compatible provider gateways",
  "license": "MIT",
  "type": "module",
  "bin": {
    "arkroute": "./bin/arkroute.js"
  },
  "optionalDependencies": {
    "@arkroute/darwin-arm64": "0.0.0-dev",
    "@arkroute/darwin-x64": "0.0.0-dev",
    "@arkroute/linux-arm64": "0.0.0-dev",
    "@arkroute/linux-x64": "0.0.0-dev",
    "@arkroute/win32-x64": "0.0.0-dev"
  },
  "scripts": {
    "test": "node --test test/*.test.js"
  },
  "engines": {
    "node": ">=18"
  }
}
```

- [ ] **Step 4: Create JS launcher**

Create `npm/arkroute/bin/arkroute.js`:

```js
#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import path from "node:path";

const require = createRequire(import.meta.url);

export function platformPackageName(platform = process.platform, arch = process.arch) {
  const key = `${platform}-${arch}`;
  switch (key) {
    case "darwin-arm64":
      return "@arkroute/darwin-arm64";
    case "darwin-x64":
      return "@arkroute/darwin-x64";
    case "linux-arm64":
      return "@arkroute/linux-arm64";
    case "linux-x64":
      return "@arkroute/linux-x64";
    case "win32-x64":
      return "@arkroute/win32-x64";
    default:
      throw new Error(`unsupported platform ${key}`);
  }
}

export function binaryName(platform = process.platform) {
  return platform === "win32" ? "arkroute.exe" : "arkroute";
}

export function resolveBinary(platform = process.platform, arch = process.arch) {
  const packageName = platformPackageName(platform, arch);
  const packageJSON = require.resolve(`${packageName}/package.json`);
  return path.join(path.dirname(packageJSON), "bin", binaryName(platform));
}

if (import.meta.url === `file://${process.argv[1]}`) {
  let binary;
  try {
    binary = resolveBinary();
  } catch (error) {
    console.error(`Arkroute binary for ${process.platform}-${process.arch} is not installed.`);
    console.error("Try reinstalling with optional dependencies enabled:");
    console.error("  npm install -g arkroute");
    process.exit(1);
  }
  const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    console.error(result.error.message);
    process.exit(1);
  }
  process.exit(result.status ?? 0);
}
```

- [ ] **Step 5: Create platform package metadata**

For each platform directory, create package metadata. Example `npm/platform/darwin-arm64/package.json`:

```json
{
  "name": "@arkroute/darwin-arm64",
  "version": "0.0.0-dev",
  "description": "Arkroute native binary for macOS arm64",
  "license": "MIT",
  "os": ["darwin"],
  "cpu": ["arm64"],
  "files": ["bin/arkroute", "README.md"]
}
```

Use matching values for:

```text
@arkroute/darwin-x64    os darwin  cpu x64    file bin/arkroute
@arkroute/linux-arm64   os linux   cpu arm64  file bin/arkroute
@arkroute/linux-x64     os linux   cpu x64    file bin/arkroute
@arkroute/win32-x64     os win32   cpu x64    file bin/arkroute.exe
```

- [ ] **Step 6: Add Makefile targets**

Modify `Makefile`:

```make
.PHONY: test build install clean build-npm

build-npm:
	mkdir -p npm/platform/darwin-arm64/bin npm/platform/darwin-x64/bin npm/platform/linux-arm64/bin npm/platform/linux-x64/bin npm/platform/win32-x64/bin
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o npm/platform/darwin-arm64/bin/$(BINARY) ./cmd/arkroute
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o npm/platform/darwin-x64/bin/$(BINARY) ./cmd/arkroute
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o npm/platform/linux-arm64/bin/$(BINARY) ./cmd/arkroute
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o npm/platform/linux-x64/bin/$(BINARY) ./cmd/arkroute
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o npm/platform/win32-x64/bin/$(BINARY).exe ./cmd/arkroute
```

- [ ] **Step 7: Run npm launcher test**

Run:

```sh
node --test npm/arkroute/test/launcher.test.js
```

Expected: PASS.

- [ ] **Step 8: Run Go package tests**

Run:

```sh
go test -count=1 ./...
```

Expected: PASS.

- [ ] **Step 9: Commit**

```sh
git add npm Makefile
git commit -m "feat: add npm platform packages"
```

## Task 9: Documentation And Full Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-06-04-arkroute-setup-panel-npm-uninstall-design.md` only if implementation differs and the difference is intentional.

- [ ] **Step 1: Update README quick start**

Modify `README.md` install section to add:

````md
### NPM

Install Arkroute without Go:

```sh
npm install -g arkroute
arkroute setup
```

`arkroute setup` opens a local setup panel. Choose a provider, save config, and activate Claude Code from the panel.

For headless environments:

```sh
arkroute setup --no-browser
```
````

- [ ] **Step 2: Update uninstall docs**

Add:

````md
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
````

- [ ] **Step 3: Run full Go verification**

Run:

```sh
go test -count=1 ./...
```

Expected: PASS.

- [ ] **Step 4: Run npm launcher verification**

Run:

```sh
node --test npm/arkroute/test/launcher.test.js
```

Expected: PASS.

- [ ] **Step 5: Run build verification**

Run:

```sh
make build
```

Expected: PASS and `dist/arkroute` exists.

- [ ] **Step 6: Run npm build staging verification**

Run:

```sh
make build-npm
```

Expected: PASS and each `npm/platform/*/bin/arkroute*` exists.

- [ ] **Step 7: Manual smoke checks**

Run:

```sh
./dist/arkroute setup --config /tmp/arkroute-plan-config.yaml --no-browser --port 0 --exit-after-print
./dist/arkroute validate --config /tmp/arkroute-plan-config.yaml
./dist/arkroute panel --config /tmp/arkroute-plan-config.yaml --no-browser --exit-after-print
```

Expected:

- setup prints `/setup#setup_token=`
- validate prints `config ok`
- panel prints `/setup#setup_token=` because no provider is configured

- [ ] **Step 8: Commit**

```sh
git add README.md docs/superpowers/specs/2026-06-04-arkroute-setup-panel-npm-uninstall-design.md docs/superpowers/plans/2026-06-04-arkroute-setup-panel-npm-uninstall.md
git commit -m "docs: document npm setup lifecycle"
```

## Final Review Checklist

- [ ] `go test -count=1 ./...` passes.
- [ ] `node --test npm/arkroute/test/launcher.test.js` passes.
- [ ] `make build` passes.
- [ ] `make build-npm` passes.
- [ ] `arkroute setup --no-browser --exit-after-print` creates bootstrap config and prints a fragment token URL.
- [ ] Setup panel endpoints reject missing or invalid setup tokens.
- [ ] Provider setup responses never include raw provider API keys.
- [ ] `arkroute uninstall` preserves config by default.
- [ ] `arkroute uninstall` leaves unrelated Claude settings untouched.
- [ ] `arkroute uninstall --purge --yes` deletes only Arkroute config/log paths.
- [ ] README documents npm install, setup, panel, uninstall, and purge.
