package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchModelsHandlerRequiresPOST(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := withSetupToken(store, handleFetchModels)
	req := httptest.NewRequest(http.MethodGet, "/internal/setup/fetch-models", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestFetchModelsHandlerAuthErrorPropagates(t *testing.T) {
	// Spin up a fake upstream that always 401s.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer upstream.Close()

	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := withSetupToken(store, handleFetchModels)
	body := strings.NewReader(`{"preset_id":"openai","base_url":"` + upstream.URL + `/v1","api_key":"bad"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/fetch-models", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401, body = %s", rr.Code, rr.Body.String())
	}
	var resp fetchModelsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.AuthError {
		t.Error("AuthError = false, want true")
	}
}

func TestFetchModelsHandlerSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-4o", "owned_by": "openai"}},
		})
	}))
	defer upstream.Close()

	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := withSetupToken(store, handleFetchModels)
	body := strings.NewReader(`{"preset_id":"openai","base_url":"` + upstream.URL + `/v1","api_key":"sk-test"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/fetch-models", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status = %d, want 200, body = %s", rr.Code, string(body))
	}
	var resp fetchModelsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Fetched == nil {
		t.Fatal("Fetched = nil")
	}
	if len(resp.Fetched.Models) != 1 {
		t.Fatalf("got %d models, want 1", len(resp.Fetched.Models))
	}
	if resp.Fetched.Models[0].ID != "gpt-4o" {
		t.Errorf("ID = %q, want gpt-4o", resp.Fetched.Models[0].ID)
	}
	if resp.Catalog == nil {
		t.Error("Catalog = nil; should always include the curated fallback")
	}
}

func TestFetchModelsHandlerMissingBaseURL(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := withSetupToken(store, handleFetchModels)
	body := strings.NewReader(`{"preset_id":"openai"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/fetch-models", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestFetchModelsHandlerResolvesEnv(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer resolved-env-key" {
			t.Errorf("Authorization = %q, want Bearer resolved-env-key", auth)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-4o", "owned_by": "openai"}},
		})
	}))
	defer upstream.Close()

	t.Setenv("TEST_CATALOG_HANDLER_ENV_KEY", "resolved-env-key")

	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := withSetupToken(store, handleFetchModels)
	body := strings.NewReader(`{"preset_id":"openai","base_url":"` + upstream.URL + `/v1","api_key":"env:TEST_CATALOG_HANDLER_ENV_KEY"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/fetch-models", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status = %d, want 200, body = %s", rr.Code, string(body))
	}
}

func TestCatalogHandler(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := withSetupToken(store, handleCatalogList)
	req := httptest.NewRequest(http.MethodGet, "/internal/setup/catalog", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		SchemaVersion int                       `json:"schema_version"`
		Providers     map[string]map[string]any `json:"providers"`
		IDs           []string                  `json:"ids"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", resp.SchemaVersion)
	}
	if len(resp.Providers) < 5 {
		t.Errorf("got %d providers, want >= 5", len(resp.Providers))
	}
	if _, ok := resp.Providers["anthropic"]; !ok {
		t.Error("anthropic missing from providers map")
	}
	if len(resp.IDs) == 0 {
		t.Error("IDs list is empty")
	}
}
