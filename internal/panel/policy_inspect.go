package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/policyinspect"
)

func handlePolicyInspect(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectIfNotMethod(w, r, http.MethodGet) {
			return
		}
		modelID := strings.TrimSpace(r.URL.Query().Get("model_id"))
		if modelID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "model_id is required"})
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		inspection, err := policyinspect.InspectModel(cfg, modelID)
		if errors.Is(err, policyinspect.ErrModelNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if errors.Is(err, policyinspect.ErrProviderNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, inspection)
	}
}
