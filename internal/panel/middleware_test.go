package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectIfNotMethodAllowsMatch(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if rejectIfNotMethod(rr, req, http.MethodPost) {
		t.Fatal("expected allowed request to return false")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want default 200", rr.Code)
	}
}

func TestRejectIfNotMethodRejectsOther(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if !rejectIfNotMethod(rr, req, http.MethodPost) {
		t.Fatal("expected rejected request to return true")
	}
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
	if got, want := rr.Header().Get("Allow"), http.MethodPost; got != want {
		t.Fatalf("Allow header = %q, want %q", got, want)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "method not allowed" {
		t.Fatalf("error = %v", body["error"])
	}
	if body["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v", body["schema_version"])
	}
}
