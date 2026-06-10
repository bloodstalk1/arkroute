package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/provider/catalog"
	"github.com/bloodstalk1/arkroute/internal/provider/modelfetcher"
)

type fetchModelsRequest struct {
	PresetID string `json:"preset_id"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Protocol string `json:"protocol,omitempty"`
}

type fetchModelsResponse struct {
	SchemaVersion int                   `json:"schema_version"`
	Catalog       *catalog.Provider     `json:"catalog,omitempty"`
	Fetched       *modelfetcher.Fetched `json:"fetched,omitempty"`
	AuthError     bool                  `json:"auth_error,omitempty"`
	Error         string                `json:"error,omitempty"`
}

func handleFetchModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
		return
	}
	var input fetchModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, fetchModelsResponse{SchemaVersion: 1, Error: "invalid fetch payload"})
		return
	}
	if input.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, fetchModelsResponse{SchemaVersion: 1, Error: "base_url is required"})
		return
	}

	// Always return the catalog entry so the panel can fall back to curated
	// options when the live fetch fails or is skipped.
	cat, _ := catalog.Load()
	var entry *catalog.Provider
	if cat != nil {
		entry = cat.Get(input.PresetID)
	}

	ctx := r.Context()
	apiKey := input.APIKey
	if strings.HasPrefix(apiKey, "env:") {
		apiKey = os.Getenv(strings.TrimPrefix(apiKey, "env:"))
	}
	fetched, err := modelfetcher.Fetch(ctx, modelfetcher.Request{
		Provider: input.PresetID,
		BaseURL:  input.BaseURL,
		APIKey:   apiKey,
		Protocol: input.Protocol,
	})
	if err != nil {
		resp := fetchModelsResponse{SchemaVersion: 1, Catalog: entry, Error: err.Error()}
		if errors.Is(err, modelfetcher.ErrAuthRequired) {
			resp.AuthError = true
			writeJSON(w, http.StatusUnauthorized, resp)
			return
		}
		// Bad gateway: upstream reachable but not a valid model list.
		// Return catalog fallback so the user can still pick a curated model.
		writeJSON(w, http.StatusBadGateway, resp)
		return
	}
	writeJSON(w, http.StatusOK, fetchModelsResponse{SchemaVersion: 1, Catalog: entry, Fetched: fetched})
}

func handleCatalogList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
		return
	}
	cat, err := catalog.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	out := make(map[string]catalog.Provider, len(cat.Providers))
	for k, v := range cat.Providers {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": 1,
		"providers":      out,
		"ids":            cat.IDs(),
	})
}
