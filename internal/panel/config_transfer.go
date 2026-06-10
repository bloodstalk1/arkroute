package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type configImportRequest struct {
	YAML string `json:"yaml"`
}

type configImportSummary struct {
	Providers             int `json:"providers"`
	Models                int `json:"models"`
	Routes                int `json:"routes"`
	CompatibilityPolicies int `json:"compatibility_policies"`
}

func handleConfigExport(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		redacted := r.URL.Query().Get("redacted") == "1" || r.URL.Query().Get("redacted") == "true"
		data, err := NewConfigStore(path).Export(redacted)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		name := "arkroute-config.yaml"
		if redacted {
			name = "arkroute-config-redacted.yaml"
		}
		w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func handleConfigImportValidate(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		cfg, err := decodeImportConfig(r, NewConfigStore(path))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, importErrorResponse(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"valid":          true,
			"summary":        summarizeConfig(cfg),
			"config":         config.Redacted(cfg),
		})
	}
}

func handleConfigImportApply(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		store := NewConfigStore(path)
		cfg, err := decodeImportConfig(r, store)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, importErrorResponse(err))
			return
		}
		result, err := store.SaveAndReload(cfg, onSave)
		if err != nil {
			writeJSON(w, httpStatusForSaveError(err), importErrorResponse(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"status":         "saved",
			"backup_path":    result.BackupPath,
			"summary":        summarizeConfig(cfg),
			"config":         config.Redacted(cfg),
		})
	}
}

func decodeImportConfig(r *http.Request, store ConfigStore) (config.Config, error) {
	var input configImportRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		return config.Config{}, errors.New("invalid import payload")
	}
	if strings.TrimSpace(input.YAML) == "" {
		return config.Config{}, errors.New("yaml must be non-empty")
	}
	return store.ParseImport([]byte(input.YAML))
}

func summarizeConfig(cfg config.Config) configImportSummary {
	return configImportSummary{
		Providers:             len(cfg.Providers),
		Models:                len(cfg.Models),
		Routes:                len(cfg.Routes),
		CompatibilityPolicies: len(cfg.CompatibilityPolicies),
	}
}

func importErrorResponse(err error) map[string]any {
	response := map[string]any{
		"schema_version": 1,
		"valid":          false,
		"error":          err.Error(),
	}
	var validationErr config.ValidationError
	if errors.As(err, &validationErr) {
		response["fields"] = validationErr.Fields
	}
	return response
}
