package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	openaiclient "github.com/bloodstalk1/arkroute/internal/client/openai"
	"github.com/bloodstalk1/arkroute/internal/clitools"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/panel"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type Deps struct {
	State                *arkruntime.State
	ConfigPath           string
	ClaudeSettingsWriter func(cfg config.Config) error
}

type Server struct {
	deps       Deps
	sessions   *panel.SessionStore
	configPath string
}

func NewServer(deps Deps) *Server {
	return &Server{deps: deps, sessions: panel.NewSessionStore(15 * time.Minute), configPath: deps.ConfigPath}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("/v1/messages", s.withAuth(s.handleMessages))
	mux.HandleFunc("/v1/messages/count_tokens", s.withAuth(s.handleCountTokens))
	openAIHandler := openaiclient.NewServer(openaiclient.Deps{State: s.deps.State}).Routes()
	mux.Handle("/v1/chat/completions", openAIHandler)
	mux.Handle("/v1/responses", openAIHandler)
	mux.HandleFunc("/internal/status", s.withAuth(s.handleInternalStatus))
	mux.HandleFunc("/internal/config", s.withAuth(s.handleInternalConfig))
	mux.HandleFunc("/internal/routes", s.withAuth(s.handleInternalRoutes))
	mux.HandleFunc("/internal/health", s.withAuth(s.handleInternalHealth))
	mux.HandleFunc("/internal/reload", s.withAuth(s.handleInternalReload))
	panelHandler := panel.Routes(panel.Deps{
		Sessions:             s.sessions,
		ConfigPath:           s.configPath,
		ClaudeSettingsWriter: s.deps.ClaudeSettingsWriter,
		CLITools:             clitools.NewService(s.configPath, true),
		OnSave: func() error {
			if s.deps.State != nil {
				result := s.deps.State.Reload(context.Background(), arkruntime.ReloadSourceAdmin, "panel_save")
				if !result.Success {
					return fmt.Errorf("%s: %s", result.ErrorClass, result.Error)
				}
			}
			return nil
		},
	})
	mux.Handle("/setup", panelHandler)
	mux.Handle("/panel", panelHandler)
	mux.Handle("/panel/assets/", panelHandler)
	mux.Handle("/internal/setup/options", panelHandler)
	mux.Handle("/internal/setup/provider", panelHandler)
	mux.Handle("/internal/setup/later", panelHandler)
	mux.Handle("/internal/setup/status", panelHandler)
	mux.Handle("/internal/config/export", panelHandler)
	mux.Handle("/internal/config/import/validate", panelHandler)
	mux.Handle("/internal/config/import/apply", panelHandler)
	mux.Handle("/internal/setup/logs", panelHandler)
	mux.Handle("/internal/cli-tools", panelHandler)
	mux.Handle("/internal/cli-tools/claude/launch", panelHandler)
	mux.Handle("/internal/policy/inspect", panelHandler)
	mux.HandleFunc("/internal/setup/session", s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, anthropicError("method_not_allowed", "method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": adminSchemaVersion,
			"setup_token":    s.sessions.Issue(),
		})
	}))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
