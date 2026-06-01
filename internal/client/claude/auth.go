package claude

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.deps.Snapshot.Config.Server.ClientKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"type": "error",
				"error": map[string]string{
					"type":    "authentication_error",
					"message": "invalid local client key",
				},
			})
			return
		}
		next(w, r)
	}
}
