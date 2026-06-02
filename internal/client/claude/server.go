package claude

import (
	"encoding/json"
	"net/http"

	"bat.dev/arkrouter/internal/config"
	arkruntime "bat.dev/arkrouter/internal/runtime"
)

type Deps struct {
	Snapshot config.Snapshot
	Executor *arkruntime.Executor
}

type Server struct {
	deps Deps
}

func NewServer(deps Deps) *Server {
	return &Server{deps: deps}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("/v1/messages", s.withAuth(s.handleMessages))
	mux.HandleFunc("/v1/messages/count_tokens", s.withAuth(s.handleCountTokens))
	mux.HandleFunc("/internal/status", s.withAuth(s.handleInternalStatus))
	mux.HandleFunc("/internal/config", s.withAuth(s.handleInternalConfig))
	mux.HandleFunc("/internal/routes", s.withAuth(s.handleInternalRoutes))
	mux.HandleFunc("/internal/health", s.withAuth(s.handleInternalHealth))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
