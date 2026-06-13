package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// requestIDHeader is the canonical header name used for request
// correlation across arkroute's HTTP surface.
const requestIDHeader = "X-Request-Id"

// ctxKey is unexported so callers must use [RequestIDFromContext].
type ctxKey struct{}

// NewRequestID returns a 16-byte, URL-safe, base64-encoded random
// identifier with the "req_" prefix. crypto/rand errors are treated as
// unrecoverable: the caller would be operating in an environment where
// every other security primitive is also unsafe, so panicking is the
// honest response.
func NewRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("httpserver: crypto/rand.Read failed: " + err.Error())
	}
	return "req_" + base64.RawURLEncoding.EncodeToString(raw[:])
}

// RequestIDFromContext returns the request ID stored on ctx by
// [WithRequestID], or "" if none is set.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// WithRequestID returns a middleware that ensures every request has an
// X-Request-Id. If the inbound request does not carry one, a fresh ID
// is generated with [NewRequestID]. The chosen ID is:
//   - echoed back to the caller on the response's X-Request-Id header
//   - stored on the request context so handlers can retrieve it via
//     [RequestIDFromContext]
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = NewRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), ctxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
