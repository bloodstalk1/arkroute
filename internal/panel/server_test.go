package panel

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/clitools"
	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "arkroute-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", tmpDir)

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

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
func TestPolicyInspectRequiresSessionToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	handler := Routes(Deps{Sessions: store})
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=openrouter-sonnet", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPolicyInspectReturnsModelPolicyWithValidToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].APIKey = "sk-secret"
	cfg.Models[0].UpstreamModel = "deepseek/deepseek-v4-pro"
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=openrouter-sonnet", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"schema_version":1`, `"model_id":"openrouter-sonnet"`, `"matched_policies"`, `"resolved_reasoning"`, `"deepseek-v4-openai-compatible"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("response leaked provider key: %s", rec.Body.String())
	}
}

func TestPolicyInspectMissingModelReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := savePanelConfig(path, config.MinimalValidConfig("local-key")); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/inspect?model_id=missing", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

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

func TestPolicyOverrideSaveDisablesDeepSeekBuiltinAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].ID = "deepseek"
	cfg.Models[0].ID = "deepseek-v4-pro"
	cfg.Models[0].ProviderID = "deepseek"
	cfg.Models[0].UpstreamModel = "deepseek-v4-pro"
	cfg.Models[0].Capabilities.Reasoning = false
	cfg.Routes[0].Targets[0].ModelID = "deepseek-v4-pro"
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
			ProviderIDs:    []string{cfg.Models[0].ProviderID},
			UpstreamModels: []string{cfg.Models[0].UpstreamModel},
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
