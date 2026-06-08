package panel

import (
	"embed"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
	setupcore "github.com/bloodstalk1/arkroute/internal/setup"
)

//go:embed assets/*
var assets embed.FS

type Deps struct {
	Sessions             *SessionStore
	ConfigPath           string
	ClaudeSettingsWriter func(cfg config.Config) error
	OnSave               func() error
	CLITools             CLIToolsService
}

func Routes(deps Deps) http.Handler {
	if deps.Sessions == nil {
		deps.Sessions = NewSessionStore(15 * time.Minute)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/setup", servePanelHTML)
	mux.HandleFunc("/panel", servePanelHTML)
	mux.Handle("/panel/assets/", http.StripPrefix("/panel/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/internal/setup/options", withSetupToken(deps.Sessions, handleOptions))
	mux.HandleFunc("/internal/setup/provider", withSetupToken(deps.Sessions, handleProvider(deps.ConfigPath, deps.ClaudeSettingsWriter, deps.OnSave)))
	mux.HandleFunc("/internal/setup/later", withSetupToken(deps.Sessions, handleLater(deps.ConfigPath, deps.OnSave)))
	mux.HandleFunc("/internal/setup/status", withSetupToken(deps.Sessions, handleGetStatus(deps.ConfigPath)))
	mux.HandleFunc("/internal/config/export", withSetupToken(deps.Sessions, handleConfigExport(deps.ConfigPath)))
	mux.HandleFunc("/internal/config/import/validate", withSetupToken(deps.Sessions, handleConfigImportValidate(deps.ConfigPath)))
	mux.HandleFunc("/internal/config/import/apply", withSetupToken(deps.Sessions, handleConfigImportApply(deps.ConfigPath, deps.OnSave)))
	mux.HandleFunc("/internal/setup/logs", withSetupToken(deps.Sessions, handleGetLogs()))
	mux.HandleFunc("/internal/cli-tools", withSetupToken(deps.Sessions, handleCLIToolsStatus(deps.CLITools)))
	mux.HandleFunc("/internal/cli-tools/claude/launch", withSetupToken(deps.Sessions, handleClaudeLaunch(deps.CLITools)))
	mux.HandleFunc("/internal/policy/inspect", withSetupToken(deps.Sessions, handlePolicyInspect(deps.ConfigPath)))
	mux.HandleFunc("/internal/policy/override", withSetupToken(deps.Sessions, handlePolicyOverride(deps.ConfigPath, deps.OnSave)))
	mux.HandleFunc("/internal/cli-context", withSetupToken(deps.Sessions, handleCLIContext(deps.ConfigPath)))
	mux.HandleFunc("/internal/route-presets", withSetupToken(deps.Sessions, handleRoutePresets()))
	mux.HandleFunc("/internal/route-presets/apply", withSetupToken(deps.Sessions, handleRoutePresetApply(deps.ConfigPath, deps.OnSave)))
	return mux
}

func servePanelHTML(w http.ResponseWriter, r *http.Request) {
	data, err := assets.ReadFile("assets/panel.html")
	if err != nil {
		http.Error(w, "panel asset missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func withSetupToken(store *SessionStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !store.Valid(r.Header.Get("X-Arkroute-Setup-Token")) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"schema_version": 1, "error": "invalid setup token"})
			return
		}
		next(w, r)
	}
}

func handleOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "presets": setupcore.Presets()})
}

func handleLater(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		key, err := security.GenerateClientKey()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg := config.BootstrapLocalConfig(key)
		if err := savePanelConfig(path, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if onSave != nil {
			if err := onSave(); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "reload failed: " + err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "status": "saved", "config": config.Redacted(cfg)})
	}
}

func handleProvider(path string, claudeWriter func(config.Config) error, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		var input setupcore.ProviderSetup
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid setup payload"})
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg, err = setupcore.ApplyProviderSetup(cfg, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if err := savePanelConfig(path, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if onSave != nil {
			if err := onSave(); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "reload failed: " + err.Error()})
				return
			}
		}
		claudeActivated := false
		var claudeErr string
		if input.ActivateClaude && claudeWriter != nil {
			if err := claudeWriter(cfg); err == nil {
				claudeActivated = true
			} else {
				claudeErr = err.Error()
			}
		}
		response := map[string]any{
			"schema_version":   1,
			"status":           "saved",
			"claude_activated": claudeActivated,
			"config":           config.Redacted(cfg),
		}
		if claudeErr != "" {
			response["claude_error"] = claudeErr
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func loadOrBootstrapConfig(path string) (config.Config, error) {
	return NewConfigStore(path).LoadOrBootstrap()
}

func savePanelConfig(path string, cfg config.Config) error {
	_, err := NewConfigStore(path).Save(cfg)
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func defaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkroute/traces.jsonl"
	}
	return filepath.Join(home, ".arkroute", "traces.jsonl")
}

func handleGetStatus(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"config":         config.Redacted(cfg),
		})
	}
}

func handleGetLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := defaultLogPath()
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "logs": []any{}})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		lines := strings.Split(string(data), "\n")
		var logs []map[string]any
		start := 0
		if len(lines) > 50 {
			start = len(lines) - 50
		}
		for i := start; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var logEntry map[string]any
			if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
				logs = append(logs, logEntry)
			} else {
				logs = append(logs, map[string]any{"msg": line})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "logs": logs})
	}
}
