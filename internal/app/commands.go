package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func ValidateConfig(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	fmt.Fprintln(w, "config ok")
	return nil
}

type DoctorOptions struct {
	ConfigPath         string
	ClaudeSettingsPath string
}

func Doctor(path string, w io.Writer) error {
	return DoctorWithOptions(DoctorOptions{ConfigPath: path}, w)
}

func DoctorWithOptions(options DoctorOptions, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(options.ConfigPath))
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	fmt.Fprintf(w, "config: ok\nproviders: %d\nmodels: %d\nroutes: %d\n", len(cfg.Providers), len(cfg.Models), len(cfg.Routes))
	missing := missingEnvRefs(cfg)
	for _, envName := range missing {
		fmt.Fprintf(w, "env:%s missing\n", envName)
	}
	if portAvailable(cfg.Server.Host, cfg.Server.Port) {
		fmt.Fprintln(w, "port: available")
	} else {
		fmt.Fprintln(w, "port: unavailable")
	}
	if serverReachable(cfg) {
		fmt.Fprintln(w, "server: reachable")
	} else {
		fmt.Fprintln(w, "server: unreachable")
	}
	if reloadEndpointReachable(cfg) {
		fmt.Fprintln(w, "reload: reachable")
	} else {
		fmt.Fprintln(w, "reload: unreachable")
	}
	printClaudeSettingsDiagnosis(w, cfg, options.ClaudeSettingsPath)
	return nil
}

func printClaudeSettingsDiagnosis(w io.Writer, cfg config.Config, path string) {
	diagnosis, err := DiagnoseClaudeSettings(path, cfg)
	if err != nil {
		fmt.Fprintf(w, "claude_settings: unreadable\nclaude_settings_path: %s\n", ClaudeSettingsPath(path))
		return
	}
	if !diagnosis.Exists || !diagnosis.HasBaseURL {
		return
	}
	if diagnosis.BaseURLMismatch {
		fmt.Fprintln(w, "claude_settings: mismatch")
		fmt.Fprintf(w, "claude_settings_path: %s\n", diagnosis.Path)
		fmt.Fprintf(w, "claude_settings_base_url: %s\n", diagnosis.BaseURL)
		fmt.Fprintf(w, "claude_settings_expected_base_url: %s\n", diagnosis.ExpectedBaseURL)
		fmt.Fprint(w, "fix: arkroute activate claude --write-settings")
		if path != "" {
			fmt.Fprintf(w, " --settings %s", path)
		}
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w, "claude_settings: ok")
}

func PrintLogs(path string, w io.Writer) error {
	return PrintLogsTail(path, 0, w)
}

func PrintLogsTail(path string, tail int, w io.Writer) error {
	if path == "" {
		path = DefaultLogPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	compact := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			compact = append(compact, line)
		}
	}
	if tail > 0 && len(compact) > tail {
		compact = compact[len(compact)-tail:]
	}
	for _, line := range compact {
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	return nil
}

func TestRoute(path string, model string, prompt string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 128,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]string{{
				"type": "text",
				"text": prompt,
			}},
		}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:%d/v1/messages", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(respBody))
	}
	_, err = w.Write(respBody)
	if err == nil {
		_, err = fmt.Fprintln(w)
	}
	return err
}

func Reload(path string, addrOverride string, clientKeyOverride string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	addr := strings.TrimRight(addrOverride, "/")
	if addr == "" {
		addr = fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	}
	key := cfg.Server.ClientKey
	if clientKeyOverride != "" {
		key = clientKeyOverride
	}
	req, err := http.NewRequest(http.MethodPost, addr+"/internal/reload", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("server_unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("admin auth failed: status %d", resp.StatusCode)
	}
	var payload struct {
		SchemaVersion  int    `json:"schema_version"`
		Status         string `json:"status"`
		Generation     uint64 `json:"generation"`
		ConfigLoadedAt string `json:"config_loaded_at"`
		ErrorClass     string `json:"error_class"`
		Error          string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("admin_malformed_response: %w", err)
	}
	if payload.SchemaVersion != 1 {
		return fmt.Errorf("admin_malformed_response: schema_version %d", payload.SchemaVersion)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || payload.Status != "reloaded" {
		if payload.Error != "" {
			return fmt.Errorf("reload failed: %s", payload.Error)
		}
		if payload.ErrorClass != "" {
			return fmt.Errorf("reload failed: %s", payload.ErrorClass)
		}
		return fmt.Errorf("reload failed: status %d", resp.StatusCode)
	}
	if payload.Generation == 0 || payload.ConfigLoadedAt == "" {
		return fmt.Errorf("admin_malformed_response: missing generation or config_loaded_at")
	}
	fmt.Fprintf(w, "reloaded generation %d\nconfig_loaded_at: %s\n", payload.Generation, payload.ConfigLoadedAt)
	return nil
}

func ConfigPath(w io.Writer) error {
	_, err := fmt.Fprintln(w, DefaultConfigPath())
	return err
}

func ShowConfig(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(config.Redacted(cfg), "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func ListProviders(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "PROVIDER\tTYPE\tENABLED\tBASE_URL")
	for _, provider := range cfg.Providers {
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", provider.ID, provider.Type, provider.Enabled, provider.BaseURL)
	}
	return nil
}

func ListModels(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "MODEL\tPROVIDER\tUPSTREAM\tALIAS\tSTREAM\tTOOLS")
	for _, model := range cfg.Models {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%t\n", model.ID, model.ProviderID, model.UpstreamModel, model.ExposedAlias, model.Capabilities.Streaming, model.Capabilities.Tools)
	}
	return nil
}

func ListRoutes(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "ROUTE\tSTRATEGY\tTARGETS")
	for _, route := range cfg.Routes {
		targets := make([]string, 0, len(route.Targets))
		for _, target := range route.Targets {
			targets = append(targets, target.ModelID)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", route.Alias, route.Strategy, strings.Join(targets, ","))
	}
	return nil
}

func PrintStatus(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	adminURL := fmt.Sprintf("http://%s:%d/internal/status", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodGet, adminURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(req)
	if err != nil {
		fmt.Fprintf(w, "server: unreachable\nproviders: %d\nmodels: %d\nroutes: %d\n", len(cfg.Providers), len(cfg.Models), len(cfg.Routes))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("admin auth failed: status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("admin status failed: status %d", resp.StatusCode)
	}
	var payload struct {
		SchemaVersion        int    `json:"schema_version"`
		Version              string `json:"version"`
		Generation           uint64 `json:"generation"`
		ProviderCount        int    `json:"provider_count"`
		ModelCount           int    `json:"model_count"`
		RouteCount           int    `json:"route_count"`
		LastReloadErrorClass string `json:"last_reload_error_class"`
		LastReloadError      string `json:"last_reload_error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("admin status malformed: %w", err)
	}
	if payload.SchemaVersion != 1 {
		return fmt.Errorf("admin status malformed: schema_version %d", payload.SchemaVersion)
	}
	fmt.Fprintf(w, "server: running\nversion: %s\ngeneration: %d\nproviders: %d\nmodels: %d\nroutes: %d\n", payload.Version, payload.Generation, payload.ProviderCount, payload.ModelCount, payload.RouteCount)
	if payload.LastReloadError != "" {
		fmt.Fprintf(w, "last_reload_error: %s\n", strings.TrimSpace(payload.LastReloadErrorClass+" "+payload.LastReloadError))
	}
	return nil
}

func pathOrDefault(path string) string {
	if path != "" {
		return path
	}
	return DefaultConfigPath()
}

func missingEnvRefs(cfg config.Config) []string {
	var missing []string
	for _, provider := range cfg.Providers {
		if strings.HasPrefix(provider.APIKey, "env:") {
			name := strings.TrimPrefix(provider.APIKey, "env:")
			if os.Getenv(name) == "" {
				missing = append(missing, name)
			}
		}
	}
	return missing
}

func portAvailable(host string, port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func serverReachable(cfg config.Config) bool {
	url := fmt.Sprintf("http://%s:%d/healthz", cfg.Server.Host, cfg.Server.Port)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

func reloadEndpointReachable(cfg config.Config) bool {
	url := fmt.Sprintf("http://%s:%d/internal/status", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	resp, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
