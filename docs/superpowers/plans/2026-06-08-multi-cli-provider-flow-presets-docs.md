# Multi-CLI Provider Flow + Presets + Docs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Arkroute's panel workflow to provider/model-first CLI setup, add safe incremental route presets, and document a complete multi-CLI operating model with an E2E release checklist.

**Architecture:** Keep Arkroute's runtime layers separate: CLI profile chooses a route/model alias, routes resolve targets, providers select protocol, and compatibility policies handle quirks. Add a read-only CLI setup context service for selected provider/model/route aliases, add a config-mutating route preset service that uses the existing `ConfigStore` safe write path, then surface both in the panel without embedding provider-specific quirks in CLI snippets.

**Tech Stack:** Go `net/http`, Arkroute YAML config, existing `internal/setup` presets and `internal/panel` session-token routes, React in `web-ui/src/App.jsx`, existing static asset build pipeline, Markdown docs, `go test ./...`, `npm test`, `npm run build`.

---

## File Structure

- Create: `internal/clisetup/context.go`
  - Pure service for CLI setup snippets scoped to selected model/route alias and client profile.
- Create: `internal/clisetup/context_test.go`
  - Unit tests for Claude, OpenCode, Codex, Droid snippets, model alias selection, and secret redaction boundaries.
- Create: `internal/panel/cli_context.go`
  - HTTP handler for `GET /internal/cli-context?model_id=<id>&route_alias=<alias>`.
- Modify: `internal/panel/server.go`
  - Register CLI context and route preset endpoints behind setup-token auth.
- Modify: `internal/client/claude/server.go`
  - Mount new panel endpoints through the gateway-hosted panel handler.
- Test: `internal/panel/server_test.go`
  - Endpoint auth and response-shape tests.
- Test: `internal/client/claude/server_test.go`
  - Gateway mount tests for CLI context and route preset endpoints.
- Create: `internal/routepreset/preset.go`
  - Route preset definitions and incremental apply logic.
- Create: `internal/routepreset/preset_test.go`
  - Validation tests for preset metadata, generated IDs, no-overwrite behavior, and compatibility defaults.
- Create: `internal/panel/route_presets.go`
  - HTTP handlers for listing and applying route presets through `ConfigStore`.
- Modify: `web-ui/src/App.jsx`
  - Provider/model detail selection, CLI setup in detail context, and route preset application UI.
- Modify: `web-ui/src/index.css`
  - Detail layout, CLI setup command blocks, preset cards, and E2E checklist styling.
- Build output: `internal/panel/assets/panel.html` and hashed static assets
  - Updated by `npm run build`.
- Create: `docs/multi-cli-routing.md`
  - Canonical multi-CLI mental model, setup commands, route aliases, presets, policy precedence, and troubleshooting.
- Modify: `README.md`
  - Link to the canonical multi-CLI guide and keep the quick-start flow short.
- Modify: `docs/openai-compatibility.md`
  - Cross-link route aliases, Codex/OpenCode/Droid setup, and known client differences.
- Create: `docs/e2e/multi-cli-checklist.md`
  - Manual outside-sandbox E2E checklist and transcript template for release gates.

---

## Task 1: CLI Setup Context Service And Endpoints

**Files:**
- Create: `internal/clisetup/context.go`
- Create: `internal/clisetup/context_test.go`
- Create: `internal/panel/cli_context.go`
- Modify: `internal/panel/server.go`
- Modify: `internal/client/claude/server.go`
- Test: `internal/panel/server_test.go`
- Test: `internal/client/claude/server_test.go`

- [ ] **Step 1: Write failing CLI setup context tests**

Create `internal/clisetup/context_test.go`:

```go
package clisetup

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestContextForModelReturnsAllCLIProfiles(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	ctx, err := BuildContext(cfg, Request{ModelID: cfg.Models[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.SchemaVersion != 1 {
		t.Fatalf("schema = %d, want 1", ctx.SchemaVersion)
	}
	if ctx.SelectedAlias != cfg.Models[0].ExposedAlias {
		t.Fatalf("alias = %q, want %q", ctx.SelectedAlias, cfg.Models[0].ExposedAlias)
	}
	wantProfiles := []string{"claude", "opencode", "codex", "droid"}
	for _, id := range wantProfiles {
		profile, ok := findProfile(ctx.Profiles, id)
		if !ok {
			t.Fatalf("profile %q missing from %+v", id, ctx.Profiles)
		}
		if strings.Contains(profile.Command, "sk-secret") {
			t.Fatalf("profile %s leaked upstream secret: %+v", id, profile)
		}
	}
}

func TestContextForRouteUsesRouteAlias(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	ctx, err := BuildContext(cfg, Request{RouteAlias: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.SelectedAlias != "sonnet" {
		t.Fatalf("alias = %q, want sonnet", ctx.SelectedAlias)
	}
	openCode, ok := findProfile(ctx.Profiles, "opencode")
	if !ok {
		t.Fatal("opencode profile missing")
	}
	if !strings.Contains(openCode.Command, "OPENAI_MODEL='sonnet'") {
		t.Fatalf("command = %q, want selected route alias", openCode.Command)
	}
}

func TestContextRejectsUnknownSelection(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	if _, err := BuildContext(cfg, Request{ModelID: "missing"}); err == nil {
		t.Fatal("BuildContext error = nil, want missing model error")
	}
	if _, err := BuildContext(cfg, Request{RouteAlias: "missing"}); err == nil {
		t.Fatal("BuildContext route error = nil, want missing route error")
	}
}

func findProfile(profiles []Profile, id string) (Profile, bool) {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/clisetup
```

Expected: `FAIL` because package `internal/clisetup` has no implementation.

- [ ] **Step 3: Implement CLI setup context service**

Create `internal/clisetup/context.go`:

```go
package clisetup

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
)

var (
	ErrSelectionRequired = errors.New("model_id or route_alias is required")
	ErrModelNotFound    = errors.New("model not found")
	ErrRouteNotFound    = errors.New("route not found")
)

type Request struct {
	ModelID    string
	RouteAlias string
}

type Context struct {
	SchemaVersion int       `json:"schema_version"`
	SelectionType string    `json:"selection_type"`
	ModelID       string    `json:"model_id,omitempty"`
	RouteAlias    string    `json:"route_alias,omitempty"`
	ProviderID    string    `json:"provider_id,omitempty"`
	UpstreamModel string    `json:"upstream_model,omitempty"`
	SelectedAlias string    `json:"selected_alias"`
	BaseURL       string    `json:"base_url"`
	OpenAIBaseURL string    `json:"openai_base_url"`
	Profiles      []Profile `json:"profiles"`
}

type Profile struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Protocol       string `json:"protocol"`
	Command        string `json:"command"`
	ModelAlias     string `json:"model_alias"`
	ModelDiscovery bool   `json:"model_discovery"`
	LaunchSupported bool `json:"launch_supported"`
	Notes          []string `json:"notes,omitempty"`
}

func BuildContext(cfg config.Config, req Request) (Context, error) {
	selection, err := resolveSelection(cfg, req)
	if err != nil {
		return Context{}, err
	}
	baseURL := localGatewayBaseURL(cfg)
	openAIBaseURL := strings.TrimRight(baseURL, "/") + "/v1"
	return Context{
		SchemaVersion: 1,
		SelectionType: selection.kind,
		ModelID:       selection.modelID,
		RouteAlias:    selection.routeAlias,
		ProviderID:    selection.providerID,
		UpstreamModel: selection.upstreamModel,
		SelectedAlias: selection.alias,
		BaseURL:       baseURL,
		OpenAIBaseURL: openAIBaseURL,
		Profiles: []Profile{
			claudeProfile(baseURL, selection.alias),
			openAIProfile("opencode", "OpenCode", openAIBaseURL, selection.alias),
			openAIProfile("codex", "Codex", openAIBaseURL, selection.alias),
			droidProfile(openAIBaseURL, selection.alias),
		},
	}, nil
}

type selection struct {
	kind          string
	modelID       string
	routeAlias    string
	providerID     string
	upstreamModel  string
	alias          string
}

func resolveSelection(cfg config.Config, req Request) (selection, error) {
	if strings.TrimSpace(req.RouteAlias) != "" {
		for _, route := range cfg.Routes {
			if route.Alias == req.RouteAlias {
				return selection{kind: "route", routeAlias: route.Alias, alias: route.Alias}, nil
			}
		}
		return selection{}, fmt.Errorf("%w: %s", ErrRouteNotFound, req.RouteAlias)
	}
	if strings.TrimSpace(req.ModelID) != "" {
		for _, model := range cfg.Models {
			if model.ID == req.ModelID {
				alias := firstNonEmpty(model.ExposedAlias, model.ID)
				return selection{
					kind: "model", modelID: model.ID, providerID: model.ProviderID,
					upstreamModel: model.UpstreamModel, alias: alias,
				}, nil
			}
		}
		return selection{}, fmt.Errorf("%w: %s", ErrModelNotFound, req.ModelID)
	}
	return selection{}, ErrSelectionRequired
}

func claudeProfile(baseURL string, alias string) Profile {
	command := fmt.Sprintf("eval \"$(arkroute activate claude)\"\n# choose model alias in Claude Code: %s", shellQuote(alias))
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("arkroute activate claude | Invoke-Expression\nREM choose model alias in Claude Code: %s", alias)
	}
	return Profile{
		ID: "claude", Name: "Claude Code", Protocol: "anthropic",
		Command: command, ModelAlias: alias, ModelDiscovery: true, LaunchSupported: true,
		Notes: []string{"Uses ANTHROPIC_BASE_URL and gateway model discovery."},
	}
}

func openAIProfile(id string, name string, baseURL string, alias string) Profile {
	command := fmt.Sprintf("eval \"$(arkroute activate %s)\"\nexport OPENAI_MODEL=%s", id, shellQuote(alias))
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("arkroute activate %s | Invoke-Expression\nset OPENAI_MODEL=%s", id, alias)
	}
	return Profile{
		ID: id, Name: name, Protocol: "openai_compatible",
		Command: command, ModelAlias: alias, ModelDiscovery: false, LaunchSupported: false,
		Notes: []string{"Uses Arkroute's local /v1 endpoint and server.client_key."},
	}
}

func droidProfile(baseURL string, alias string) Profile {
	command := fmt.Sprintf("eval \"$(arkroute activate droid)\"\nexport ARKROUTE_OPENAI_MODEL=%s\n# droidrun run --provider OpenAILike --model \"$ARKROUTE_OPENAI_MODEL\" --api_base \"$ARKROUTE_OPENAI_BASE_URL\" \"Open the settings app\"", shellQuote(alias))
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("arkroute activate droid | Invoke-Expression\nset ARKROUTE_OPENAI_MODEL=%s\nREM droidrun run --provider OpenAILike --model \"%%ARKROUTE_OPENAI_MODEL%%\" --api_base \"%%ARKROUTE_OPENAI_BASE_URL%%\" \"Open the settings app\"", alias)
	}
	return Profile{
		ID: "droid", Name: "Droid / OpenAI-like", Protocol: "openai_compatible",
		Command: command, ModelAlias: alias, ModelDiscovery: false, LaunchSupported: false,
		Notes: []string{"DroidRun passes the OpenAI-like base URL as --api_base."},
	}
}

func localGatewayBaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

func shellQuote(value string) string {
	return security.ShellQuote(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
```

- [ ] **Step 4: Run CLI setup context tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/clisetup
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/clisetup`.

- [ ] **Step 5: Add panel endpoint tests**

Append to `internal/panel/server_test.go`:

```go
func TestCLIContextRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-context?route_alias=sonnet", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCLIContextReturnsProfilesForRoute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-secret"
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-context?route_alias=sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"selected_alias":"sonnet"`, `"id":"claude"`, `"id":"opencode"`, `"id":"codex"`, `"id":"droid"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("response leaked provider secret: %s", rec.Body.String())
	}
}
```

- [ ] **Step 6: Implement panel handler and routes**

Create `internal/panel/cli_context.go`:

```go
package panel

import (
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/clisetup"
)

func handleCLIContext(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		ctx, err := clisetup.BuildContext(cfg, clisetup.Request{
			ModelID:    r.URL.Query().Get("model_id"),
			RouteAlias: r.URL.Query().Get("route_alias"),
		})
		if err != nil {
			writeJSON(w, cliContextStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ctx)
	}
}

func cliContextStatus(err error) int {
	if errors.Is(err, clisetup.ErrModelNotFound) || errors.Is(err, clisetup.ErrRouteNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
```

In `internal/panel/server.go`, register:

```go
	mux.HandleFunc("/internal/cli-context", withSetupToken(deps.Sessions, handleCLIContext(deps.ConfigPath)))
```

In `internal/client/claude/server.go`, mount:

```go
	mux.Handle("/internal/cli-context", panelHandler)
```

- [ ] **Step 7: Run endpoint tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/clisetup ./internal/panel
```

Expected: both packages pass.

- [ ] **Step 8: Add gateway mount test**

Append to `internal/client/claude/server_test.go`:

```go
func TestGatewayMountsCLIContextEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := panelTestWriteConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Deps{ConfigPath: path})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-context?route_alias=sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"selected_alias":"sonnet"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

- [ ] **Step 9: Run gateway test and commit**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/client/claude -run TestGatewayMountsCLIContextEndpoint
git add internal/clisetup internal/panel/cli_context.go internal/panel/server.go internal/panel/server_test.go internal/client/claude/server.go internal/client/claude/server_test.go
git commit -m "feat: add cli setup context endpoint"
```

Expected: test passes and commit succeeds.

---

## Task 2: Route Preset Engine And Safe Apply Endpoints

**Files:**
- Create: `internal/routepreset/preset.go`
- Create: `internal/routepreset/preset_test.go`
- Create: `internal/panel/route_presets.go`
- Modify: `internal/panel/server.go`
- Modify: `internal/client/claude/server.go`
- Test: `internal/panel/server_test.go`
- Test: `internal/client/claude/server_test.go`

- [ ] **Step 1: Write failing route preset tests**

Create `internal/routepreset/preset_test.go`:

```go
package routepreset

import (
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestPresetsCoverRequiredFamilies(t *testing.T) {
	want := map[string]bool{
		"deepseek-v4-pro": false,
		"qwen-coder": false,
		"glm": false,
		"kimi-k2": false,
		"minimax": false,
		"claude-openrouter": false,
		"generic-openai-compatible": false,
	}
	for _, preset := range Presets() {
		if _, ok := want[preset.ID]; ok {
			want[preset.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("preset %q missing from %+v", id, Presets())
		}
	}
}

func TestApplyPresetAddsProviderModelRouteAndProfile(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	out, summary, err := Apply(cfg, ApplyRequest{
		PresetID: "deepseek-v4-pro",
		ProviderID: "deepseek",
		APIKeyMode: "env",
		EnvName: "DEEPSEEK_API_KEY",
		RouteAlias: "sonnet",
		ProfileName: "deepseek",
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.ProviderID != "deepseek" || summary.ModelID == "" || summary.RouteAlias != "sonnet" {
		t.Fatalf("summary = %+v", summary)
	}
	if len(out.Providers) != 1 || out.Providers[0].APIKey != "env:DEEPSEEK_API_KEY" {
		t.Fatalf("providers = %+v", out.Providers)
	}
	if len(out.Models) != 1 || out.Models[0].ProviderID != "deepseek" {
		t.Fatalf("models = %+v", out.Models)
	}
	if len(out.Routes) != 1 || out.Routes[0].Targets[0].ModelID != out.Models[0].ID {
		t.Fatalf("routes = %+v", out.Routes)
	}
	if out.Profiles["deepseek"] != "sonnet" {
		t.Fatalf("profiles = %+v, want deepseek -> sonnet", out.Profiles)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyPresetDoesNotOverwriteWithoutConfirmation(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	_, _, err := Apply(cfg, ApplyRequest{
		PresetID: "claude-openrouter",
		ProviderID: cfg.Providers[0].ID,
		RouteAlias: "sonnet",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}
}

func TestApplyPresetCanAppendFallbackTarget(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	out, _, err := Apply(cfg, ApplyRequest{
		PresetID: "qwen-coder",
		ProviderID: "qwen",
		APIKeyMode: "env",
		EnvName: "QWEN_API_KEY",
		RouteAlias: "sonnet",
		AppendToRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Routes) != 1 || len(out.Routes[0].Targets) != 2 {
		t.Fatalf("routes = %+v, want appended fallback target", out.Routes)
	}
	if out.Routes[0].Strategy != "fallback" {
		t.Fatalf("strategy = %q, want fallback", out.Routes[0].Strategy)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/routepreset
```

Expected: `FAIL` because package `internal/routepreset` has no implementation.

- [ ] **Step 3: Implement route preset engine**

Create `internal/routepreset/preset.go`:

```go
package routepreset

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	setupcore "github.com/bloodstalk1/arkroute/internal/setup"
)

var ErrConflict = errors.New("preset target already exists")

type Preset struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	ProviderType  string              `json:"provider_type"`
	BaseURL       string              `json:"base_url"`
	UpstreamModel string              `json:"upstream_model"`
	DefaultAlias  string              `json:"default_alias"`
	DefaultRoute  string              `json:"default_route"`
	Capabilities  config.Capabilities `json:"capabilities"`
	ReasoningReplay bool              `json:"reasoning_replay"`
	AutoThinking bool                 `json:"auto_thinking"`
}

type ApplyRequest struct {
	PresetID         string `json:"preset_id"`
	ProviderID       string `json:"provider_id"`
	ProviderName     string `json:"provider_name"`
	APIKeyMode       string `json:"api_key_mode"`
	APIKey           string `json:"api_key"`
	EnvName          string `json:"env_name"`
	RouteAlias       string `json:"route_alias"`
	ProfileName      string `json:"profile_name"`
	AppendToRoute    bool   `json:"append_to_route"`
	ConfirmOverwrite bool   `json:"confirm_overwrite"`
}

type ApplySummary struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	RouteAlias string `json:"route_alias"`
	ProfileName string `json:"profile_name,omitempty"`
	AddedProvider bool `json:"added_provider"`
	AddedModel bool `json:"added_model"`
	AppendedRouteTarget bool `json:"appended_route_target"`
}

func Presets() []Preset {
	caps := config.Capabilities{
		Streaming: true, Tools: true, ToolResults: true, SystemMessages: true,
		ContextWindow: 200000, MaxOutputTokens: 8192,
	}
	return []Preset{
		{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", ProviderType: "openai_compatible", BaseURL: "https://api.deepseek.com/v1", UpstreamModel: "deepseek-v4-pro", DefaultAlias: "deepseek-v4-pro", DefaultRoute: "sonnet", Capabilities: caps, ReasoningReplay: true, AutoThinking: true},
		{ID: "qwen-coder", Name: "Qwen Coder / Thinking", ProviderType: "openai_compatible", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", UpstreamModel: "qwen3-coder-plus", DefaultAlias: "qwen-coder", DefaultRoute: "sonnet", Capabilities: caps, ReasoningReplay: true},
		{ID: "glm", Name: "GLM", ProviderType: "openai_compatible", BaseURL: "https://open.bigmodel.cn/api/paas/v4", UpstreamModel: "glm-4.6", DefaultAlias: "glm", DefaultRoute: "sonnet", Capabilities: caps, ReasoningReplay: true},
		{ID: "kimi-k2", Name: "Kimi K2", ProviderType: "openai_compatible", BaseURL: "https://api.moonshot.ai/v1", UpstreamModel: "kimi-k2-0905-preview", DefaultAlias: "kimi-k2", DefaultRoute: "sonnet", Capabilities: caps, ReasoningReplay: true},
		{ID: "minimax", Name: "MiniMax", ProviderType: "openai_compatible", BaseURL: "https://api.minimax.io/v1", UpstreamModel: "minimax-m2", DefaultAlias: "minimax", DefaultRoute: "sonnet", Capabilities: caps},
		{ID: "claude-openrouter", Name: "Claude via OpenRouter", ProviderType: "openai_compatible", BaseURL: "https://openrouter.ai/api/v1", UpstreamModel: "anthropic/claude-sonnet-4.5", DefaultAlias: "sonnet-or", DefaultRoute: "sonnet", Capabilities: caps},
		{ID: "generic-openai-compatible", Name: "Generic OpenAI-compatible", ProviderType: "openai_compatible", BaseURL: "https://example.com/v1", UpstreamModel: "provider/model", DefaultAlias: "custom-model", DefaultRoute: "sonnet", Capabilities: caps},
	}
}

func Apply(cfg config.Config, req ApplyRequest) (config.Config, ApplySummary, error) {
	preset, ok := findPreset(req.PresetID)
	if !ok {
		return config.Config{}, ApplySummary{}, fmt.Errorf("unknown route preset %q", req.PresetID)
	}
	providerID := firstNonEmpty(req.ProviderID, preset.ID)
	providerName := firstNonEmpty(req.ProviderName, preset.Name)
	routeAlias := firstNonEmpty(req.RouteAlias, preset.DefaultRoute)
	modelAlias := preset.DefaultAlias
	modelID := normalizeID(providerID + "-" + modelAlias)
	if err := checkConflicts(cfg, providerID, modelID, req.ConfirmOverwrite); err != nil {
		return config.Config{}, ApplySummary{}, err
	}
	cfg = removeExisting(cfg, providerID, modelID, req.ConfirmOverwrite)
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{
		ID: providerID, Name: providerName, Type: preset.ProviderType, BaseURL: preset.BaseURL,
		APIKey: providerAPIKey(req, providerID), Enabled: true,
	})
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: modelID, ProviderID: providerID, UpstreamModel: preset.UpstreamModel,
		ExposedAlias: modelAlias, ClaudeDiscoveryAlias: "claude-sonnet-4-20250514",
		DisplayName: providerName + " " + preset.UpstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	})
	cfg.Routes = upsertRoute(cfg.Routes, routeAlias, modelID, req.AppendToRoute)
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]string{}
	}
	if strings.TrimSpace(req.ProfileName) != "" {
		cfg.Profiles[strings.TrimSpace(req.ProfileName)] = routeAlias
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, ApplySummary{}, err
	}
	return cfg, ApplySummary{
		ProviderID: providerID, ModelID: modelID, RouteAlias: routeAlias, ProfileName: req.ProfileName,
		AddedProvider: true, AddedModel: true, AppendedRouteTarget: req.AppendToRoute,
	}, nil
}

func findPreset(id string) (Preset, bool) {
	for _, preset := range Presets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

func checkConflicts(cfg config.Config, providerID string, modelID string, confirm bool) error {
	if confirm {
		return nil
	}
	for _, provider := range cfg.Providers {
		if provider.ID == providerID {
			return fmt.Errorf("%w: provider %s already exists", ErrConflict, providerID)
		}
	}
	for _, model := range cfg.Models {
		if model.ID == modelID {
			return fmt.Errorf("%w: model %s already exists", ErrConflict, modelID)
		}
	}
	return nil
}

func removeExisting(cfg config.Config, providerID string, modelID string, confirm bool) config.Config {
	if !confirm {
		return cfg
	}
	cfg.Providers = filterProviders(cfg.Providers, providerID)
	cfg.Models = filterModels(cfg.Models, modelID)
	for i := range cfg.Routes {
		cfg.Routes[i].Targets = filterTargets(cfg.Routes[i].Targets, modelID)
	}
	return cfg
}

func upsertRoute(routes []config.RouteConfig, alias string, modelID string, appendToRoute bool) []config.RouteConfig {
	for i := range routes {
		if routes[i].Alias == alias {
			if appendToRoute {
				routes[i].Strategy = "fallback"
				routes[i].Targets = append(routes[i].Targets, config.RouteTarget{ModelID: modelID, Enabled: true})
			} else {
				routes[i].Strategy = "fallback"
				routes[i].Targets = []config.RouteTarget{{ModelID: modelID, Enabled: true}}
			}
			routes[i].Enabled = true
			return routes
		}
	}
	return append(routes, config.RouteConfig{
		Alias: alias, ClaudeDiscoveryAlias: "claude-sonnet-4-20250514", Strategy: "fallback",
		Targets: []config.RouteTarget{{ModelID: modelID, Enabled: true}}, Enabled: true,
	})
}

func providerAPIKey(req ApplyRequest, providerID string) string {
	if req.APIKeyMode == setupcore.APIKeyModeConfig {
		return req.APIKey
	}
	return "env:" + firstNonEmpty(req.EnvName, setupcore.EnvNameForProvider(providerID))
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

func filterProviders(in []config.ProviderConfig, providerID string) []config.ProviderConfig {
	out := in[:0]
	for _, item := range in {
		if item.ID != providerID {
			out = append(out, item)
		}
	}
	return out
}

func filterModels(in []config.ModelConfig, modelID string) []config.ModelConfig {
	out := in[:0]
	for _, item := range in {
		if item.ID != modelID {
			out = append(out, item)
		}
	}
	return out
}

func filterTargets(in []config.RouteTarget, modelID string) []config.RouteTarget {
	out := in[:0]
	for _, item := range in {
		if item.ModelID != modelID {
			out = append(out, item)
		}
	}
	return out
}
```

- [ ] **Step 4: Run route preset tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/routepreset
```

Expected: package passes.

- [ ] **Step 5: Add panel route preset endpoint tests**

Append to `internal/panel/server_test.go`:

```go
func TestRoutePresetsListRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/route-presets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRoutePresetApplyWritesBackupAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := savePanelConfig(path, config.BootstrapLocalConfig("local-key")); err != nil {
		t.Fatal(err)
	}
	reloads := 0
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{
		Sessions: store, ConfigPath: path,
		OnSave: func() error { reloads++; return nil },
	})
	body := strings.NewReader(`{"preset_id":"deepseek-v4-pro","provider_id":"deepseek","api_key_mode":"env","env_name":"DEEPSEEK_API_KEY","route_alias":"sonnet","profile_name":"deepseek"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/route-presets/apply", body)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
	if !strings.Contains(rec.Body.String(), `"provider_id":"deepseek"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	updated, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Providers) != 1 || updated.Profiles["deepseek"] != "sonnet" {
		t.Fatalf("updated config = %+v", updated)
	}
}
```

- [ ] **Step 6: Implement route preset panel handlers**

Create `internal/panel/route_presets.go`:

```go
package panel

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/routepreset"
)

func handleRoutePresets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "presets": routepreset.Presets()})
	}
}

func handleRoutePresetApply(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		var input routepreset.ApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid route preset payload"})
			return
		}
		store := NewConfigStore(path)
		cfg, err := store.LoadOrBootstrap()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg, summary, err := routepreset.Apply(cfg, input)
		if err != nil {
			writeJSON(w, routePresetStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		result, err := store.Save(cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if err := reloadAfterPanelSave(onSave); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"status": "saved",
			"backup_path": result.BackupPath,
			"summary": summary,
			"config": config.Redacted(cfg),
		})
	}
}

func routePresetStatus(err error) int {
	if errors.Is(err, routepreset.ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
```

In `internal/panel/server.go`, register:

```go
	mux.HandleFunc("/internal/route-presets", withSetupToken(deps.Sessions, handleRoutePresets()))
	mux.HandleFunc("/internal/route-presets/apply", withSetupToken(deps.Sessions, handleRoutePresetApply(deps.ConfigPath, deps.OnSave)))
```

In `internal/client/claude/server.go`, mount:

```go
	mux.Handle("/internal/route-presets", panelHandler)
	mux.Handle("/internal/route-presets/apply", panelHandler)
```

- [ ] **Step 7: Run panel and route preset tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/routepreset ./internal/panel
```

Expected: both packages pass.

- [ ] **Step 8: Add gateway mount test and commit**

Append to `internal/client/claude/server_test.go`:

```go
func TestGatewayMountsRoutePresetsEndpoint(t *testing.T) {
	server := NewServer(Deps{})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodGet, "/internal/route-presets", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deepseek-v4-pro"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/client/claude -run TestGatewayMountsRoutePresetsEndpoint
git add internal/routepreset internal/panel/route_presets.go internal/panel/server.go internal/panel/server_test.go internal/client/claude/server.go internal/client/claude/server_test.go
git commit -m "feat: add route preset apply service"
```

Expected: test passes and commit succeeds.

---

## Task 3: Provider And Model Detail Flow With Contextual CLI Setup

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`
- Build output: `internal/panel/assets/panel.html` and hashed assets

- [ ] **Step 1: Add UI state and data loading helpers**

In `web-ui/src/App.jsx`, add these state values inside `App` next to existing selection state:

```jsx
  const [selectedProviderId, setSelectedProviderId] = useState("");
  const [selectedRouteAlias, setSelectedRouteAlias] = useState("");
  const [cliContext, setCliContext] = useState(null);
  const [cliContextStatus, setCliContextStatus] = useState({ text: "", type: "" });
  const [routePresets, setRoutePresets] = useState([]);
  const [routePresetStatus, setRoutePresetStatus] = useState({ text: "", type: "" });
```

Add provider and route selection effects after the existing model selection effect:

```jsx
  useEffect(() => {
    const providers = config?.providers || [];
    if (providers.length === 0) {
      setSelectedProviderId("");
      return;
    }
    if (!selectedProviderId || !providers.some((provider) => provider.id === selectedProviderId)) {
      setSelectedProviderId(providers[0].id);
    }
  }, [config, selectedProviderId]);

  useEffect(() => {
    const routes = config?.routes || [];
    if (routes.length === 0) {
      setSelectedRouteAlias("");
      return;
    }
    if (!selectedRouteAlias || !routes.some((route) => route.alias === selectedRouteAlias)) {
      setSelectedRouteAlias(routes[0].alias);
    }
  }, [config, selectedRouteAlias]);
```

Add fetch helpers:

```jsx
  const loadRoutePresets = useCallback((cancelled = () => false) => {
    return fetch("/internal/route-presets", { headers: apiHeaders })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((payload) => {
        if (!cancelled()) {
          setRoutePresets(payload.presets || []);
        }
      })
      .catch((err) => {
        if (!cancelled()) {
          setRoutePresetStatus({ text: err.message, type: "error" });
        }
      });
  }, [apiHeaders]);

  const loadCLIContext = useCallback((selection, cancelled = () => false) => {
    const params = new URLSearchParams();
    if (selection.route_alias) params.set("route_alias", selection.route_alias);
    if (selection.model_id) params.set("model_id", selection.model_id);
    if (!params.toString()) return Promise.resolve();
    setCliContextStatus({ text: "", type: "" });
    return fetch(`/internal/cli-context?${params.toString()}`, { headers: apiHeaders })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((payload) => {
        if (!cancelled()) {
          setCliContext(payload);
        }
      })
      .catch((err) => {
        if (!cancelled()) {
          setCliContext(null);
          setCliContextStatus({ text: err.message, type: "error" });
        }
      });
  }, [apiHeaders]);
```

Update the active-tab effect to load route presets and CLI context when needed:

```jsx
    if (activeTab === "models") {
      loadRoutePresets(isCancelled);
      if (selectedRouteAlias) {
        loadCLIContext({ route_alias: selectedRouteAlias }, isCancelled);
      } else if (selectedModelId) {
        loadCLIContext({ model_id: selectedModelId }, isCancelled);
      }
    }
```

- [ ] **Step 2: Add detail components**

Add these components above `App`:

```jsx
function ProviderDetail({ provider, models, routes, onSelectModel, onSelectRoute }) {
  if (!provider) {
    return <EmptyState icon="ph-hard-drive" title="No provider selected">Choose a configured provider.</EmptyState>;
  }
  const providerModels = models.filter((model) => model.provider_id === provider.id);
  return (
    <section className="operator-card detail-panel">
      <div className="card-heading">
        <div>
          <StatusBadge tone={provider.enabled ? "ok" : "pending"}>{provider.type || "auto"}</StatusBadge>
          <h3><i className="ph-light ph-hard-drive"></i>{provider.name || provider.id}</h3>
        </div>
      </div>
      <div className="policy-summary-grid">
        <DataRow label="Provider ID">{provider.id}</DataRow>
        <DataRow label="Base URL">{provider.base_url}</DataRow>
        <DataRow label="Models">{providerModels.length}</DataRow>
      </div>
      <div className="context-list">
        {providerModels.map((model) => (
          <button type="button" key={model.id} onClick={() => onSelectModel(model.id)}>
            <span>{model.exposed_alias || model.id}</span>
            <code>{model.upstream_model}</code>
          </button>
        ))}
      </div>
      <div className="context-list">
        {routes.map((route) => (
          <button type="button" key={route.alias} onClick={() => onSelectRoute(route.alias)}>
            <span>{route.alias}</span>
            <code>{route.strategy}</code>
          </button>
        ))}
      </div>
    </section>
  );
}

function CLIContextPanel({ context, status, onCopy }) {
  if (status.text) {
    return <div className={`status-box ${status.type}`}>{status.text}</div>;
  }
  if (!context) {
    return <EmptyState icon="ph-terminal-window" title="No CLI context">Select a model or route.</EmptyState>;
  }
  return (
    <section className="operator-card cli-context-card">
      <div className="card-heading">
        <div>
          <StatusBadge tone="ok">{context.selected_alias}</StatusBadge>
          <h3><i className="ph-light ph-terminal-window"></i>CLI Setup</h3>
        </div>
      </div>
      <div className="cli-context-grid">
        {(context.profiles || []).map((profile) => (
          <article className="cli-context-profile" key={profile.id}>
            <div className="cli-context-title">
              <strong>{profile.name}</strong>
              <code>{profile.protocol}</code>
            </div>
            <pre>{profile.command}</pre>
            <button type="button" className="btn-secondary" onClick={() => onCopy(profile.command)}>
              <i className="ph-bold ph-copy"></i>Copy
            </button>
          </article>
        ))}
      </div>
    </section>
  );
}

function RoutePresetPanel({ presets, status, onApply }) {
  return (
    <section className="operator-card route-presets-card">
      <div className="card-heading">
        <div>
          <StatusBadge tone={presets.length > 0 ? "ok" : "pending"}>{presets.length || "loading"}</StatusBadge>
          <h3><i className="ph-light ph-stack-plus"></i>Route Presets</h3>
        </div>
      </div>
      <div className="preset-grid">
        {presets.map((preset) => (
          <button type="button" className="route-preset-card" key={preset.id} onClick={() => onApply(preset)}>
            <span>{preset.name}</span>
            <code>{preset.default_alias} -> {preset.upstream_model}</code>
          </button>
        ))}
      </div>
      {status.text && <div className={`status-box ${status.type}`}>{status.text}</div>}
    </section>
  );
}
```

- [ ] **Step 3: Add route preset apply and copy actions**

Inside `App`, add:

```jsx
  const copyCLICommand = async (command) => {
    try {
      await navigator.clipboard.writeText(command);
      setCliContextStatus({ text: "Command copied.", type: "ok" });
    } catch {
      setCliContextStatus({ text: command, type: "info" });
    }
  };

  const applyRoutePreset = async (preset) => {
    setRoutePresetStatus({ text: "Applying route preset...", type: "" });
    const response = await fetch("/internal/route-presets/apply", {
      method: "POST",
      headers: apiHeaders,
      body: JSON.stringify({
        preset_id: preset.id,
        provider_id: preset.id,
        api_key_mode: "env",
        route_alias: preset.default_route,
        profile_name: preset.id,
        append_to_route: true
      })
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) {
      setRoutePresetStatus({ text: result.error || "Preset apply failed", type: "error" });
      return;
    }
    setConfig(result.config);
    setRoutePresetStatus({ text: `Preset applied: ${result.summary?.model_id || preset.id}`, type: "ok" });
    loadStatus();
  };
```

- [ ] **Step 4: Render detail flow in Providers and Routes tabs**

In the Providers tab, make `ProviderCard` clickable by changing its usage to:

```jsx
config.providers.map((provider) => (
  <button className="provider-card-button" type="button" key={provider.id} onClick={() => setSelectedProviderId(provider.id)}>
    <ProviderCard provider={provider} />
  </button>
))
```

Add below configured providers:

```jsx
<div className="detail-workbench">
  <ProviderDetail
    provider={(config?.providers || []).find((provider) => provider.id === selectedProviderId)}
    models={config?.models || []}
    routes={config?.routes || []}
    onSelectModel={(modelId) => {
      setSelectedModelId(modelId);
      loadCLIContext({ model_id: modelId });
      setActiveTab("models");
    }}
    onSelectRoute={(routeAlias) => {
      setSelectedRouteAlias(routeAlias);
      loadCLIContext({ route_alias: routeAlias });
      setActiveTab("models");
    }}
  />
  <CLIContextPanel context={cliContext} status={cliContextStatus} onCopy={copyCLICommand} />
</div>
```

In the Routes tab, render `RoutePresetPanel` and `CLIContextPanel` next to `PolicyInspector`:

```jsx
<RoutePresetPanel presets={routePresets} status={routePresetStatus} onApply={applyRoutePreset} />
<CLIContextPanel context={cliContext} status={cliContextStatus} onCopy={copyCLICommand} />
```

Update `RouteItem` calls so clicking a route also sets route context:

```jsx
<RouteItem
  key={route.alias}
  route={route}
  selectedModelId={selectedModelId}
  onSelectModel={(modelId) => {
    setSelectedModelId(modelId);
    loadCLIContext({ model_id: modelId });
  }}
/>
```

- [ ] **Step 5: Add CSS for detail panels**

Append to `web-ui/src/index.css`:

```css
.detail-workbench {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 0.9fr);
  gap: 16px;
  margin-top: 18px;
}

.provider-card-button {
  appearance: none;
  border: 0;
  padding: 0;
  text-align: left;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.provider-card-button > .operator-card {
  height: 100%;
}

.detail-panel,
.cli-context-card,
.route-presets-card {
  gap: 16px;
}

.context-list {
  display: grid;
  gap: 8px;
}

.context-list button,
.route-preset-card {
  display: grid;
  gap: 4px;
  width: 100%;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-soft);
  color: var(--text);
  padding: 10px 12px;
  text-align: left;
}

.context-list code,
.route-preset-card code,
.cli-context-title code {
  color: var(--muted);
  font-size: 12px;
}

.cli-context-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.cli-context-profile {
  display: grid;
  gap: 10px;
  min-width: 0;
}

.cli-context-profile pre {
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-soft);
  padding: 10px;
  font-size: 12px;
  line-height: 1.45;
}

.cli-context-title {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

@media (max-width: 980px) {
  .detail-workbench,
  .cli-context-grid,
  .preset-grid {
    grid-template-columns: 1fr;
  }
}
```

- [ ] **Step 6: Build frontend and commit**

Run:

```bash
npm run build
git add web-ui/src/App.jsx web-ui/src/index.css internal/panel/assets
git commit -m "feat: add provider model cli context flow"
```

Expected: build passes and commit succeeds.

---

## Task 4: Multi-CLI Docs And E2E Checklist

**Files:**
- Create: `docs/multi-cli-routing.md`
- Create: `docs/e2e/multi-cli-checklist.md`
- Modify: `README.md`
- Modify: `docs/openai-compatibility.md`

- [ ] **Step 1: Add canonical multi-CLI guide**

Create `docs/multi-cli-routing.md`:

```markdown
# Multi-CLI Routing With Arkroute

Arkroute keeps four layers separate:

```text
CLI profile -> route/model alias -> provider/protocol resolver -> compatibility policy
```

CLI profiles only point clients at Arkroute and choose a route or exposed model alias. Provider-specific quirks belong in compatibility policies, not in CLI snippets.

## Activation Commands

```sh
eval "$(arkroute activate claude)"
eval "$(arkroute activate opencode)"
eval "$(arkroute activate codex)"
eval "$(arkroute activate droid)"
```

Claude Code uses Arkroute's Anthropic-compatible endpoint and gateway model discovery. OpenCode, Codex, Droid, and OpenAI-like clients use Arkroute's `/v1` endpoint with `server.client_key`.

## Choosing Aliases

Use route aliases for shared workflows and exposed model aliases for direct targeting.

```yaml
routes:
  - alias: sonnet
    strategy: fallback
    targets:
      - model_id: deepseek-deepseek-v4-pro
        enabled: true
      - model_id: qwen-qwen-coder
        enabled: true
      - model_id: openrouter-sonnet-or
        enabled: true
```

The same alias can be used by Claude Code, OpenCode, Codex, Droid, and SDK clients.

## Compatibility Policy Precedence

Reasoning compatibility is resolved in this order:

1. Model-level `models[].reasoning`
2. User `compatibility_policies`
3. Builtin compatibility policies
4. Capability defaults

Example override:

```yaml
compatibility_policies:
  - id: model-deepseek-v4-pro-compat
    match:
      provider_ids: [deepseek]
      upstream_models: [deepseek-v4-pro]
    reasoning:
      auto_enable: false
      replay: false
      omit_tool_choice: false
```

## DeepSeek V4 Pro Troubleshooting

If Claude Code fails on a DeepSeek V4 Pro route, inspect the model in the Routes panel. Check `auto_enable`, `auto_effort`, `replay`, and `omit_tool_choice`. Disable the generated user override only when the upstream provider behaves correctly with Claude-style reasoning controls.
```

- [ ] **Step 2: Add E2E checklist**

Create `docs/e2e/multi-cli-checklist.md`:

```markdown
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
```

- [ ] **Step 3: Update README and OpenAI compatibility doc**

In `README.md`, add near the activation section:

```markdown
For the full multi-CLI mental model, route alias examples, policy precedence, and E2E checklist, see [Multi-CLI Routing With Arkroute](docs/multi-cli-routing.md).
```

In `docs/openai-compatibility.md`, add near the OpenCode/Codex/Droid section:

```markdown
For provider/model-first setup and route presets, use the panel's Providers and Routes tabs. CLI snippets should only select a route or exposed model alias; provider quirks belong in compatibility policies. See [Multi-CLI Routing With Arkroute](multi-cli-routing.md).
```

- [ ] **Step 4: Verify doc anchors and commit**

Run:

```bash
rg -n "Multi-CLI Routing|arkroute activate (claude|opencode|codex|droid)|compatibility_policies|provider_ids|upstream_models" README.md docs/multi-cli-routing.md docs/openai-compatibility.md docs/e2e/multi-cli-checklist.md
git add README.md docs/openai-compatibility.md docs/multi-cli-routing.md docs/e2e/multi-cli-checklist.md
git commit -m "docs: add multi-cli routing guide"
```

Expected: `rg` prints entries from all four docs and commit succeeds.

---

## Task 5: Regression Tests And Final E2E Gate

**Files:**
- Test: `internal/client/claude/server_test.go`
- Test: `internal/client/openai/*_test.go`
- Test: `internal/setup/*_test.go`
- Test: `internal/routepreset/preset_test.go`
- Docs: `docs/e2e/multi-cli-checklist.md`

- [ ] **Step 1: Add model discovery regression test**

Add to `internal/client/claude/server_test.go`:

```go
func TestModelsIncludesRouteAndExposedAliases(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	path := writeClaudeServerConfig(t, cfg)
	state := testStateFromPath(t, path, cfg.Server.Host, cfg.Server.Port)
	server := NewServer(Deps{State: state})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"id":"sonnet"`, `"id":"sonnet-or"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Add OpenAI-compatible fixture regression**

Add to `internal/client/openai/fixtures_test.go`:

```go
func TestChatCompletionsAcceptsCodexAndDroidMetadataShapes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fixture","choices":[{"message":{"role":"assistant","content":"fixture ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	handler := testServerWithConfig(t, cfg).Routes()

	for _, body := range []string{
		`{"model":"sonnet","messages":[{"role":"user","content":"hello"}],"metadata":{"client":"codex-cli"}}`,
		`{"model":"sonnet","messages":[{"role":"system","content":"Control Android."},{"role":"user","content":"Open settings"}],"user":"droidrun-local"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer local-key")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}
}
```

- [ ] **Step 3: Run full automated verification**

Run outside sandbox when loopback ports or cache writes are restricted:

```bash
git diff --check
git diff --cached --check
npm test
npm run build
go vet ./...
```

Expected:

- Both diff checks produce no output.
- `npm test` reports all Go packages and npm package tests passing.
- `npm run build` compiles frontend assets and `dist/arkroute`.
- `go vet ./...` produces no output.

- [ ] **Step 4: Complete manual E2E checklist**

Run the commands in `docs/e2e/multi-cli-checklist.md` outside sandbox. Fill the Transcript section with one successful run that covers:

- `arkroute serve`
- `eval "$(arkroute activate claude)"`
- a Claude Code request through the selected route
- `eval "$(arkroute activate opencode)"` or `eval "$(arkroute activate codex)"`
- one OpenAI-compatible request through the same route alias
- Routes panel policy inspector showing the selected model

- [ ] **Step 5: Commit final verification docs**

Run:

```bash
git add docs/e2e/multi-cli-checklist.md internal/client/claude/server_test.go internal/client/openai
git commit -m "test: add multi-cli e2e checklist"
```

Expected: commit succeeds when files changed. If no test files changed because equivalent tests already existed, commit only the filled E2E checklist.

---

## Acceptance Mapping

- Provider/model detail flow: Task 3 adds provider detail, model/route context selection, and detail workbench.
- CLI setup in provider/model context: Task 1 adds backend context snippets; Task 3 renders and copies them in Providers and Routes tabs.
- Route presets: Task 2 adds route preset definitions, safe incremental apply, no-overwrite protection, backup/reload path, and panel endpoints.
- Multi-CLI docs: Task 4 creates the canonical guide and links README/OpenAI docs.
- E2E checklist: Task 4 creates the checklist; Task 5 requires a transcript before release.
- No upstream key leakage: Task 1 tests CLI context response does not include provider API keys.
- CLI profiles stay quirk-free: Tasks 1 and 3 only choose aliases; compatibility policy behavior remains in inspector/policy editor.

## Execution Choice

Plan complete and saved to `docs/superpowers/plans/2026-06-08-multi-cli-provider-flow-presets-docs.md`. Two execution options:

**1. Subagent-Driven (recommended)** - dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** - execute tasks in this session using executing-plans, batch execution with checkpoints.
