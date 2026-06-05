package claude

import (
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/buildinfo"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

const adminSchemaVersion = 1

func (s *Server) handleInternalStatus(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	snapshot := gen.Snapshot()
	status := s.deps.State.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":            adminSchemaVersion,
		"version":                   buildinfo.Version,
		"commit":                    buildinfo.Commit,
		"build_date":                buildinfo.BuildDate,
		"config_path":               status.ConfigPath,
		"generation":                gen.Number(),
		"config_loaded_at":          gen.LoadedAt(),
		"last_reload_attempt_at":    status.LastReloadAttemptAt,
		"last_successful_reload_at": status.LastSuccessfulReloadAt,
		"last_failed_reload_at":     status.LastFailedReloadAt,
		"last_reload_error_class":   status.LastReloadErrorClass,
		"last_reload_error":         status.LastReloadError,
		"reload_count":              status.ReloadCount,
		"failed_reload_count":       status.FailedReloadCount,
		"provider_count":            len(snapshot.ProvidersByID),
		"model_count":               len(snapshot.ModelsByID),
		"route_count":               len(snapshot.RoutesByAlias),
		"health":                    s.deps.State.Health().Snapshot(),
		"trace":                     s.deps.State.Trace().Stats(),
	})
}

func (s *Server) handleInternalConfig(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"generation":     gen.Number(),
		"config":         config.Redacted(gen.Snapshot().Config),
	})
}

func (s *Server) handleInternalRoutes(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"generation":     gen.Number(),
		"routes":         gen.Snapshot().RoutesByAlias,
	})
}

func (s *Server) handleInternalHealth(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"generation":     gen.Number(),
		"health":         s.deps.State.Health().Snapshot(),
	})
}

func (s *Server) handleInternalReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, anthropicError("method_not_allowed", "method not allowed"))
		return
	}

	result := s.deps.State.Reload(r.Context(), arkruntime.ReloadSourceAdmin, requestID(r))
	body := map[string]any{
		"schema_version":   adminSchemaVersion,
		"status":           result.Status,
		"generation":       result.Generation,
		"config_loaded_at": result.ConfigLoadedAt,
	}
	if result.Success {
		writeJSON(w, http.StatusOK, body)
		return
	}

	body["error_class"] = string(result.ErrorClass)
	body["error"] = result.Error
	status := http.StatusInternalServerError
	if result.ErrorClass == failure.ErrorConfigValidationFailed || result.ErrorClass == failure.ErrorListenerChangeRequiresRestart {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, body)
}
