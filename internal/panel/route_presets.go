package panel

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/routepreset"
)

func handleRoutePresets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": 1, "presets": routepreset.Presets()})
	}
}

func handleRoutePresetApply(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		var input routepreset.ApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid route preset payload"})
			return
		}
		store := NewConfigStore(path)
		cfg, err := store.LoadOrBootstrap()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg, summary, err := routepreset.Apply(cfg, input)
		if err != nil {
			writeJSON(w, routePresetStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		result, err := store.SaveAndReload(cfg, onSave)
		if err != nil {
			writeJSON(w, httpStatusForSaveError(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"status":         "saved",
			"backup_path":    result.BackupPath,
			"summary":        summary,
			"config":         config.Redacted(cfg),
		})
	}
}

func routePresetStatus(err error) int {
	if errors.Is(err, routepreset.ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
