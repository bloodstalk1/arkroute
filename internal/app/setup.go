package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bloodstalk1/arkroute/internal/clitools"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/panel"
	"github.com/bloodstalk1/arkroute/internal/security"
	"gopkg.in/yaml.v3"
)

type SetupOptions struct {
	ConfigPath     string
	NoBrowser      bool
	Host           string
	Port           int
	ExitAfterPrint bool
	OpenBrowser    func(string) error
}

type PanelOptions struct {
	ConfigPath     string
	NoBrowser      bool
	ExitAfterPrint bool
	OpenBrowser    func(string) error
}

func Setup(options SetupOptions, w io.Writer) error {
	path := pathOrDefault(options.ConfigPath)
	host := options.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("host must be loopback")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		key, err := security.GenerateClientKey()
		if err != nil {
			return err
		}
		if err := saveConfig(path, config.BootstrapLocalConfig(key)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	port := options.Port
	if port == 0 {
		port = config.DefaultServerPort
	}
	actualPort, err := findAvailableSetupPort(host, port)
	if err != nil {
		return err
	}
	store := panel.NewSessionStore(15 * time.Minute)
	token := store.Issue()
	writeTerminalOutput(w, setupReadyMessage(host, actualPort, token, path))
	if !options.NoBrowser {
		open := options.OpenBrowser
		if open == nil {
			open = openBrowserURL
		}
		url := fmt.Sprintf("http://%s:%d/setup#setup_token=%s", host, actualPort, token)
		if err := open(url); err != nil {
			fmt.Fprintf(w, "browser open failed: %v\n", err)
		}
	}
	if options.ExitAfterPrint {
		return nil
	}
	writer := func(cfg config.Config) error {
		return WriteClaudeSettings("", cfg)
	}
	return runTemporaryPanelServer(path, host, actualPort, store, writer)
}

func Panel(options PanelOptions, w io.Writer) error {
	path := pathOrDefault(options.ConfigPath)
	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	token, err := requestPanelSession(cfg)
	if err != nil {
		store := panel.NewSessionStore(15 * time.Minute)
		token = store.Issue()
		actualPort, portErr := findAvailableSetupPort(cfg.Server.Host, cfg.Server.Port)
		if portErr != nil {
			return portErr
		}
		page := "panel"
		if !HasUsableProvider(cfg) {
			page = "setup"
		}
		url := fmt.Sprintf("http://%s:%d/%s#setup_token=%s", cfg.Server.Host, actualPort, page, token)
		writeTerminalOutput(w, panelReadyMessage(page, url, path))
		if !options.NoBrowser {
			open := options.OpenBrowser
			if open == nil {
				open = openBrowserURL
			}
			if err := open(url); err != nil {
				fmt.Fprintf(w, "browser open failed: %v\n", err)
			}
		}
		if options.ExitAfterPrint {
			return nil
		}
		writer := func(c config.Config) error {
			return WriteClaudeSettings("", c)
		}
		return runTemporaryPanelServer(path, cfg.Server.Host, actualPort, store, writer)
	}
	page := "panel"
	if !HasUsableProvider(cfg) {
		page = "setup"
	}
	url := fmt.Sprintf("http://%s:%d/%s#setup_token=%s", cfg.Server.Host, cfg.Server.Port, page, token)
	writeTerminalOutput(w, panelReadyMessage(page, url, path))
	if !options.NoBrowser {
		open := options.OpenBrowser
		if open == nil {
			open = openBrowserURL
		}
		if err := open(url); err != nil {
			fmt.Fprintf(w, "browser open failed: %v\n", err)
		}
	}
	if options.ExitAfterPrint {
		return nil
	}
	return nil
}

func saveConfig(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func findAvailableSetupPort(host string, preferred int) (int, error) {
	for port := preferred; port < preferred+20; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no available loopback setup port starting at %d", preferred)
}

func isLoopbackHost(host string) bool {
	return security.IsLoopbackHost(host)
}

func requestPanelSession(cfg config.Config) (string, error) {
	url := fmt.Sprintf("http://%s:%d/internal/setup/session", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("setup session failed: status %d", resp.StatusCode)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		SetupToken    string `json:"setup_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.SchemaVersion != 1 || payload.SetupToken == "" {
		return "", fmt.Errorf("setup session malformed")
	}
	return payload.SetupToken, nil
}

func runTemporaryPanelServer(path string, host string, port int, store *panel.SessionStore, claudeWriter func(config.Config) error) error {
	handler := panel.Routes(panel.Deps{
		Sessions:             store,
		ConfigPath:           path,
		ClaudeSettingsWriter: claudeWriter,
		CLITools:             clitools.NewService(path, false),
	})
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	return srv.ListenAndServe()
}
