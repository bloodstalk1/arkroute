# Panel CLI Tools Launch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a panel CLI Tools module that can preflight Claude Code routing through Arkroute, copy the activation command, and launch Claude Code only when the Arkroute process has a usable interactive terminal.

**Architecture:** Put launch/status logic in a new `internal/clitools` package so `internal/panel` can use it without importing `internal/app` and creating a cycle. Panel endpoints stay setup-token protected and are mounted through both `panel.Routes` and `client/claude.Server.Routes`. The React panel consumes those endpoints and enables `Launch` only when the backend reports `launch_supported`.

**Tech Stack:** Go 1.23 standard library HTTP/process APIs, existing Arkroute config/panel/server packages, React/Vite panel, existing `make build-frontend` asset embedding.

---

## File Structure

- Create `internal/clitools/service.go`: status, env sanitization, gateway preflight, interactive launch gating, and Claude process start.
- Create `internal/clitools/service_test.go`: focused unit tests for env, status, preflight, launch success, and blocked launch.
- Create `internal/panel/cli_tools.go`: setup-token protected HTTP handlers for CLI Tools status and Claude launch.
- Modify `internal/panel/server.go`: add `CLITools` dependency and route registrations.
- Modify `internal/panel/server_test.go`: test token protection and handler JSON behavior.
- Modify `internal/client/claude/server.go`: create the gateway-hosted CLI Tools service and mount the two `/internal/cli-tools` paths.
- Modify `internal/client/claude/server_test.go`: prove CLI Tools routes are reachable through the gateway only with a setup session token.
- Modify `internal/app/setup.go`: create a temporary-panel CLI Tools service with launch disabled.
- Modify `web-ui/src/App.jsx`: add CLI Tools nav item, fetch status, render Claude Code row, copy activation command, and POST launch.
- Modify `web-ui/src/index.css`: add small styles for the CLI Tools row, command block, and disabled launch state.

Do not manually edit `internal/panel/assets/*`. Run `npm run build:frontend` after frontend changes so generated assets are copied by the existing Makefile.

---

### Task 1: Core CLI Tools Service

**Files:**
- Create: `internal/clitools/service.go`
- Create: `internal/clitools/service_test.go`

- [ ] **Step 1: Write failing env and status tests**

Create `internal/clitools/service_test.go` with these tests:

```go
package clitools

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

func testConfig() config.Config {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 20128
	return cfg
}

func TestClaudeLaunchEnvRemovesStaleAnthropicValues(t *testing.T) {
	cfg := testConfig()
	got := ClaudeLaunchEnv([]string{
		"PATH=/bin",
		"ANTHROPIC_BASE_URL=https://api.anthropic.com",
		"ANTHROPIC_AUTH_TOKEN=old",
		"ANTHROPIC_API_KEY=old",
		"OTHER=value",
	}, cfg)
	joined := "\n" + strings.Join(got, "\n") + "\n"
	for _, forbidden := range []string{
		"https://api.anthropic.com",
		"ANTHROPIC_AUTH_TOKEN=old",
		"ANTHROPIC_API_KEY=old",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("env contains stale value %q: %v", forbidden, got)
		}
	}
	for _, want := range []string{
		"\nPATH=/bin\n",
		"\nOTHER=value\n",
		"\nANTHROPIC_BASE_URL=http://127.0.0.1:20128\n",
		"\nANTHROPIC_AUTH_TOKEN=local-key\n",
		"\nANTHROPIC_API_KEY=local-key\n",
		"\nCLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1\n",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("env missing %q: %v", want, got)
		}
	}
}

func TestStatusReportsReadyWhenEverythingIsAvailable(t *testing.T) {
	path := writeConfig(t, testConfig())
	svc := Service{
		ConfigPath:              path,
		GatewayHosted:           true,
		LookupPath:              func(string) (string, error) { return "/usr/local/bin/claude", nil },
		GatewayReachable:        func(config.Config) bool { return true },
		HasInteractiveTerminal:  func() bool { return true },
		ActivationCommandBuilder: func(config.Config) string { return `eval "$(arkroute activate claude)"` },
	}
	resp, err := svc.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("tools len = %d", len(resp.Tools))
	}
	tool := resp.Tools[0]
	if !tool.Installed || !tool.GatewayReachable || !tool.LaunchSupported {
		t.Fatalf("tool not ready: %+v", tool)
	}
	if tool.BaseURL != "http://127.0.0.1:20128" {
		t.Fatalf("BaseURL = %q", tool.BaseURL)
	}
	if tool.ActivationCommand == "" {
		t.Fatalf("ActivationCommand empty: %+v", tool)
	}
}

func TestStatusBlocksLaunchWithoutTerminal(t *testing.T) {
	path := writeConfig(t, testConfig())
	svc := Service{
		ConfigPath:             path,
		GatewayHosted:          true,
		LookupPath:             func(string) (string, error) { return "/usr/local/bin/claude", nil },
		GatewayReachable:       func(config.Config) bool { return true },
		HasInteractiveTerminal: func() bool { return false },
	}
	resp, err := svc.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	tool := resp.Tools[0]
	if tool.LaunchSupported {
		t.Fatalf("LaunchSupported = true, want false: %+v", tool)
	}
	if tool.LaunchBlockedReason != "interactive terminal unavailable" {
		t.Fatalf("LaunchBlockedReason = %q", tool.LaunchBlockedReason)
	}
}

func TestStatusReportsMissingClaudeBinary(t *testing.T) {
	path := writeConfig(t, testConfig())
	svc := Service{
		ConfigPath:       path,
		GatewayHosted:    true,
		LookupPath:       func(string) (string, error) { return "", errors.New("not found") },
		GatewayReachable: func(config.Config) bool { return true },
	}
	resp, err := svc.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	tool := resp.Tools[0]
	if tool.Installed {
		t.Fatalf("Installed = true, want false: %+v", tool)
	}
	if tool.LaunchSupported {
		t.Fatalf("LaunchSupported = true, want false: %+v", tool)
	}
}

func TestLaunchClaudeStartsProcessWhenSupported(t *testing.T) {
	path := writeConfig(t, testConfig())
	started := false
	var startedSpec ProcessSpec
	svc := Service{
		ConfigPath:             path,
		GatewayHosted:          true,
		LookupPath:             func(string) (string, error) { return "/usr/local/bin/claude", nil },
		GatewayReachable:       func(config.Config) bool { return true },
		HasInteractiveTerminal: func() bool { return true },
		Environ:                func() []string { return []string{"PATH=/bin", "ANTHROPIC_BASE_URL=stale"} },
		StartProcess: func(spec ProcessSpec) (int, error) {
			started = true
			startedSpec = spec
			return 4242, nil
		},
	}
	resp, err := svc.LaunchClaude()
	if err != nil {
		t.Fatalf("LaunchClaude() error = %v", err)
	}
	if !started {
		t.Fatal("StartProcess was not called")
	}
	if resp.PID != 4242 || !resp.Launched || resp.Command != "claude" {
		t.Fatalf("launch response = %+v", resp)
	}
	if startedSpec.Command != "/usr/local/bin/claude" {
		t.Fatalf("started command = %q", startedSpec.Command)
	}
	joined := strings.Join(startedSpec.Env, "\n")
	if strings.Contains(joined, "ANTHROPIC_BASE_URL=stale") {
		t.Fatalf("stale env was passed to child: %v", startedSpec.Env)
	}
	if !strings.Contains(joined, "ANTHROPIC_BASE_URL=http://127.0.0.1:20128") {
		t.Fatalf("child env missing base URL: %v", startedSpec.Env)
	}
}

func TestLaunchClaudeDoesNotStartWithoutTerminal(t *testing.T) {
	path := writeConfig(t, testConfig())
	started := false
	svc := Service{
		ConfigPath:             path,
		GatewayHosted:          true,
		LookupPath:             func(string) (string, error) { return "/usr/local/bin/claude", nil },
		GatewayReachable:       func(config.Config) bool { return true },
		HasInteractiveTerminal: func() bool { return false },
		StartProcess: func(ProcessSpec) (int, error) {
			started = true
			return 0, nil
		},
	}
	_, err := svc.LaunchClaude()
	if err == nil {
		t.Fatal("LaunchClaude() error = nil, want launch_unavailable")
	}
	if started {
		t.Fatal("StartProcess was called despite missing terminal")
	}
	cliErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if cliErr.Code != "launch_unavailable" {
		t.Fatalf("error code = %q", cliErr.Code)
	}
}

func writeConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
```

- [ ] **Step 2: Run tests and verify they fail because the package is missing**

Run:

```sh
go test -count=1 ./internal/clitools
```

Expected: FAIL with missing package/types such as `undefined: Service` and `undefined: ClaudeLaunchEnv`.

- [ ] **Step 3: Implement `internal/clitools/service.go`**

Create `internal/clitools/service.go`:

```go
package clitools

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
)

const schemaVersion = 1

type ToolStatus struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Command             string `json:"command"`
	Installed           bool   `json:"installed"`
	GatewayReachable    bool   `json:"gateway_reachable"`
	BaseURL             string `json:"base_url"`
	ModelDiscovery      bool   `json:"model_discovery"`
	LaunchSupported     bool   `json:"launch_supported"`
	LaunchBlockedReason string `json:"launch_blocked_reason"`
	ActivationCommand   string `json:"activation_command"`
}

type ListResponse struct {
	SchemaVersion int          `json:"schema_version"`
	Tools         []ToolStatus `json:"tools"`
}

type LaunchResponse struct {
	SchemaVersion int    `json:"schema_version"`
	Launched      bool   `json:"launched"`
	PID           int    `json:"pid"`
	Command       string `json:"command"`
}

type ProcessSpec struct {
	Command string
	Env     []string
}

type Service struct {
	ConfigPath               string
	GatewayHosted            bool
	LookupPath               func(string) (string, error)
	GatewayReachable         func(config.Config) bool
	HasInteractiveTerminal   func() bool
	StartProcess             func(ProcessSpec) (int, error)
	Environ                  func() []string
	ActivationCommandBuilder func(config.Config) string
}

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"error"`
	Remediation string `json:"remediation"`
	Status      int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

func NewService(configPath string, gatewayHosted bool) *Service {
	return &Service{ConfigPath: configPath, GatewayHosted: gatewayHosted}
}

func (s *Service) Status() (ListResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return ListResponse{}, err
	}
	_, lookupErr := s.lookupPath()("claude")
	installed := lookupErr == nil
	gatewayReachable := s.gatewayReachable()(cfg)
	launchSupported, blocked := s.launchSupport(installed, gatewayReachable)
	return ListResponse{
		SchemaVersion: schemaVersion,
		Tools: []ToolStatus{{
			ID:                  "claude",
			Name:                "Claude Code",
			Command:             "claude",
			Installed:           installed,
			GatewayReachable:    gatewayReachable,
			BaseURL:             BaseURL(cfg),
			ModelDiscovery:      true,
			LaunchSupported:     launchSupported,
			LaunchBlockedReason: blocked,
			ActivationCommand:   s.activationCommand()(cfg),
		}},
	}, nil
}

func (s *Service) LaunchClaude() (LaunchResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return LaunchResponse{}, err
	}
	path, err := s.lookupPath()("claude")
	if err != nil || path == "" {
		return LaunchResponse{}, &Error{Code: "missing_binary", Message: "Claude Code binary not found", Remediation: "Install Claude Code, then reopen CLI Tools.", Status: http.StatusBadRequest}
	}
	if !s.gatewayReachable()(cfg) {
		return LaunchResponse{}, &Error{Code: "gateway_unreachable", Message: "Arkroute gateway is not reachable", Remediation: "Start arkroute serve, then retry.", Status: http.StatusBadRequest}
	}
	launchSupported, _ := s.launchSupport(true, true)
	if !launchSupported {
		return LaunchResponse{}, &Error{Code: "launch_unavailable", Message: "Interactive launch is unavailable", Remediation: "Copy the activation command and run Claude Code in a terminal.", Status: http.StatusBadRequest}
	}
	pid, err := s.startProcess()(ProcessSpec{Command: path, Env: ClaudeLaunchEnv(s.environ(), cfg)})
	if err != nil {
		return LaunchResponse{}, &Error{Code: "spawn_failed", Message: "Failed to launch Claude Code", Remediation: "Copy the activation command and run Claude Code in a terminal.", Status: http.StatusInternalServerError}
	}
	return LaunchResponse{SchemaVersion: schemaVersion, Launched: true, PID: pid, Command: "claude"}, nil
}

func (s *Service) loadConfig() (config.Config, error) {
	path := s.ConfigPath
	if path == "" {
		path = defaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		return config.Config{}, &Error{Code: "config_error", Message: err.Error(), Remediation: "Run arkroute setup.", Status: http.StatusBadRequest}
	}
	if err := validateConfig(cfg); err != nil {
		return config.Config{}, &Error{Code: "config_error", Message: err.Error(), Remediation: "Run arkroute setup.", Status: http.StatusBadRequest}
	}
	return cfg, nil
}

func (s *Service) lookupPath() func(string) (string, error) {
	if s.LookupPath != nil {
		return s.LookupPath
	}
	return exec.LookPath
}

func (s *Service) gatewayReachable() func(config.Config) bool {
	if s.GatewayReachable != nil {
		return s.GatewayReachable
	}
	return DefaultGatewayReachable
}

func (s *Service) hasInteractiveTerminal() func() bool {
	if s.HasInteractiveTerminal != nil {
		return s.HasInteractiveTerminal
	}
	return DefaultHasInteractiveTerminal
}

func (s *Service) startProcess() func(ProcessSpec) (int, error) {
	if s.StartProcess != nil {
		return s.StartProcess
	}
	return DefaultStartProcess
}

func (s *Service) environ() []string {
	if s.Environ != nil {
		return s.Environ()
	}
	return os.Environ()
}

func (s *Service) activationCommand() func(config.Config) string {
	if s.ActivationCommandBuilder != nil {
		return s.ActivationCommandBuilder
	}
	return ActivationCommand
}

func (s *Service) launchSupport(installed bool, gatewayReachable bool) (bool, string) {
	if !installed {
		return false, "claude binary not found"
	}
	if !gatewayReachable {
		return false, "gateway unreachable"
	}
	if !s.GatewayHosted {
		return false, "panel is not hosted by the running gateway"
	}
	if !s.hasInteractiveTerminal() {
		return false, "interactive terminal unavailable"
	}
	return true, ""
}

func ClaudeLaunchEnv(base []string, cfg config.Config) []string {
	env := make([]string, 0, len(base)+4)
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "ANTHROPIC_") {
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"ANTHROPIC_BASE_URL="+BaseURL(cfg),
		"ANTHROPIC_AUTH_TOKEN="+cfg.Server.ClientKey,
		"ANTHROPIC_API_KEY="+cfg.Server.ClientKey,
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1",
	)
	return env
}

func BaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

func ActivationCommand(config.Config) string {
	if runtime.GOOS == "windows" {
		return "arkroute activate claude | Invoke-Expression"
	}
	return `eval "$(arkroute activate claude)"`
}

func DefaultGatewayReachable(cfg config.Config) bool {
	client := http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(BaseURL(cfg) + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func DefaultHasInteractiveTerminal() bool {
	for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		info, err := file.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

func DefaultStartProcess(spec ProcessSpec) (int, error) {
	cmd := exec.Command(spec.Command)
	cmd.Env = spec.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func validateConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		return fmt.Errorf("server.client_key is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if !isLoopbackHost(cfg.Server.Host) {
		return fmt.Errorf("server.host must be loopback for CLI tool launch")
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

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkroute/config.yaml"
	}
	return home + string(os.PathSeparator) + ".arkroute" + string(os.PathSeparator) + "config.yaml"
}
```

- [ ] **Step 4: Run core tests**

Run:

```sh
go test -count=1 ./internal/clitools
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```sh
git add internal/clitools/service.go internal/clitools/service_test.go
git commit -m "feat: add cli tools launch service"
```

---

### Task 2: Panel CLI Tools HTTP Handlers

**Files:**
- Create: `internal/panel/cli_tools.go`
- Modify: `internal/panel/server.go`
- Modify: `internal/panel/server_test.go`

- [ ] **Step 1: Write failing panel route tests**

Add these tests to `internal/panel/server_test.go`:

```go
func TestCLIToolsRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-tools", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCLIToolsReturnsStatusWithValidToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{
		Sessions:   store,
		ConfigPath: path,
		CLITools:   clitools.NewService(path, false),
	})
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-tools", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"id":"claude"`, `"activation_command"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestCLIToolsLaunchMethodRequiresPost(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-tools/claude/launch", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
```

Add this import to `internal/panel/server_test.go`:

```go
import "github.com/bloodstalk1/arkroute/internal/clitools"
```

- [ ] **Step 2: Run panel tests and verify they fail**

Run:

```sh
go test -count=1 ./internal/panel
```

Expected: FAIL because `Deps.CLITools` and `/internal/cli-tools` routes do not exist.

- [ ] **Step 3: Create panel handler file**

Create `internal/panel/cli_tools.go`:

```go
package panel

import (
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/clitools"
)

type CLIToolsService interface {
	Status() (clitools.ListResponse, error)
	LaunchClaude() (clitools.LaunchResponse, error)
}

func handleCLIToolsStatus(service CLIToolsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"schema_version": 1, "error": "cli tools unavailable"})
			return
		}
		resp, err := service.Status()
		if err != nil {
			writeCLIToolsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleClaudeLaunch(service CLIToolsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"schema_version": 1, "error": "cli tools unavailable"})
			return
		}
		resp, err := service.LaunchClaude()
		if err != nil {
			writeCLIToolsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func writeCLIToolsError(w http.ResponseWriter, err error) {
	if cliErr, ok := err.(*clitools.Error); ok {
		writeJSON(w, cliErr.Status, map[string]any{
			"schema_version": 1,
			"code":           cliErr.Code,
			"error":          cliErr.Message,
			"remediation":    cliErr.Remediation,
		})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
}
```

- [ ] **Step 4: Register panel dependency and routes**

In `internal/panel/server.go`, add a field to `Deps`:

```go
CLITools CLIToolsService
```

In `Routes`, add these route registrations after setup/log routes:

```go
mux.HandleFunc("/internal/cli-tools", withSetupToken(deps.Sessions, handleCLIToolsStatus(deps.CLITools)))
mux.HandleFunc("/internal/cli-tools/claude/launch", withSetupToken(deps.Sessions, handleClaudeLaunch(deps.CLITools)))
```

- [ ] **Step 5: Run panel tests**

Run:

```sh
go test -count=1 ./internal/panel
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```sh
git add internal/panel/cli_tools.go internal/panel/server.go internal/panel/server_test.go
git commit -m "feat: add panel cli tools endpoints"
```

---

### Task 3: Mount CLI Tools In Gateway And Temporary Panel

**Files:**
- Modify: `internal/client/claude/server.go`
- Modify: `internal/client/claude/server_test.go`
- Modify: `internal/app/setup.go`

- [ ] **Step 1: Write failing gateway mount test**

Add this test near existing setup-session tests in `internal/client/claude/server_test.go`:

```go
func TestCLIToolsMountedOnGatewayWithSetupSession(t *testing.T) {
	srv := testServer(t)
	sessionReq := httptest.NewRequest(http.MethodPost, "/internal/setup/session", nil)
	sessionReq.Header.Set("Authorization", "Bearer local-key")
	sessionRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionRec.Code, sessionRec.Body.String())
	}
	var sessionPayload struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.Unmarshal(sessionRec.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/internal/cli-tools", nil)
	req.Header.Set("X-Arkroute-Setup-Token", sessionPayload.SetupToken)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"claude"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run gateway test and verify it fails**

Run:

```sh
go test -count=1 ./internal/client/claude -run TestCLIToolsMountedOnGatewayWithSetupSession
```

Expected: FAIL with 404 or missing route.

- [ ] **Step 3: Mount gateway-hosted service**

In `internal/client/claude/server.go`, add the import:

```go
import "github.com/bloodstalk1/arkroute/internal/clitools"
```

In the `panel.Routes(panel.Deps{...})` call, add:

```go
CLITools: clitools.NewService(s.configPath, true),
```

After existing `/internal/setup/logs` mount, add:

```go
mux.Handle("/internal/cli-tools", panelHandler)
mux.Handle("/internal/cli-tools/claude/launch", panelHandler)
```

- [ ] **Step 4: Add temporary-panel service**

In `internal/app/setup.go`, add the import:

```go
import "github.com/bloodstalk1/arkroute/internal/clitools"
```

In `runTemporaryPanelServer`, add `CLITools` to `panel.Deps`:

```go
CLITools: clitools.NewService(path, false),
```

- [ ] **Step 5: Run gateway and setup package tests**

Run:

```sh
go test -count=1 ./internal/client/claude ./internal/app
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```sh
git add internal/client/claude/server.go internal/client/claude/server_test.go internal/app/setup.go
git commit -m "feat: mount cli tools in panel hosts"
```

---

### Task 4: CLI Tools Panel UI

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`

- [ ] **Step 1: Add CLI Tools state and loader**

In `web-ui/src/App.jsx`, add a nav item:

```jsx
{ id: "cli-tools", icon: "ph-terminal-window", label: "CLI Tools" },
```

Inside `App`, add state:

```jsx
const [cliTools, setCliTools] = useState([]);
const [cliToolsStatus, setCliToolsStatus] = useState({ text: "", type: "" });
const [launchingTool, setLaunchingTool] = useState("");
```

Add a loader next to `loadLogs`:

```jsx
const loadCliTools = useCallback((cancelled = () => false) => {
  return fetch("/internal/cli-tools", { headers: apiHeaders })
    .then((resp) => (resp.ok ? resp.json() : resp.json().then((data) => Promise.reject(new Error(data.error || resp.statusText)))))
    .then((data) => {
      if (!cancelled()) {
        setCliTools(data.tools || []);
      }
    })
    .catch((err) => {
      if (!cancelled()) {
        setCliToolsStatus({ text: err.message, type: "error" });
      }
    });
}, [apiHeaders]);
```

In the `useEffect` that reacts to `activeTab`, add:

```jsx
if (activeTab === "cli-tools") {
  loadCliTools(isCancelled);
}
```

- [ ] **Step 2: Add copy and launch handlers**

Inside `App`, add:

```jsx
const handleCopyActivation = async (tool) => {
  try {
    await navigator.clipboard.writeText(tool.activation_command || "arkroute activate claude");
    setCliToolsStatus({ text: "Activation command copied.", type: "ok" });
  } catch (err) {
    setCliToolsStatus({ text: tool.activation_command || "arkroute activate claude", type: "info" });
  }
};

const handleLaunchClaude = async () => {
  setLaunchingTool("claude");
  setCliToolsStatus({ text: "Launching Claude Code...", type: "" });
  try {
    const resp = await fetch("/internal/cli-tools/claude/launch", { method: "POST", headers: apiHeaders });
    const data = await resp.json();
    if (!resp.ok) {
      const remediation = data.remediation ? ` ${data.remediation}` : "";
      setCliToolsStatus({ text: `${data.error || resp.statusText}.${remediation}`, type: "error" });
      return;
    }
    setCliToolsStatus({ text: `Claude Code launched with pid ${data.pid}.`, type: "ok" });
    loadCliTools();
  } catch (err) {
    setCliToolsStatus({ text: `Launch failed: ${err.message}`, type: "error" });
  } finally {
    setLaunchingTool("");
  }
};
```

- [ ] **Step 3: Add CLI Tools tab markup**

Add this tab before the System tab:

```jsx
<div className={`tab-content ${activeTab === "cli-tools" ? "active" : ""}`}>
  <PageHeader
    icon="ph-terminal-window"
    eyebrow="local clients"
    title="CLI Tools"
    description="Inspect local client readiness and launch supported tools through the Arkroute gateway."
    stats={[{ label: "tools", value: cliTools.length }]}
  />
  <section className="operator-panel cli-tools-panel">
    {cliTools.length > 0 ? cliTools.map((tool) => {
      const ready = tool.installed && tool.gateway_reachable;
      const canLaunch = ready && tool.launch_supported && launchingTool !== tool.id;
      return (
        <article className="cli-tool-row" key={tool.id}>
          <div className="cli-tool-main">
            <StatusBadge tone={canLaunch ? "ok" : ready ? "pending" : "error"}>
              {canLaunch ? "Ready" : ready ? "Launch unavailable" : "Needs attention"}
            </StatusBadge>
            <h3><i className="ph-light ph-terminal-window"></i>{tool.name}</h3>
            <code>{tool.command}</code>
          </div>
          <div className="cli-tool-details">
            <DataRow label="Binary">{tool.installed ? "found" : "not found"}</DataRow>
            <DataRow label="Gateway">{tool.gateway_reachable ? "reachable" : "offline"}</DataRow>
            <DataRow label="Base URL">{tool.base_url || "not configured"}</DataRow>
            <DataRow label="Discovery">{tool.model_discovery ? "enabled" : "disabled"}</DataRow>
          </div>
          {tool.launch_blocked_reason && (
            <div className="terminal-note cli-tool-note">
              <i className="ph-light ph-info"></i>
              <span>{tool.launch_blocked_reason}</span>
            </div>
          )}
          <div className="cli-tool-actions">
            <button type="button" disabled={!canLaunch} onClick={handleLaunchClaude}>
              <i className="ph-bold ph-play"></i>
              {launchingTool === tool.id ? "Launching" : "Launch"}
            </button>
            <button type="button" className="btn-secondary" onClick={() => handleCopyActivation(tool)}>
              <i className="ph-bold ph-copy"></i>
              Copy Env
            </button>
          </div>
        </article>
      );
    }) : (
      <EmptyState icon="ph-terminal-window" title="No CLI tools detected">
        Refresh the panel after the gateway session is ready.
      </EmptyState>
    )}
    {cliToolsStatus.text && (
      <div className={`status-box ${cliToolsStatus.type}`}>{cliToolsStatus.text}</div>
    )}
  </section>
</div>
```

- [ ] **Step 4: Add styles**

Append to `web-ui/src/index.css`:

```css
.cli-tools-panel {
  display: grid;
  gap: 14px;
  padding: 18px;
}

.cli-tool-row {
  display: grid;
  gap: 16px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.018);
  padding: 18px;
}

.cli-tool-main {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.cli-tool-main h3 {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 18px;
}

.cli-tool-main code {
  color: var(--muted);
  font-size: 12px;
}

.cli-tool-details {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.cli-tool-note {
  margin: 0;
}

.cli-tool-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.cli-tool-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.48;
}

.status-indicator.error {
  border-color: rgba(255, 107, 107, 0.24);
  background: rgba(255, 107, 107, 0.06);
  color: var(--error);
}

.status-box.info {
  color: var(--accent-2);
}

@media (max-width: 760px) {
  .cli-tool-main {
    align-items: flex-start;
    flex-direction: column;
  }

  .cli-tool-details {
    grid-template-columns: 1fr;
  }
}
```

- [ ] **Step 5: Build frontend**

Run:

```sh
npm run build:frontend
```

Expected: PASS and generated panel assets updated under `internal/panel/assets/`.

- [ ] **Step 6: Commit**

Run:

```sh
git add web-ui/src/App.jsx web-ui/src/index.css internal/panel/assets
git commit -m "feat: add cli tools panel"
```

---

### Task 5: Full Verification

**Files:**
- No source edits expected.

- [ ] **Step 1: Run backend tests**

Run:

```sh
go test -count=1 ./...
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run:

```sh
npm run build:frontend
```

Expected: PASS.

- [ ] **Step 3: Inspect final git status**

Run:

```sh
git status --short
```

Expected: only intentional committed changes remain. Pre-existing unrelated `internal/client/openai/*` changes may still appear and must not be reverted.

- [ ] **Step 4: Commit verification-only asset changes if build changed hashes**

If Step 2 regenerated `internal/panel/assets/*` after the Task 4 commit, commit only generated panel assets:

```sh
git add internal/panel/assets
git commit -m "build: refresh embedded panel assets"
```

Expected: commit created only when assets changed.

---

## Self-Review Notes

- Spec coverage: core service covers sanitized Claude env, gateway preflight, launch support gating, and process start. Panel handlers cover setup-session protection and JSON errors. Gateway/temp mounts cover both panel hosting modes. Frontend covers the CLI Tools module and copy/launch actions. Verification covers Go tests and frontend build.
- Import-cycle check: `internal/clitools` is independent. `internal/panel` imports `internal/clitools`; `internal/app` imports both `internal/panel` and `internal/clitools`; no package imports `internal/app`.
- Frontend test check: no frontend test runner exists, so the plan uses `npm run build:frontend` as required by the revised spec.
