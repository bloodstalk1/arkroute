package openai

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type generationContextKey struct{}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gen := s.deps.State.Current()
		snapshot := gen.Snapshot()
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(snapshot.Config.Server.ClientKey)) != 1 {
			writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "", "invalid local client key")
			return
		}
		ctx := context.WithValue(r.Context(), generationContextKey{}, gen)
		next(w, r.WithContext(ctx))
	}
}

func generationFromRequest(r *http.Request) *arkruntime.Generation {
	if gen, ok := r.Context().Value(generationContextKey{}).(*arkruntime.Generation); ok && gen != nil {
		return gen
	}
	return nil
}
