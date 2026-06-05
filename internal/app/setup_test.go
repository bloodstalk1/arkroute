package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestSetupNoBrowserCreatesBootstrapConfigAndPrintsURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var out bytes.Buffer
	err := Setup(SetupOptions{ConfigPath: path, NoBrowser: true, Host: "127.0.0.1", Port: 0, ExitAfterPrint: true}, &out)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "/setup#setup_token=") {
		t.Fatalf("output missing setup token URL: %q", got)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 0 || cfg.Server.ClientKey == "" {
		t.Fatalf("bootstrap config = %+v", cfg)
	}
}

func TestSetupUsesDefaultPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var out bytes.Buffer
	if err := Setup(SetupOptions{ConfigPath: path, NoBrowser: true, Host: "127.0.0.1", ExitAfterPrint: true}, &out); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !strings.Contains(out.String(), "http://127.0.0.1:2002/setup#setup_token=") {
		t.Fatalf("setup URL = %q, want default port 2002", out.String())
	}
}

func TestSetupPrintsBrandedTerminalOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var out bytes.Buffer
	if err := Setup(SetupOptions{ConfigPath: path, NoBrowser: true, Host: "127.0.0.1", ExitAfterPrint: true}, &out); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		">_ arkroute",
		"terminal portal gateway",
		"setup panel",
		"status  running",
		"panel   http://127.0.0.1:2002/setup#setup_token=",
		"config  " + path,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("setup output missing %q: %q", want, got)
		}
	}
}

func TestPanelNoBrowserRequiresExistingConfig(t *testing.T) {
	var out bytes.Buffer
	err := Panel(PanelOptions{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), NoBrowser: true, ExitAfterPrint: true}, &out)
	if err == nil {
		t.Fatal("Panel() error = nil, want missing config error")
	}
}

func TestSetupRejectsNonLoopbackHost(t *testing.T) {
	err := Setup(SetupOptions{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), NoBrowser: true, Host: "0.0.0.0", ExitAfterPrint: true}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "host must be loopback") {
		t.Fatalf("error = %v, want loopback validation", err)
	}
}

func TestSetupDoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if _, err := InitConfig(path, false); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Setup(SetupOptions{ConfigPath: path, NoBrowser: true, Port: 0, ExitAfterPrint: true}, &out); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("Setup() overwrote existing config")
	}
}

func TestSetupOpensBrowserWhenAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var opened string
	err := Setup(SetupOptions{
		ConfigPath: path, Host: "127.0.0.1", Port: 0, ExitAfterPrint: true,
		OpenBrowser: func(url string) error {
			opened = url
			return nil
		},
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !strings.Contains(opened, "/setup#setup_token=") {
		t.Fatalf("opened URL = %q", opened)
	}
}

func TestPanelUsesRunningGatewaySession(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/setup/session" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"setup_token":"issued-token"}`))
	}))
	defer admin.Close()
	path := writeAppCommandConfigForURL(t, admin.URL, "local-key")
	var out bytes.Buffer
	if err := Panel(PanelOptions{ConfigPath: path, NoBrowser: true, ExitAfterPrint: true}, &out); err != nil {
		t.Fatalf("Panel() error = %v", err)
	}
	if !strings.Contains(out.String(), "#setup_token=issued-token") {
		t.Fatalf("output = %q", out.String())
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestSetupActivation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	buf := &safeBuffer{}
	go func() {
		// Run Setup command which blocks on ListenAndServe
		_ = Setup(SetupOptions{
			ConfigPath:     configPath,
			NoBrowser:      true,
			Host:           "127.0.0.1",
			Port:           21500, // Starts scanning from 21500
			ExitAfterPrint: false,
		}, buf)
	}()

	var actualPort int
	var token string

	// Poll output to parse port and token
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		str := buf.String()
		idx := strings.Index(str, "http://127.0.0.1:")
		if idx != -1 {
			sub := str[idx+len("http://127.0.0.1:"):]
			slashIdx := strings.Index(sub, "/setup")
			if slashIdx != -1 {
				portStr := sub[:slashIdx]
				var err error
				actualPort, err = strconv.Atoi(portStr)
				if err == nil {
					hashIdx := strings.Index(sub, "setup_token=")
					if hashIdx != -1 {
						token = strings.TrimSpace(strings.Split(sub[hashIdx+len("setup_token="):], "\n")[0])
						break
					}
				}
			}
		}
	}

	if actualPort == 0 || token == "" {
		t.Fatalf("failed to parse setup server port and token from output: %q", buf.String())
	}

	// Make HTTP call to complete provider setup and request Claude Code activation
	body := strings.NewReader(`{"preset_id":"openrouter","api_key_mode":"config","api_key":"sk-secret","upstream_model":"anthropic/claude-sonnet-4.5","exposed_alias":"sonnet-or","route_alias":"sonnet","activate_claude":true}`)
	url := fmt.Sprintf("http://127.0.0.1:%d/internal/setup/provider", actualPort)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Arkroute-Setup-Token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("provider setup response status = %d", resp.StatusCode)
	}

	// Verify config.yaml exists and was written
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		t.Fatalf("config file load failed: %v", err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].APIKey != "sk-secret" {
		t.Fatalf("config setup incorrect: %+v", cfg)
	}

	// Verify Claude settings file was written inside redirected HOME
	claudeSettingsPath := filepath.Join(dir, ".claude", "settings.json")
	if _, err := os.Stat(claudeSettingsPath); os.IsNotExist(err) {
		t.Fatalf("Claude settings file not written at %s", claudeSettingsPath)
	}

	data, err := os.ReadFile(claudeSettingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"ANTHROPIC_BASE_URL": "http://127.0.0.1:2002"`) {
		t.Fatalf("Claude settings missing expected Base URL: %s", string(data))
	}
}
