# Config Safety + Compatibility Policy UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make panel-driven config changes safe to apply, easy to export/import, and able to edit compatibility policy overrides for a selected model without adding model-specific hard-code.

**Architecture:** Route every panel write through a shared config store that validates YAML, creates bounded backups, writes atomically, and reloads runtime state through the existing `OnSave` path. Add a small policy edit service that mutates `compatibility_policies` with stable model override IDs, expose setup-token protected endpoints, and extend the current Routes panel policy inspector with editable override controls plus Config Safety import/export UI.

**Tech Stack:** Go `net/http`, Arkroute YAML config, existing `internal/config` validation and redaction, setup-token protected panel handlers, React in `web-ui/src/App.jsx`, existing `npm test` and `npm run build` frontend pipeline.

---

## File Structure

- Modify: `internal/config/load.go`
  - Add `LoadBytes` so file load and import validation share the same migration/default path.
- Test: `internal/config/config_test.go`
  - Cover `LoadBytes` validation and migration/default behavior.
- Create: `internal/panel/config_store.go`
  - Own panel config loading, safe saving, atomic writes, backup creation, backup pruning, export, and import parsing.
- Create: `internal/panel/config_store_test.go`
  - Unit tests for backup creation, pruning, atomic save output, invalid import rejection, and redacted export behavior.
- Modify: `internal/panel/server.go`
  - Replace direct config writes with `ConfigStore`; register config transfer and policy override endpoints.
- Create: `internal/panel/config_transfer.go`
  - HTTP handlers for config export, import validation, and import apply.
- Test: `internal/panel/server_test.go`
  - Endpoint auth, export redaction, invalid import non-overwrite, backup response, and reload callback coverage.
- Create: `internal/policyedit/policy.go`
  - Pure config mutation helpers for model-scoped compatibility policy overrides.
- Create: `internal/policyedit/policy_test.go`
  - Tests for stable IDs, upsert, delete, invalid values, and DeepSeek V4 builtin override behavior.
- Modify: `internal/policyinspect/inspect.go`
  - Include model user override metadata for the UI editor.
- Test: `internal/policyinspect/inspect_test.go`
  - Verify user override metadata does not expose provider secrets and matches generated override policy.
- Create: `internal/panel/policy_override.go`
  - HTTP handlers for saving and deleting model compatibility policy overrides.
- Modify: `internal/client/claude/server.go`
  - Mount new panel endpoints through the gateway-hosted panel handler.
- Test: `internal/client/claude/server_test.go`
  - Verify gateway-hosted setup-token access for config transfer and policy override endpoints.
- Modify: `web-ui/src/App.jsx`
  - Add Config Safety UI and editable policy override controls in the Routes panel.
- Modify: `web-ui/src/index.css`
  - Add focused styles for config transfer controls and policy override editor.
- Build output: `internal/panel/assets/panel.html`
  - Updated by `npm run build` after React changes.

---

## Task 1: Shared Config Parsing And Safe Store

**Files:**
- Modify: `internal/config/load.go`
- Test: `internal/config/config_test.go`
- Create: `internal/panel/config_store.go`
- Create: `internal/panel/config_store_test.go`
- Modify: `internal/panel/server.go`

- [ ] **Step 1: Add failing tests for `config.LoadBytes`**

Append these tests to `internal/config/config_test.go`:

```go
func TestLoadBytesAppliesDefaultsAndMigration(t *testing.T) {
	data := []byte(`
version: 1
server:
  host: 127.0.0.1
  client_key: local-key
clients:
  claude:
    enabled: true
providers: []
models: []
routes: []
profiles: {}
`)
	cfg, err := LoadBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != DefaultServerPort {
		t.Fatalf("port = %d, want default %d", cfg.Server.Port, DefaultServerPort)
	}
	if cfg.Server.UpstreamTimeoutSeconds != 600 {
		t.Fatalf("timeout = %d, want 600", cfg.Server.UpstreamTimeoutSeconds)
	}
	if cfg.Profiles == nil {
		t.Fatal("profiles = nil, want initialized map")
	}
}

func TestLoadBytesRejectsInvalidYAML(t *testing.T) {
	if _, err := LoadBytes([]byte("server:\n  host: [")); err == nil {
		t.Fatal("LoadBytes error = nil, want YAML parse error")
	}
}
```

- [ ] **Step 2: Run config tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/config
```

Expected: `FAIL` with `undefined: LoadBytes`.

- [ ] **Step 3: Implement `LoadBytes` and reuse it from `LoadFile`**

Edit `internal/config/load.go` so the top of the file is:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return LoadBytes(data)
}

func LoadBytes(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg, err := Migrate(cfg)
	if err != nil {
		return Config{}, err
	}
	ApplyDefaults(&cfg)
	return cfg, nil
}
```

Keep the existing `ApplyDefaults`, `MinimalValidConfig`, and `BootstrapLocalConfig` functions unchanged below this block.

- [ ] **Step 4: Run config tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/config
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/config`.

- [ ] **Step 5: Add failing config store tests**

Create `internal/panel/config_store_test.go`:

```go
package panel

import (
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
```

- [ ] **Step 6: Run config store tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run 'TestConfigStore'
```

Expected: `FAIL` with `undefined: ConfigStore`.

- [ ] **Step 7: Implement `ConfigStore`**

Create `internal/panel/config_store.go`:

```go
package panel

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
	"gopkg.in/yaml.v3"
)

const defaultBackupLimit = 20

type ConfigStore struct {
	Path        string
	BackupDir   string
	BackupLimit int
	Now         func() time.Time
}

type ConfigSaveResult struct {
	BackupPath string `json:"backup_path,omitempty"`
}

func NewConfigStore(path string) ConfigStore {
	return ConfigStore{Path: path, BackupLimit: defaultBackupLimit}
}

func (s ConfigStore) LoadOrBootstrap() (config.Config, error) {
	cfg, err := config.LoadFile(s.Path)
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

func (s ConfigStore) ParseImport(data []byte) (config.Config, error) {
	cfg, err := config.LoadBytes(data)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func (s ConfigStore) Export(redacted bool) ([]byte, error) {
	cfg, err := config.LoadFile(s.Path)
	if err != nil {
		return nil, err
	}
	if redacted {
		cfg = config.Redacted(cfg)
	}
	return yaml.Marshal(cfg)
}

func (s ConfigStore) Save(cfg config.Config) (ConfigSaveResult, error) {
	if err := cfg.Validate(); err != nil {
		return ConfigSaveResult{}, err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return ConfigSaveResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return ConfigSaveResult{}, err
	}
	backupPath, err := s.createBackup()
	if err != nil {
		return ConfigSaveResult{}, err
	}
	if err := atomicWriteFile(s.Path, data, 0o600); err != nil {
		return ConfigSaveResult{}, err
	}
	if err := s.PruneBackups(); err != nil {
		return ConfigSaveResult{}, err
	}
	return ConfigSaveResult{BackupPath: backupPath}, nil
}

func (s ConfigStore) PruneBackups() error {
	limit := s.limit()
	if limit < 1 {
		return nil
	}
	dir := s.backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "config-") && strings.HasSuffix(name, ".yaml") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) <= limit {
		return nil
	}
	for _, name := range names[:len(names)-limit] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func (s ConfigStore) createBackup() (string, error) {
	current, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	dir := s.backupDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := "config-" + s.now().Format("20060102-150405") + ".yaml"
	path := filepath.Join(dir, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("config-%s-%02d.yaml", s.now().Format("20060102-150405"), i))
	}
	if err := os.WriteFile(path, current, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s ConfigStore) backupDir() string {
	if s.BackupDir != "" {
		return s.BackupDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(filepath.Dir(s.Path), "backups")
	}
	return filepath.Join(home, ".arkroute", "backups")
}

func (s ConfigStore) limit() int {
	if s.BackupLimit == 0 {
		return defaultBackupLimit
	}
	return s.BackupLimit
}

func (s ConfigStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
```

- [ ] **Step 8: Update existing panel config helper functions to use `ConfigStore`**

In `internal/panel/server.go`, remove imports that are no longer used by the direct save implementation:

```go
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
```

Keep `errors` and `os` if another function in the file still uses them. Remove `path/filepath` and `gopkg.in/yaml.v3` from this file after moving writes to `config_store.go`.

Replace `loadOrBootstrapConfig` and `savePanelConfig` with:

```go
func loadOrBootstrapConfig(path string) (config.Config, error) {
	return NewConfigStore(path).LoadOrBootstrap()
}

func savePanelConfig(path string, cfg config.Config) error {
	_, err := NewConfigStore(path).Save(cfg)
	return err
}
```

- [ ] **Step 9: Run panel config store tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run 'TestConfigStore'
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/panel`.

- [ ] **Step 10: Commit Task 1**

Run:

```bash
git add internal/config/load.go internal/config/config_test.go internal/panel/config_store.go internal/panel/config_store_test.go internal/panel/server.go
git commit -m "feat: add safe panel config store"
```

Expected: commit succeeds.

---

## Task 2: Config Export, Import Validate, And Import Apply Endpoints

**Files:**
- Create: `internal/panel/config_transfer.go`
- Modify: `internal/panel/server.go`
- Test: `internal/panel/server_test.go`
- Modify: `internal/client/claude/server.go`
- Test: `internal/client/claude/server_test.go`

- [ ] **Step 1: Add failing panel endpoint tests**

Append these tests to `internal/panel/server_test.go`:

```go
func TestConfigExportRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/config/export?redacted=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestConfigExportRedactedDoesNotLeakSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-secret"
	cfg.Providers[0].Headers = map[string]string{"X-Test": "secret-header"}
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodGet, "/internal/config/export?redacted=1", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, leaked := range []string{"local-key", "sk-secret", "secret-header"} {
		if strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("redacted export leaked %q: %s", leaked, rec.Body.String())
		}
	}
}

func TestConfigImportValidateRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := savePanelConfig(path, config.MinimalValidConfig("local-key")); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	body := strings.NewReader(`{"yaml":"version: 1\nserver:\n  host: 0.0.0.0\n  port: 2002\n  client_key: local-key\nproviders: []\nmodels: []\nroutes: []\nprofiles: {}\n"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/config/import/validate", body)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	current, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if current.Server.Host != "127.0.0.1" {
		t.Fatalf("current host = %q, want unchanged loopback host", current.Server.Host)
	}
}

func TestConfigImportApplyCreatesBackupAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := savePanelConfig(path, config.MinimalValidConfig("old-key")); err != nil {
		t.Fatal(err)
	}
	reloads := 0
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{
		Sessions:   store,
		ConfigPath: path,
		OnSave: func() error {
			reloads++
			return nil
		},
	})
	body := strings.NewReader(`{"yaml":"version: 1\nserver:\n  host: 127.0.0.1\n  port: 2002\n  client_key: new-key\n  upstream_timeout_seconds: 600\nclients:\n  claude:\n    enabled: true\n    model_discovery: true\nproviders: []\nmodels: []\nroutes: []\nprofiles: {}\n"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/config/import/apply", body)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
	if !strings.Contains(rec.Body.String(), `"backup_path"`) {
		t.Fatalf("body missing backup_path: %s", rec.Body.String())
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ClientKey != "new-key" {
		t.Fatalf("client key = %q, want new-key", cfg.Server.ClientKey)
	}
}
```

- [ ] **Step 2: Run endpoint tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run 'TestConfig'
```

Expected: `FAIL` because `/internal/config/export`, `/internal/config/import/validate`, and `/internal/config/import/apply` are not registered.

- [ ] **Step 3: Implement config transfer handlers**

Create `internal/panel/config_transfer.go`:

```go
package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type configImportRequest struct {
	YAML string `json:"yaml"`
}

type configImportSummary struct {
	Providers             int `json:"providers"`
	Models                int `json:"models"`
	Routes                int `json:"routes"`
	CompatibilityPolicies int `json:"compatibility_policies"`
}

func handleConfigExport(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		redacted := r.URL.Query().Get("redacted") == "1" || r.URL.Query().Get("redacted") == "true"
		data, err := NewConfigStore(path).Export(redacted)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		name := "arkroute-config.yaml"
		if redacted {
			name = "arkroute-config-redacted.yaml"
		}
		w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func handleConfigImportValidate(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		cfg, err := decodeImportConfig(r, NewConfigStore(path))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, importErrorResponse(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"valid":          true,
			"summary":        summarizeConfig(cfg),
			"config":         config.Redacted(cfg),
		})
	}
}

func handleConfigImportApply(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		store := NewConfigStore(path)
		cfg, err := decodeImportConfig(r, store)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, importErrorResponse(err))
			return
		}
		result, err := store.Save(cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, importErrorResponse(err))
			return
		}
		if onSave != nil {
			if err := onSave(); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "reload failed: " + err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"status":         "saved",
			"backup_path":    result.BackupPath,
			"summary":        summarizeConfig(cfg),
			"config":         config.Redacted(cfg),
		})
	}
}

func decodeImportConfig(r *http.Request, store ConfigStore) (config.Config, error) {
	var input configImportRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		return config.Config{}, errors.New("invalid import payload")
	}
	if strings.TrimSpace(input.YAML) == "" {
		return config.Config{}, errors.New("yaml must be non-empty")
	}
	return store.ParseImport([]byte(input.YAML))
}

func summarizeConfig(cfg config.Config) configImportSummary {
	return configImportSummary{
		Providers:             len(cfg.Providers),
		Models:                len(cfg.Models),
		Routes:                len(cfg.Routes),
		CompatibilityPolicies: len(cfg.CompatibilityPolicies),
	}
}

func importErrorResponse(err error) map[string]any {
	response := map[string]any{
		"schema_version": 1,
		"valid":          false,
		"error":          err.Error(),
	}
	var validationErr config.ValidationError
	if errors.As(err, &validationErr) {
		response["fields"] = validationErr.Fields
	}
	return response
}
```

- [ ] **Step 4: Register config transfer routes**

In `internal/panel/server.go`, add these handlers inside `Routes` after `/internal/setup/status`:

```go
	mux.HandleFunc("/internal/config/export", withSetupToken(deps.Sessions, handleConfigExport(deps.ConfigPath)))
	mux.HandleFunc("/internal/config/import/validate", withSetupToken(deps.Sessions, handleConfigImportValidate(deps.ConfigPath)))
	mux.HandleFunc("/internal/config/import/apply", withSetupToken(deps.Sessions, handleConfigImportApply(deps.ConfigPath, deps.OnSave)))
```

- [ ] **Step 5: Run panel endpoint tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run 'TestConfig'
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/panel`.

- [ ] **Step 6: Mount config transfer routes through gateway**

In `internal/client/claude/server.go`, add these mounts after `/internal/setup/status`:

```go
	mux.Handle("/internal/config/export", panelHandler)
	mux.Handle("/internal/config/import/validate", panelHandler)
	mux.Handle("/internal/config/import/apply", panelHandler)
```

- [ ] **Step 7: Add gateway mount tests**

Append this test to `internal/client/claude/server_test.go`:

```go
func TestGatewayMountsConfigExportEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := os.WriteFile(path, []byte(`version: 1
server:
  host: 127.0.0.1
  port: 2002
  client_key: local-key
  upstream_timeout_seconds: 600
clients:
  claude:
    enabled: true
    model_discovery: true
providers: []
models: []
routes: []
profiles: {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = cfg
	server := NewServer(Deps{ConfigPath: path})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodGet, "/internal/config/export?redacted=1", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "version: 1") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

If `internal/client/claude/server_test.go` does not already import `os`, `path/filepath`, `strings`, or `github.com/bloodstalk1/arkroute/internal/config`, add only the missing imports.

- [ ] **Step 8: Run gateway tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/client/claude -run 'TestGatewayMountsConfigExportEndpoint'
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/client/claude`.

- [ ] **Step 9: Commit Task 2**

Run:

```bash
git add internal/panel/config_transfer.go internal/panel/server.go internal/panel/server_test.go internal/client/claude/server.go internal/client/claude/server_test.go
git commit -m "feat: add safe config import export endpoints"
```

Expected: commit succeeds.

---

## Task 3: Compatibility Policy Override Service And Endpoint

**Files:**
- Create: `internal/policyedit/policy.go`
- Create: `internal/policyedit/policy_test.go`
- Modify: `internal/policyinspect/inspect.go`
- Test: `internal/policyinspect/inspect_test.go`
- Create: `internal/panel/policy_override.go`
- Modify: `internal/panel/server.go`
- Test: `internal/panel/server_test.go`
- Modify: `internal/client/claude/server.go`
- Test: `internal/client/claude/server_test.go`

- [ ] **Step 1: Add failing policy edit service tests**

Create `internal/policyedit/policy_test.go`:

```go
package policyedit_test

import (
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/policyedit"
	"github.com/bloodstalk1/arkroute/internal/policyinspect"
)

func TestStableModelPolicyIDSanitizesModelID(t *testing.T) {
	got := policyedit.StableModelPolicyID("DeepSeek/V4 Pro++")
	want := "model-deepseek-v4-pro-compat"
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
}

func TestUpsertModelOverrideDisablesBuiltinDeepSeekV4AutoThinking(t *testing.T) {
	falseValue := false
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].UpstreamModel = "deepseek-v4-pro"
	cfg.Models[0].Capabilities.Reasoning = false

	updated, policy, err := policyedit.UpsertModelOverride(cfg, policyedit.OverrideInput{
		ModelID:        "deepseek-v4-pro",
		AutoEnable:    &falseValue,
		Replay:        &falseValue,
		OmitToolChoice: &falseValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.ID != "model-deepseek-v4-pro-compat" {
		t.Fatalf("policy id = %q", policy.ID)
	}
	inspection, err := policyinspect.InspectModel(updated, "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ResolvedReasoning.AutoEnable || inspection.ResolvedReasoning.Enabled {
		t.Fatalf("resolved reasoning = %+v, want auto thinking disabled", inspection.ResolvedReasoning)
	}
	if inspection.ResolvedReasoning.Replay || inspection.ResolvedReasoning.OmitToolChoice {
		t.Fatalf("resolved reasoning = %+v, want replay and omit_tool_choice disabled", inspection.ResolvedReasoning)
	}
}

func TestUpsertModelOverrideRejectsInvalidAutoEffort(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	if _, _, err := policyedit.UpsertModelOverride(cfg, policyedit.OverrideInput{
		ModelID:    cfg.Models[0].ID,
		AutoEffort: "ultracode",
	}); err == nil {
		t.Fatal("UpsertModelOverride error = nil, want invalid effort error")
	}
}

func TestDeleteModelOverrideRemovesGeneratedPolicy(t *testing.T) {
	trueValue := true
	cfg := config.MinimalValidConfig("local-key")
	updated, _, err := policyedit.UpsertModelOverride(cfg, policyedit.OverrideInput{
		ModelID:    cfg.Models[0].ID,
		AutoEnable: &trueValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, policyID, err := policyedit.DeleteModelOverride(updated, cfg.Models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if policyID != policyedit.StableModelPolicyID(cfg.Models[0].ID) {
		t.Fatalf("policy id = %q", policyID)
	}
	if len(deleted.CompatibilityPolicies) != 0 {
		t.Fatalf("compatibility policies = %+v, want empty", deleted.CompatibilityPolicies)
	}
}
```

- [ ] **Step 2: Run policy edit tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/policyedit
```

Expected: `FAIL` because package `internal/policyedit` has no implementation.

- [ ] **Step 3: Implement policy edit service**

Create `internal/policyedit/policy.go`:

```go
package policyedit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

var ErrModelNotFound = errors.New("model not found")

type OverrideInput struct {
	ModelID        string `json:"model_id"`
	AutoEnable    *bool  `json:"auto_enable,omitempty"`
	AutoEffort    string `json:"auto_effort,omitempty"`
	Replay        *bool  `json:"replay,omitempty"`
	OmitToolChoice *bool  `json:"omit_tool_choice,omitempty"`
}

type UserOverride struct {
	Exists         bool   `json:"exists"`
	PolicyID       string `json:"policy_id"`
	AutoEnable     *bool  `json:"auto_enable,omitempty"`
	AutoEffort     string `json:"auto_effort,omitempty"`
	Replay         *bool  `json:"replay,omitempty"`
	OmitToolChoice *bool  `json:"omit_tool_choice,omitempty"`
}

func StableModelPolicyID(modelID string) string {
	clean := strings.ToLower(strings.TrimSpace(modelID))
	var b strings.Builder
	lastDash := false
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		value = "model"
	}
	return "model-" + value + "-compat"
}

func UpsertModelOverride(cfg config.Config, input OverrideInput) (config.Config, config.CompatibilityPolicyConfig, error) {
	model, err := findModel(cfg, input.ModelID)
	if err != nil {
		return config.Config{}, config.CompatibilityPolicyConfig{}, err
	}
	if err := validateOverrideInput(input); err != nil {
		return config.Config{}, config.CompatibilityPolicyConfig{}, err
	}
	policy := config.CompatibilityPolicyConfig{
		ID: StableModelPolicyID(model.ID),
		Match: config.CompatibilityMatchConfig{
			ProviderIDContains:    []string{model.ProviderID},
			UpstreamModelPatterns: []string{model.UpstreamModel},
		},
		Reasoning: config.CompatibilityReasoningConfig{
			AutoEnable:     cloneBool(input.AutoEnable),
			AutoEffort:     input.AutoEffort,
			Replay:         cloneBool(input.Replay),
			OmitToolChoice: cloneBool(input.OmitToolChoice),
		},
	}
	next := removePolicyByID(cfg.CompatibilityPolicies, policy.ID)
	next = append(next, policy)
	cfg.CompatibilityPolicies = next
	if err := cfg.Validate(); err != nil {
		return config.Config{}, config.CompatibilityPolicyConfig{}, err
	}
	return cfg, policy, nil
}

func DeleteModelOverride(cfg config.Config, modelID string) (config.Config, string, error) {
	if _, err := findModel(cfg, modelID); err != nil {
		return config.Config{}, "", err
	}
	policyID := StableModelPolicyID(modelID)
	cfg.CompatibilityPolicies = removePolicyByID(cfg.CompatibilityPolicies, policyID)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, "", err
	}
	return cfg, policyID, nil
}

func FindModelOverride(cfg config.Config, modelID string) UserOverride {
	policyID := StableModelPolicyID(modelID)
	for _, policy := range cfg.CompatibilityPolicies {
		if policy.ID == policyID {
			return UserOverride{
				Exists:         true,
				PolicyID:       policy.ID,
				AutoEnable:     cloneBool(policy.Reasoning.AutoEnable),
				AutoEffort:     policy.Reasoning.AutoEffort,
				Replay:         cloneBool(policy.Reasoning.Replay),
				OmitToolChoice: cloneBool(policy.Reasoning.OmitToolChoice),
			}
		}
	}
	return UserOverride{Exists: false, PolicyID: policyID}
}

func validateOverrideInput(input OverrideInput) error {
	if strings.TrimSpace(input.ModelID) == "" {
		return errors.New("model_id must be non-empty")
	}
	if input.AutoEffort != "" {
		switch input.AutoEffort {
		case "low", "medium", "high", "max":
		default:
			return fmt.Errorf("auto_effort must be low, medium, high, or max")
		}
	}
	if input.AutoEnable == nil && input.AutoEffort == "" && input.Replay == nil && input.OmitToolChoice == nil {
		return errors.New("at least one override field must be set")
	}
	return nil
}

func findModel(cfg config.Config, modelID string) (config.ModelConfig, error) {
	for _, model := range cfg.Models {
		if model.ID == modelID {
			return model, nil
		}
	}
	return config.ModelConfig{}, fmt.Errorf("%w: %s", ErrModelNotFound, modelID)
}

func removePolicyByID(policies []config.CompatibilityPolicyConfig, policyID string) []config.CompatibilityPolicyConfig {
	next := make([]config.CompatibilityPolicyConfig, 0, len(policies))
	for _, policy := range policies {
		if policy.ID != policyID {
			next = append(next, policy)
		}
	}
	return next
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
```

The generated policy uses the existing compatibility schema. The match is model-scoped by combining provider ID containment and upstream model pattern matching. Do not add any `if strings.Contains(model, "deepseek-v4")` checks in runtime code.

- [ ] **Step 4: Run policy edit tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/policyedit
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/policyedit`.

- [ ] **Step 5: Extend policy inspector response with user override metadata**

Modify `internal/policyinspect/inspect.go` imports to include:

```go
	"github.com/bloodstalk1/arkroute/internal/policyedit"
```

Add a field to `Inspection`:

```go
	UserOverride      policyedit.UserOverride                    `json:"user_override"`
```

Set the field in the `InspectModel` return:

```go
		UserOverride:      policyedit.FindModelOverride(cfg, compat.Model.ID),
```

- [ ] **Step 6: Add inspector metadata test**

Append to `internal/policyinspect/inspect_test.go`:

```go
func TestInspectModelIncludesUserOverrideMetadata(t *testing.T) {
	falseValue := false
	cfg := config.MinimalValidConfig("local-key")
	cfg.CompatibilityPolicies = []config.CompatibilityPolicyConfig{{
		ID: "model-openrouter-sonnet-compat",
		Match: config.CompatibilityMatchConfig{
			ProviderIDContains:    []string{cfg.Models[0].ProviderID},
			UpstreamModelPatterns: []string{cfg.Models[0].UpstreamModel},
		},
		Reasoning: config.CompatibilityReasoningConfig{
			Replay: &falseValue,
		},
	}}
	got, err := InspectModel(cfg, cfg.Models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UserOverride.Exists {
		t.Fatalf("user override = %+v, want exists", got.UserOverride)
	}
	if got.UserOverride.Replay == nil || *got.UserOverride.Replay {
		t.Fatalf("user override replay = %v, want false", got.UserOverride.Replay)
	}
}
```

- [ ] **Step 7: Run policy inspector tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/policyinspect
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/policyinspect`.

- [ ] **Step 8: Add failing panel override endpoint tests**

Append these tests to `internal/panel/server_test.go`:

```go
func TestPolicyOverrideSaveDisablesDeepSeekBuiltinAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].UpstreamModel = "deepseek-v4-pro"
	cfg.Models[0].Capabilities.Reasoning = false
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	reloads := 0
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{
		Sessions:   store,
		ConfigPath: path,
		OnSave: func() error {
			reloads++
			return nil
		},
	})
	body := strings.NewReader(`{"model_id":"deepseek-v4-pro","auto_enable":false,"replay":false,"omit_tool_choice":false}`)
	req := httptest.NewRequest(http.MethodPut, "/internal/policy/override", body)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
	if !strings.Contains(rec.Body.String(), `"backup_path"`) {
		t.Fatalf("body missing backup_path: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"auto_enable":false`) {
		t.Fatalf("body = %s, want auto_enable false", rec.Body.String())
	}
}

func TestPolicyOverrideRejectsInvalidEffortWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	body := strings.NewReader(`{"model_id":"openrouter-sonnet","auto_effort":"ultracode"}`)
	req := httptest.NewRequest(http.MethodPut, "/internal/policy/override", body)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("config changed after invalid override\nbefore=%s\nafter=%s", string(before), string(after))
	}
}

func TestPolicyOverrideDeleteRemovesGeneratedPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	trueValue := true
	cfg.CompatibilityPolicies = []config.CompatibilityPolicyConfig{{
		ID: "model-openrouter-sonnet-compat",
		Match: config.CompatibilityMatchConfig{
			ProviderIDContains:    []string{cfg.Models[0].ProviderID},
			UpstreamModelPatterns: []string{cfg.Models[0].UpstreamModel},
		},
		Reasoning: config.CompatibilityReasoningConfig{AutoEnable: &trueValue},
	}}
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodDelete, "/internal/policy/override?model_id=openrouter-sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	updated, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.CompatibilityPolicies) != 0 {
		t.Fatalf("policies = %+v, want empty", updated.CompatibilityPolicies)
	}
}
```

Add `os` to the imports in `internal/panel/server_test.go` if missing.

- [ ] **Step 9: Run panel override tests to verify failure**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/panel -run 'TestPolicyOverride'
```

Expected: `FAIL` because `/internal/policy/override` is not registered.

- [ ] **Step 10: Implement policy override handlers**

Create `internal/panel/policy_override.go`:

```go
package panel

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/policyedit"
	"github.com/bloodstalk1/arkroute/internal/policyinspect"
)

func handlePolicyOverride(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			savePolicyOverride(w, r, path, onSave)
		case http.MethodDelete:
			deletePolicyOverride(w, r, path, onSave)
		default:
			w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
		}
	}
}

func savePolicyOverride(w http.ResponseWriter, r *http.Request, path string, onSave func() error) {
	var input policyedit.OverrideInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid policy override payload"})
		return
	}
	store := NewConfigStore(path)
	cfg, err := store.LoadOrBootstrap()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	cfg, policy, err := policyedit.UpsertModelOverride(cfg, input)
	if err != nil {
		writeJSON(w, policyEditStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
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
	inspection, err := policyinspect.InspectModel(cfg, input.ModelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": 1,
		"status":         "saved",
		"policy_id":      policy.ID,
		"backup_path":    result.BackupPath,
		"inspection":     inspection,
		"config":         config.Redacted(cfg),
	})
}

func deletePolicyOverride(w http.ResponseWriter, r *http.Request, path string, onSave func() error) {
	modelID := r.URL.Query().Get("model_id")
	store := NewConfigStore(path)
	cfg, err := store.LoadOrBootstrap()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	cfg, policyID, err := policyedit.DeleteModelOverride(cfg, modelID)
	if err != nil {
		writeJSON(w, policyEditStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
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
	inspection, err := policyinspect.InspectModel(cfg, modelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": 1,
		"status":         "deleted",
		"policy_id":      policyID,
		"backup_path":    result.BackupPath,
		"inspection":     inspection,
		"config":         config.Redacted(cfg),
	})
}

func reloadAfterPanelSave(onSave func() error) error {
	if onSave == nil {
		return nil
	}
	if err := onSave(); err != nil {
		return errors.New("reload failed: " + err.Error())
	}
	return nil
}

func policyEditStatus(err error) int {
	if errors.Is(err, policyedit.ErrModelNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
```

- [ ] **Step 11: Register and mount policy override endpoint**

In `internal/panel/server.go`, add after `/internal/policy/inspect`:

```go
	mux.HandleFunc("/internal/policy/override", withSetupToken(deps.Sessions, handlePolicyOverride(deps.ConfigPath, deps.OnSave)))
```

In `internal/client/claude/server.go`, add after `/internal/policy/inspect`:

```go
	mux.Handle("/internal/policy/override", panelHandler)
```

- [ ] **Step 12: Run policy override tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/policyedit ./internal/policyinspect ./internal/panel -run 'TestPolicy'
```

Expected: all listed packages pass.

- [ ] **Step 13: Add gateway policy override mount test**

Append to `internal/client/claude/server_test.go`:

```go
func TestGatewayMountsPolicyOverrideEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	if err := panelTestWriteConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Deps{ConfigPath: path})
	handler := server.Routes()
	token := server.sessions.Issue()
	req := httptest.NewRequest(http.MethodPut, "/internal/policy/override", strings.NewReader(`{"model_id":"openrouter-sonnet","replay":false}`))
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"policy_id":"model-openrouter-sonnet-compat"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func panelTestWriteConfig(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
```

Add `gopkg.in/yaml.v3` and `github.com/bloodstalk1/arkroute/internal/panel` imports only if the file needs them.

- [ ] **Step 14: Run gateway policy override test**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/client/claude -run 'TestGatewayMountsPolicyOverrideEndpoint'
```

Expected: `ok  	github.com/bloodstalk1/arkroute/internal/client/claude`.

- [ ] **Step 15: Commit Task 3**

Run:

```bash
git add internal/policyedit internal/policyinspect/inspect.go internal/policyinspect/inspect_test.go internal/panel/policy_override.go internal/panel/server.go internal/panel/server_test.go internal/client/claude/server.go internal/client/claude/server_test.go
git commit -m "feat: edit model compatibility overrides"
```

Expected: commit succeeds.

---

## Task 4: Config Safety UI

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`
- Build output: `internal/panel/assets/panel.html`

- [ ] **Step 1: Add Config Safety state and helpers**

In `web-ui/src/App.jsx`, near the other React state declarations in `App`, add:

```jsx
  const [configDraft, setConfigDraft] = useState("");
  const [configTransferStatus, setConfigTransferStatus] = useState({ text: "", type: "" });
  const [configImportSummary, setConfigImportSummary] = useState(null);
```

Add these helper functions inside `App` before `handleSaveSetup`:

```jsx
  const downloadConfig = async (redacted) => {
    setConfigTransferStatus({ text: "", type: "" });
    const response = await fetch(`/internal/config/export?redacted=${redacted ? "1" : "0"}`, {
      headers: apiHeaders
    });
    const text = await response.text();
    if (!response.ok) {
      setConfigTransferStatus({ text: text || "Export failed", type: "error" });
      return;
    }
    const blob = new Blob([text], { type: "application/x-yaml;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = redacted ? "arkroute-config-redacted.yaml" : "arkroute-config.yaml";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
    setConfigTransferStatus({ text: redacted ? "Redacted config exported" : "Config exported", type: "ok" });
  };

  const copyRedactedConfig = async () => {
    setConfigTransferStatus({ text: "", type: "" });
    const response = await fetch("/internal/config/export?redacted=1", {
      headers: apiHeaders
    });
    const text = await response.text();
    if (!response.ok) {
      setConfigTransferStatus({ text: text || "Copy failed", type: "error" });
      return;
    }
    await navigator.clipboard.writeText(text);
    setConfigTransferStatus({ text: "Redacted config copied", type: "ok" });
  };

  const validateConfigDraft = async () => {
    setConfigTransferStatus({ text: "", type: "" });
    setConfigImportSummary(null);
    const response = await fetch("/internal/config/import/validate", {
      method: "POST",
      headers: { ...apiHeaders, "Content-Type": "application/json" },
      body: JSON.stringify({ yaml: configDraft })
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) {
      setConfigTransferStatus({ text: result.error || "Config validation failed", type: "error" });
      return;
    }
    setConfigImportSummary(result.summary || null);
    setConfigTransferStatus({ text: "Config is valid", type: "ok" });
  };

  const applyConfigDraft = async () => {
    setConfigTransferStatus({ text: "", type: "" });
    const response = await fetch("/internal/config/import/apply", {
      method: "POST",
      headers: { ...apiHeaders, "Content-Type": "application/json" },
      body: JSON.stringify({ yaml: configDraft })
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) {
      setConfigTransferStatus({ text: result.error || "Import failed", type: "error" });
      return;
    }
    setConfig(result.config);
    setConfigImportSummary(result.summary || null);
    setConfigTransferStatus({
      text: result.backup_path ? `Config imported, backup: ${result.backup_path}` : "Config imported",
      type: "ok"
    });
    loadStatus();
  };
```

- [ ] **Step 2: Add Config Safety panel markup**

In the System tab content of `web-ui/src/App.jsx`, add this section inside the existing `<div className="operator-grid">` after the `Local Gateway` card and before the `Claude Code` card:

```jsx
          <section className="operator-card config-safety-card">
            <div className="card-heading">
              <div>
                <StatusBadge tone={configImportSummary ? "ok" : "pending"}>{configImportSummary ? "validated" : "config"}</StatusBadge>
                <h3><i className="ph-light ph-floppy-disk-back"></i>Config Safety</h3>
              </div>
            </div>
            <div className="config-action-row">
              <button className="secondary-button" type="button" onClick={() => downloadConfig(false)}>
                <i className="ph-light ph-download-simple"></i>Export full
              </button>
              <button className="secondary-button" type="button" onClick={() => downloadConfig(true)}>
                <i className="ph-light ph-shield-check"></i>Export redacted
              </button>
              <button className="secondary-button" type="button" onClick={copyRedactedConfig}>
                <i className="ph-light ph-copy"></i>Copy redacted
              </button>
            </div>
            <textarea
              className="config-import-textarea"
              value={configDraft}
              onChange={(event) => setConfigDraft(event.target.value)}
              spellCheck="false"
              placeholder="version: 1"
            />
            <div className="config-action-row">
              <button className="secondary-button" type="button" onClick={validateConfigDraft} disabled={!configDraft.trim()}>
                <i className="ph-light ph-check-circle"></i>Validate import
              </button>
              <button className="primary-button" type="button" onClick={applyConfigDraft} disabled={!configDraft.trim()}>
                <i className="ph-light ph-upload-simple"></i>Apply import
              </button>
            </div>
            {configImportSummary && (
              <div className="config-summary-row">
                <DataRow label="Providers">{configImportSummary.providers}</DataRow>
                <DataRow label="Models">{configImportSummary.models}</DataRow>
                <DataRow label="Routes">{configImportSummary.routes}</DataRow>
                <DataRow label="Policies">{configImportSummary.compatibility_policies}</DataRow>
              </div>
            )}
            {configTransferStatus.text && <div className={`status-box ${configTransferStatus.type}`}>{configTransferStatus.text}</div>}
          </section>
```

Use the existing System tab container already in the file. Do not add a new top-level tab for Config Safety.

- [ ] **Step 3: Add Config Safety styles**

Append to `web-ui/src/index.css`:

```css
.config-safety-card {
  gap: 16px;
}

.config-action-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.config-import-textarea {
  width: 100%;
  min-height: 180px;
  resize: vertical;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-soft);
  color: var(--text);
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace;
  font-size: 13px;
  line-height: 1.5;
  padding: 12px;
}

.config-import-textarea:focus {
  outline: 2px solid rgba(37, 99, 235, 0.24);
  border-color: rgba(37, 99, 235, 0.55);
}

.config-summary-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

@media (max-width: 720px) {
  .config-summary-row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
```

If CSS variables differ in the current file, use the existing variable names from nearby operator-card inputs.

- [ ] **Step 4: Build frontend**

Run:

```bash
npm run build
```

Expected: frontend build passes and updates `internal/panel/assets/panel.html`.

- [ ] **Step 5: Commit Task 4**

Run:

```bash
git add web-ui/src/App.jsx web-ui/src/index.css internal/panel/assets/panel.html
git commit -m "feat: add config safety panel"
```

Expected: commit succeeds.

---

## Task 5: Compatibility Policy Editor UI

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`
- Build output: `internal/panel/assets/panel.html`

- [ ] **Step 1: Add policy override form helpers**

In `web-ui/src/App.jsx`, add this helper outside `App`, near `PolicyValue`:

```jsx
function policyBoolToSelect(value) {
  if (value === true) return "true";
  if (value === false) return "false";
  return "inherit";
}

function selectToPolicyBool(value) {
  if (value === "true") return true;
  if (value === "false") return false;
  return null;
}
```

Add this state inside `App`:

```jsx
  const [policyDraft, setPolicyDraft] = useState({
    auto_enable: "inherit",
    auto_effort: "",
    replay: "inherit",
    omit_tool_choice: "inherit"
  });
  const [policySaving, setPolicySaving] = useState(false);
```

Add this effect after the existing policy inspect fetch effect:

```jsx
  useEffect(() => {
    const override = policyInspect?.user_override;
    if (!override) {
      setPolicyDraft({
        auto_enable: "inherit",
        auto_effort: "",
        replay: "inherit",
        omit_tool_choice: "inherit"
      });
      return;
    }
    setPolicyDraft({
      auto_enable: policyBoolToSelect(override.auto_enable),
      auto_effort: override.auto_effort || "",
      replay: policyBoolToSelect(override.replay),
      omit_tool_choice: policyBoolToSelect(override.omit_tool_choice)
    });
  }, [policyInspect?.user_override?.policy_id, policyInspect?.user_override?.exists]);
```

Add these functions inside `App`:

```jsx
  const updatePolicyDraft = (field, value) => {
    setPolicyDraft((current) => ({ ...current, [field]: value }));
  };

  const savePolicyOverride = async () => {
    if (!selectedModelId) return;
    setPolicySaving(true);
    setPolicyInspectStatus({ text: "", type: "" });
    const body = {
      model_id: selectedModelId,
      auto_effort: policyDraft.auto_effort || ""
    };
    const autoEnable = selectToPolicyBool(policyDraft.auto_enable);
    const replay = selectToPolicyBool(policyDraft.replay);
    const omitToolChoice = selectToPolicyBool(policyDraft.omit_tool_choice);
    if (autoEnable !== null) body.auto_enable = autoEnable;
    if (replay !== null) body.replay = replay;
    if (omitToolChoice !== null) body.omit_tool_choice = omitToolChoice;
    const response = await fetch("/internal/policy/override", {
      method: "PUT",
      headers: { ...apiHeaders, "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
    const result = await response.json().catch(() => ({}));
    setPolicySaving(false);
    if (!response.ok) {
      setPolicyInspectStatus({ text: result.error || "Policy save failed", type: "error" });
      return;
    }
    setConfig(result.config);
    setPolicyInspect(result.inspection);
    setPolicyInspectStatus({ text: "Policy override saved", type: "ok" });
    loadStatus();
  };

  const resetPolicyOverride = async () => {
    if (!selectedModelId) return;
    setPolicySaving(true);
    setPolicyInspectStatus({ text: "", type: "" });
    const response = await fetch(`/internal/policy/override?model_id=${encodeURIComponent(selectedModelId)}`, {
      method: "DELETE",
      headers: apiHeaders
    });
    const result = await response.json().catch(() => ({}));
    setPolicySaving(false);
    if (!response.ok) {
      setPolicyInspectStatus({ text: result.error || "Policy reset failed", type: "error" });
      return;
    }
    setConfig(result.config);
    setPolicyInspect(result.inspection);
    setPolicyInspectStatus({ text: "Policy reset to builtin", type: "ok" });
    loadStatus();
  };
```

- [ ] **Step 2: Extend `PolicyInspector` props and editor markup**

Change the function signature:

```jsx
function PolicyInspector({
  inspection,
  loading,
  status,
  draft,
  saving,
  onDraftChange,
  onSave,
  onReset
}) {
```

Inside the rendered `<section>`, after the `policy-value-grid`, add:

```jsx
      <div className="policy-editor">
        <div className="policy-editor-heading">
          <div>
            <span className="eyebrow">user override</span>
            <h4>{inspection.user_override?.policy_id || "model override"}</h4>
          </div>
          <StatusBadge tone={inspection.user_override?.exists ? "ok" : "pending"}>
            {inspection.user_override?.exists ? "active" : "builtin"}
          </StatusBadge>
        </div>
        <div className="policy-editor-grid">
          <label>
            <span>auto_enable</span>
            <select value={draft.auto_enable} onChange={(event) => onDraftChange("auto_enable", event.target.value)}>
              <option value="inherit">inherit</option>
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          </label>
          <label>
            <span>auto_effort</span>
            <select value={draft.auto_effort} onChange={(event) => onDraftChange("auto_effort", event.target.value)}>
              <option value="">inherit</option>
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="max">max</option>
            </select>
          </label>
          <label>
            <span>replay</span>
            <select value={draft.replay} onChange={(event) => onDraftChange("replay", event.target.value)}>
              <option value="inherit">inherit</option>
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          </label>
          <label>
            <span>omit_tool_choice</span>
            <select value={draft.omit_tool_choice} onChange={(event) => onDraftChange("omit_tool_choice", event.target.value)}>
              <option value="inherit">inherit</option>
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          </label>
        </div>
        <div className="policy-editor-actions">
          <button className="primary-button" type="button" onClick={onSave} disabled={saving}>
            <i className="ph-light ph-floppy-disk"></i>{saving ? "Saving" : "Save override"}
          </button>
          <button className="secondary-button" type="button" onClick={onReset} disabled={saving || !inspection.user_override?.policy_id}>
            <i className="ph-light ph-arrow-counter-clockwise"></i>Reset to builtin
          </button>
        </div>
      </div>
```

Update the call site in the Routes tab:

```jsx
            <PolicyInspector
              inspection={policyInspect}
              loading={policyInspectLoading}
              status={policyInspectStatus}
              draft={policyDraft}
              saving={policySaving}
              onDraftChange={updatePolicyDraft}
              onSave={savePolicyOverride}
              onReset={resetPolicyOverride}
            />
```

- [ ] **Step 3: Add policy editor styles**

Append to `web-ui/src/index.css`:

```css
.policy-editor {
  display: grid;
  gap: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
}

.policy-editor-heading,
.policy-editor-actions {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
}

.policy-editor-heading h4 {
  margin: 4px 0 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--text);
}

.policy-editor-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.policy-editor-grid label {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.policy-editor-grid label span {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--muted);
}

.policy-editor-grid select {
  min-width: 0;
  width: 100%;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-soft);
  color: var(--text);
  padding: 9px 10px;
}

@media (max-width: 960px) {
  .policy-editor-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 620px) {
  .policy-editor-grid,
  .policy-editor-heading,
  .policy-editor-actions {
    grid-template-columns: 1fr;
  }

  .policy-editor-heading,
  .policy-editor-actions {
    align-items: stretch;
    flex-direction: column;
  }
}
```

If `.primary-button` and `.secondary-button` have different local class names, use the existing button classes from provider setup actions.

- [ ] **Step 4: Build frontend**

Run:

```bash
npm run build
```

Expected: frontend build passes and updates `internal/panel/assets/panel.html`.

- [ ] **Step 5: Manual local UI smoke check**

Run the app in one terminal:

```bash
go run ./cmd/arkroute serve --config /tmp/arkroute-plan-b-config.yaml
```

Expected: server starts on the configured local address and prints no panic.

In another terminal, open the panel session by using the existing setup session flow from the current README or CLI help. Verify:

- The Routes panel still renders registered models and policy inspector.
- Selecting a DeepSeek V4 model shows builtin matches.
- Setting `auto_enable=false`, `replay=false`, and `omit_tool_choice=false` saves a user override.
- Reset to builtin removes the generated user override.
- System panel config export downloads YAML.
- Redacted copy contains `[redacted]` and not provider API key values.

Stop the server with `Ctrl-C`.

- [ ] **Step 6: Commit Task 5**

Run:

```bash
git add web-ui/src/App.jsx web-ui/src/index.css internal/panel/assets/panel.html
git commit -m "feat: add compatibility policy editor UI"
```

Expected: commit succeeds.

---

## Task 6: Full Verification And Production Readiness Check

**Files:**
- No source files created in this task.
- Verifies all files touched in Tasks 1 through 5.

- [ ] **Step 1: Run focused backend tests**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./internal/config ./internal/panel ./internal/policyedit ./internal/policyinspect ./internal/client/claude
```

Expected: all packages report `ok`.

- [ ] **Step 2: Run full backend suite**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go test ./...
```

Expected: every package reports `ok` or `[no test files]`; no package reports `FAIL`.

- [ ] **Step 3: Run Go vet**

Run:

```bash
GOCACHE=/tmp/arkroute-go-build-cache go vet ./...
```

Expected: no output and exit code `0`.

- [ ] **Step 4: Run frontend test suite**

Run outside sandbox when needed for local browser/build tooling:

```bash
npm test
```

Expected: test runner exits successfully.

- [ ] **Step 5: Run frontend build**

Run:

```bash
npm run build
```

Expected: build succeeds and `internal/panel/assets/panel.html` is current.

- [ ] **Step 6: Check for accidental secret leakage in source**

Run:

```bash
rg -n "sk-[A-Za-z0-9]|api_key: [A-Za-z0-9_-]{12,}|ANTHROPIC_AUTH_TOKEN=|OPENAI_API_KEY=" internal web-ui docs --glob '!docs/superpowers/plans/*'
```

Expected: no real secret values. Test fixture strings such as `sk-secret` are acceptable only in `_test.go` files.

- [ ] **Step 7: Inspect git status**

Run:

```bash
git status --short --branch
```

Expected: branch contains only intentional Plan B commits and no unexpected unstaged files.

- [ ] **Step 8: Commit verification note if docs changed after Task 5**

If build output or docs changed during verification, commit them:

```bash
git add internal/panel/assets/panel.html docs/superpowers/plans/2026-06-07-config-safety-compatibility-policy-ui.md
git commit -m "docs: add config safety policy ui plan"
```

Expected: commit succeeds when there are staged changes. If there are no staged changes, skip this command.

---

## Acceptance Mapping

- User can disable builtin DeepSeek V4 auto thinking from UI: Task 3 service test and Task 5 editor save flow set `auto_enable=false` for the selected model override.
- User can disable or enable reasoning replay from UI: Task 3 accepts `replay` as an optional boolean and Task 5 exposes true/false/inherit.
- Saving policy reloads runtime config: Task 3 handlers call `OnSave`; panel tests count the reload callback.
- Invalid values are rejected before writing config: Task 1 validates parsed imports, Task 3 rejects invalid `auto_effort`, and tests compare file contents after failure.
- Every panel overwrite creates a backup: Task 1 `ConfigStore.Save` backs up the current file before atomic rename. First write has no backup path because no previous config exists.
- Invalid import cannot overwrite current config: Task 2 import validation test verifies the existing file remains unchanged.
- Redacted export never leaks provider API keys: Task 1 store test and Task 2 HTTP test check client key, provider API key, and provider headers.
- Backup pruning deterministic and tested: Task 1 sorts backup filenames and keeps the newest `BackupLimit` entries.

## Execution Choice

Plan complete and saved to `docs/superpowers/plans/2026-06-07-config-safety-compatibility-policy-ui.md`. Two execution options:

**1. Subagent-Driven (recommended)** - dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** - execute tasks in this session using executing-plans, batch execution with checkpoints.
