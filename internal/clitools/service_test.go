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
