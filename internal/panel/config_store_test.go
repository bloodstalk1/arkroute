package panel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestConfigStoreSaveCreatesBackupBeforeOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	backupDir := filepath.Join(dir, "backups")
	store := ConfigStore{
		Path:        path,
		BackupDir:   backupDir,
		BackupLimit: 20,
		Now:         func() time.Time { return time.Date(2026, 6, 7, 10, 11, 12, 0, time.UTC) },
	}

	first := config.MinimalValidConfig("first-key")
	if _, err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	second := config.MinimalValidConfig("second-key")
	result, err := store.Save(second)
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath empty, want backup path")
	}
	backupData, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(backupData), "first-key") {
		t.Fatalf("backup = %s, want previous config", string(backupData))
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "second-key") {
		t.Fatalf("current = %s, want new config", string(current))
	}
}

func TestConfigStorePrunesBackupsDeterministically(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"config-20260607-100000.yaml",
		"config-20260607-100001.yaml",
		"config-20260607-100002.yaml",
	} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := ConfigStore{
		Path:        filepath.Join(dir, "config.yaml"),
		BackupDir:   backupDir,
		BackupLimit: 2,
		Now:         func() time.Time { return time.Date(2026, 6, 7, 10, 0, 3, 0, time.UTC) },
	}
	if err := store.PruneBackups(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	want := []string{"config-20260607-100001.yaml", "config-20260607-100002.yaml"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("backups = %v, want %v", got, want)
	}
}

func TestConfigStoreParseImportValidatesConfig(t *testing.T) {
	store := ConfigStore{}
	invalid := []byte(`
version: 1
server:
  host: 0.0.0.0
  port: 2002
  client_key: local-key
providers: []
models: []
routes: []
profiles: {}
`)
	if _, err := store.ParseImport(invalid); err == nil {
		t.Fatal("ParseImport error = nil, want validation error")
	}
}

func TestConfigStoreExportRedactedHidesSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	store := ConfigStore{Path: path}
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-secret"
	cfg.Providers[0].Headers = map[string]string{"X-Test": "secret-header"}
	if _, err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := store.Export(true)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, leaked := range []string{"local-key", "sk-secret", "secret-header"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("redacted export leaked %q: %s", leaked, text)
		}
	}
	if !strings.Contains(text, "[redacted]") {
		t.Fatalf("redacted export = %s, want redacted marker", text)
	}
}

func TestConfigStoreSaveAndReloadRollsBackOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	backupDir := filepath.Join(dir, "backups")
	store := ConfigStore{
		Path:        path,
		BackupDir:   backupDir,
		BackupLimit: 20,
		Now:         func() time.Time { return time.Date(2026, 6, 7, 10, 11, 12, 0, time.UTC) },
	}

	first := config.MinimalValidConfig("first-key")
	if _, err := store.Save(first); err != nil {
		t.Fatal(err)
	}

	second := config.MinimalValidConfig("second-key")
	_, err := store.SaveAndReload(second, func() error {
		return errors.New("reload failed mock error")
	})
	if err == nil || !strings.Contains(err.Error(), "reload failed") {
		t.Fatalf("SaveAndReload error = %v, want reload failed", err)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "first-key") {
		t.Fatalf("current = %s, want rolled back first-key config", string(current))
	}
}

func TestConfigStoreSaveAndReloadReportsRollbackFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	backupDir := filepath.Join(dir, "backups")
	store := ConfigStore{
		Path:        path,
		BackupDir:   backupDir,
		BackupLimit: 20,
		Now:         func() time.Time { return time.Date(2026, 6, 7, 10, 11, 12, 0, time.UTC) },
	}

	first := config.MinimalValidConfig("first-key")
	if _, err := store.Save(first); err != nil {
		t.Fatal(err)
	}

	second := config.MinimalValidConfig("second-key")
	_, err := store.SaveAndReload(second, func() error {
		if removeErr := os.RemoveAll(backupDir); removeErr != nil {
			t.Fatal(removeErr)
		}
		return errors.New("reload failed mock error")
	})
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("SaveAndReload error = %v, want rollback failed", err)
	}
	if strings.Contains(err.Error(), "config rolled back") {
		t.Fatalf("SaveAndReload error = %v, must not claim rollback succeeded", err)
	}
}
