package panel

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/clitools"
	"github.com/bloodstalk1/arkroute/internal/config"
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
