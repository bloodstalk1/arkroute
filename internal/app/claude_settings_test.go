package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

func TestWriteClaudeSettingsPreservesExistingFieldsAndUpdatesArkRouteEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","env":{"EXISTING":"keep","ANTHROPIC_BASE_URL":"http://127.0.0.1:20128"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20134

	if err := WriteClaudeSettings(path, cfg); err != nil {
		t.Fatalf("WriteClaudeSettings() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("settings JSON error = %v; data = %s", err, data)
	}
	if decoded["theme"] != "dark" {
		t.Fatalf("theme = %#v, want preserved", decoded["theme"])
	}
	env := decoded["env"].(map[string]any)
	if env["EXISTING"] != "keep" {
		t.Fatalf("existing env = %#v, want preserved", env["EXISTING"])
	}
	if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:20134" {
		t.Fatalf("ANTHROPIC_BASE_URL = %#v", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "local-key" || env["ANTHROPIC_API_KEY"] != nil {
		t.Fatalf("auth env not updated: %#v", env)
	}
	if env["CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"] != "1" {
		t.Fatalf("model discovery env = %#v", env["CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"])
	}
	if env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"] != "190000" {
		t.Fatalf("auto compact env = %#v", env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"])
	}
}

func TestDiagnoseClaudeSettingsDetectsBaseURLMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:20128"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 20134

	diagnosis, err := DiagnoseClaudeSettings(path, cfg)
	if err != nil {
		t.Fatalf("DiagnoseClaudeSettings() error = %v", err)
	}
	if !diagnosis.Exists || !diagnosis.HasBaseURL || !diagnosis.BaseURLMismatch {
		t.Fatalf("diagnosis = %+v, want mismatch", diagnosis)
	}
	if diagnosis.BaseURL != "http://127.0.0.1:20128" || diagnosis.ExpectedBaseURL != "http://127.0.0.1:20134" {
		t.Fatalf("diagnosis URLs = %+v", diagnosis)
	}
}

func TestDoctorReportsClaudeSettingsOverride(t *testing.T) {
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

	var out bytes.Buffer
	if err := DoctorWithOptions(DoctorOptions{ConfigPath: configPath, ClaudeSettingsPath: settingsPath}, &out); err != nil {
		t.Fatalf("DoctorWithOptions() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"claude_settings: mismatch",
		"claude_settings_base_url: http://127.0.0.1:20128",
		"claude_settings_expected_base_url: http://127.0.0.1:20134",
		"fix: arkroute activate claude --write-settings --settings ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q: %q", want, got)
		}
	}
}

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
	for _, secret := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY", "CLAUDE_CODE_AUTO_COMPACT_WINDOW"} {
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
