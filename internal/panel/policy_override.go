package panel

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/policyedit"
	"github.com/bloodstalk1/arkroute/internal/policyinspect"
)

func handlePolicyOverride(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			savePolicyOverride(w, r, path, onSave)
		case http.MethodDelete:
			deletePolicyOverride(w, r, path, onSave)
		default:
			w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
		}
	}
}

func savePolicyOverride(w http.ResponseWriter, r *http.Request, path string, onSave func() error) {
	var input policyedit.OverrideInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid policy override payload"})
		return
	}
	store := NewConfigStore(path)
	cfg, err := store.LoadOrBootstrap()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	cfg, policy, err := policyedit.UpsertModelOverride(cfg, input)
	if err != nil {
		writeJSON(w, policyEditStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	result, err := store.Save(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	if err := reloadAfterPanelSave(onSave); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	inspection, err := policyinspect.InspectModel(cfg, input.ModelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": 1,
		"status":         "saved",
		"policy_id":      policy.ID,
		"backup_path":    result.BackupPath,
		"inspection":     inspection,
		"config":         config.Redacted(cfg),
	})
}

func deletePolicyOverride(w http.ResponseWriter, r *http.Request, path string, onSave func() error) {
	modelID := r.URL.Query().Get("model_id")
	store := NewConfigStore(path)
	cfg, err := store.LoadOrBootstrap()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	cfg, policyID, err := policyedit.DeleteModelOverride(cfg, modelID)
	if err != nil {
		writeJSON(w, policyEditStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	result, err := store.Save(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	if err := reloadAfterPanelSave(onSave); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	inspection, err := policyinspect.InspectModel(cfg, modelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": 1,
		"status":         "deleted",
		"policy_id":      policyID,
		"backup_path":    result.BackupPath,
		"inspection":     inspection,
		"config":         config.Redacted(cfg),
	})
}

func reloadAfterPanelSave(onSave func() error) error {
	if onSave == nil {
		return nil
	}
	if err := onSave(); err != nil {
		return errors.New("reload failed: " + err.Error())
	}
	return nil
}

func policyEditStatus(err error) int {
	if errors.Is(err, policyedit.ErrModelNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
