package app

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

func TestPrintStatusFallsBackWhenServerUnreachable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 1
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PrintStatus(path, &out); err != nil {
		t.Fatalf("PrintStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "server: unreachable") {
		t.Fatalf("status output = %q", out.String())
	}
}

func TestPrintStatusUsesAdminWhenReachable(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":1,"version":"dev","generation":7,"provider_count":1,"model_count":1,"route_count":1,"last_reload_error_class":"config_validation_failed","last_reload_error":"config validation failed: routes[0].targets"}`))
	}))
	defer admin.Close()

	parsed, err := url.Parse(admin.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Host = host
	cfg.Server.Port = port
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PrintStatus(path, &out); err != nil {
		t.Fatalf("PrintStatus() error = %v", err)
	}
	for _, want := range []string{"server: running", "version: dev", "generation: 7", "last_reload_error: config_validation_failed config validation failed: routes[0].targets"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q: %q", want, out.String())
		}
	}
}

func TestDoctorReportsReloadReachable(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("ok"))
		case "/internal/status":
			if r.Header.Get("Authorization") != "Bearer local-key" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"schema_version":1}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	if err := Doctor(path, &out); err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !strings.Contains(out.String(), "reload: reachable") {
		t.Fatalf("status output = %q", out.String())
	}
}

func TestReloadPostsInternalReload(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/reload" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded","generation":2,"config_loaded_at":"2026-06-02T00:00:00Z"}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	if err := Reload(path, "", "", &out); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !strings.Contains(out.String(), "reloaded generation 2") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestReloadUsesClientKeyOverride(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded","generation":2,"config_loaded_at":"2026-06-02T00:00:00Z"}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "new-key")
	var out bytes.Buffer
	if err := Reload(path, "", "old-key", &out); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
}

func TestReloadUsesAddressOverride(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded","generation":2,"config_loaded_at":"2026-06-02T00:00:00Z"}`))
	}))
	defer admin.Close()
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 1
	path := writeAppCommandConfig(t, cfg)
	var out bytes.Buffer
	if err := Reload(path, admin.URL, "", &out); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
}

func TestReloadReportsFailurePayload(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"failed","generation":1,"error_class":"config_validation_failed","error":"config validation failed: routes[0].targets: must contain at least one target"}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	err := Reload(path, "", "", &out)
	if err == nil {
		t.Fatal("Reload() error = nil")
	}
	if !strings.Contains(err.Error(), "routes[0].targets") {
		t.Fatalf("error = %v", err)
	}
}

func TestReloadReportsAuthFailure(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid client key"}}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	err := Reload(path, "", "bad-key", &out)
	if err == nil {
		t.Fatal("Reload() error = nil")
	}
	if !strings.Contains(err.Error(), "admin auth failed") && !strings.Contains(err.Error(), "admin_auth_failed") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "admin_malformed_response") {
		t.Fatalf("error = %v", err)
	}
}

func TestReloadRejectsMalformedSuccessPayload(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"status":"reloaded"}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	err := Reload(path, "", "", &out)
	if err == nil {
		t.Fatal("Reload() error = nil")
	}
	if !strings.Contains(err.Error(), "admin_malformed_response") {
		t.Fatalf("error = %v", err)
	}
}

func writeAppCommandConfigForURL(t *testing.T, rawURL string, key string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.MinimalValidConfig(key)
	cfg.Server.Host = host
	cfg.Server.Port = port
	return writeAppCommandConfig(t, cfg)
}

func writeAppCommandConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServeSetupGuidanceForNoProvider(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	got := ServeSetupGuidance(cfg)
	want := "provider\n  status  not configured\n  next    arkroute setup\n"
	if got != want {
		t.Fatalf("ServeSetupGuidance() = %q, want %q", got, want)
	}
}

func TestServeSetupGuidanceEmptyWhenProviderConfigured(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	if got := ServeSetupGuidance(cfg); got != "" {
		t.Fatalf("ServeSetupGuidance() = %q, want empty", got)
	}
}
