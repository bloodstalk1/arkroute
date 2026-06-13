package openai

import (
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/httpserver"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
	"github.com/bloodstalk1/arkroute/internal/security/ratelimit"
)

type Deps struct {
	State       *arkruntime.State
	RateLimiter *ratelimit.Store
}

type Server struct {
	deps Deps
}

func NewServer(deps Deps) *Server {
	return &Server{deps: deps}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("/v1/chat/completions", s.withAuth(s.handleChatCompletions))
	mux.HandleFunc("/v1/responses", s.withAuth(s.handleResponses))
	return httpserver.WithRequestID(mux)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	httpserver.WriteJSON(w, status, value)
}
