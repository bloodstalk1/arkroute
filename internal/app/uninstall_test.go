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
