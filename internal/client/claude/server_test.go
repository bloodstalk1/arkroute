package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/router"
)

func TestModelsRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestModelsReturnsRouteAliases(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"sonnet", "claude-sonnet-4-20250514"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("models response missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestHealthz(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health body = %s", rec.Body.String())
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.MinimalValidConfig("local-key")
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	return NewServer(Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, router.NewHealthStore()),
	})
}
