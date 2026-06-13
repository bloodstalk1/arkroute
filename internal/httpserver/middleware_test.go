package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRequestIDIsUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		id := NewRequestID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q after %d generations", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestWithRequestIDGeneratesWhenMissing(t *testing.T) {
	called := false
	handler := WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		id := RequestIDFromContext(r.Context())
		if id == "" {
			t.Fatal("expected id in context")
		}
		if got := w.Header().Get(requestIDHeader); got != id {
			t.Fatalf("response header = %q, want %q", got, id)
		}
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("downstream handler not invoked")
	}
	if rr.Header().Get(requestIDHeader) == "" {
		t.Fatal("expected response to carry X-Request-Id")
	}
}

func TestWithRequestIDPreservesCallerHeader(t *testing.T) {
	const supplied = "req_caller_supplied_id"
	handler := WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != supplied {
			t.Fatalf("context id = %q, want %q", got, supplied)
		}
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeader, supplied)
	handler.ServeHTTP(rr, req)
	if got := rr.Header().Get(requestIDHeader); got != supplied {
		t.Fatalf("response header = %q, want %q", got, supplied)
	}
}

func TestRequestIDFromContextReturnsEmptyForPlainContext(t *testing.T) {
	//nolint:staticcheck // SA1012: the function explicitly handles nil context.
	if got := RequestIDFromContext(nil); got != "" {
		t.Fatalf("nil ctx -> %q, want \"\"", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := RequestIDFromContext(req.Context()); got != "" {
		t.Fatalf("plain ctx -> %q, want \"\"", got)
	}
}
