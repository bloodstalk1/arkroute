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
		_, _ = w.Write([]byte(`{"schema_version":1,"version":"dev","provider_count":1,"model_count":1,"route_count":1}`))
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
	if !strings.Contains(out.String(), "server: running") || !strings.Contains(out.String(), "version: dev") {
		t.Fatalf("status output = %q", out.String())
	}
}
