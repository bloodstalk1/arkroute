package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

func TestRunNoArgsRunsSetup(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), ">_ arkroute") || !strings.Contains(stdout.String(), "/setup#setup_token=") {
		t.Fatalf("stdout missing setup url: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "missing"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: missing") {
		t.Fatalf("stderr missing unknown command: %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != "arkroute dev" {
		t.Fatalf("stdout = %q, want version", stdout.String())
	}
}

func TestRunActivateClaudePrintsExports(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "claude", "--config", path, "--host", "127.0.0.1", "--port", "20128", "--client-key", "local-key"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activate output missing %s: %q", want, out)
		}
	}
}

func TestRunActivateClaudeWritesSettings(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	settingsPath := filepath.Join(dir, "settings.json")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20134
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "claude", "--config", configPath, "--settings", settingsPath, "--write-settings"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "updated Claude settings") {
		t.Fatalf("stdout = %q, want settings update message", stdout.String())
	}
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), `"ANTHROPIC_BASE_URL": "http://127.0.0.1:20134"`) {
		t.Fatalf("settings missing base URL: %s", settings)
	}
}

func TestRunActivateClaudeWarnsAboutMismatchedSettings(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	settingsPath := filepath.Join(dir, "settings.json")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20134
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:20128"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "claude", "--config", configPath, "--settings", settingsPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	got := stdout.String()
	wants := []string{
		"export ANTHROPIC_BASE_URL",
		"# warning: Claude settings",
		"arkroute activate claude --write-settings --settings",
	}
	if runtime.GOOS == "windows" {
		wants = []string{
			"set ANTHROPIC_BASE_URL",
			"REM warning: Claude settings",
			"arkroute activate claude --write-settings --settings",
		}
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("activate output missing %q: %q", want, got)
		}
	}
}

func TestRunValidateMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "validate", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "validate failed") {
		t.Fatalf("stderr = %q, want validate failed", stderr.String())
	}
}

func TestRunDoctor(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "doctor", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "doctor failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunDoctorParsesClaudeSettingsFlag(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	configPath := filepath.Join(dir, "config.yaml")
	settingsPath := filepath.Join(dir, "settings.json")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20134
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:20128"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "doctor", "--config", configPath, "--claude-settings", settingsPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "claude_settings: mismatch") {
		t.Fatalf("stdout missing settings mismatch: %q", stdout.String())
	}
}

func TestRunLogsMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "logs", "--file", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "logs failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunTestMissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "test"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: arkroute test <model> <prompt>") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunConfigPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "config", "path"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ".arkroute") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunProviderListMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "provider", "list", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "provider list failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunStatusMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "status", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "status failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReloadMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "reload", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "reload failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReloadParsesFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "reload", "--config", "/path/does/not/exist", "--addr", "http://127.0.0.1:20128", "--client-key", "old-key"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "reload failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunVersionDebug(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "version", "--debug"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"version:", "commit:", "build_date:", "go:", "os_arch:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %s: %q", want, stdout.String())
		}
	}
}

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
