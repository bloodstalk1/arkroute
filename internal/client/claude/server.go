package claude

import (
	"encoding/json"
	"net/http"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/router"
)

type Deps struct {
	Snapshot config.Snapshot
	Router   *router.Router
	Health   *router.HealthStore
}

type Server struct {
	deps Deps
}

func NewServer(deps Deps) *Server {
	if deps.Health == nil {
		deps.Health = router.NewHealthStore()
	}
	return &Server{deps: deps}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
