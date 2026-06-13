// Package httpserver provides small HTTP response helpers shared between
// arkroute's internal servers (panel, client adapters).
package httpserver

import (
	"encoding/json"
	"net/http"
)

// WriteJSON serializes value as JSON and writes it with the given status
// code. Encoding errors are ignored because by the time we discover them
// the response status has already been sent; callers cannot recover.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteError writes a JSON error envelope with the given status code.
// The body has the shape {"error": message}; callers that need a richer
// envelope should call WriteJSON directly.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}
