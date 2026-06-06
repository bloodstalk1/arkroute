package openai

import (
	"encoding/json"
	"net/http"

	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type Deps struct {
	State *arkruntime.State
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
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
