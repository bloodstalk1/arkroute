package claude

import (
	"net/http"

	"bat.dev/arkroute/internal/buildinfo"
	"bat.dev/arkroute/internal/config"
)

const adminSchemaVersion = 1

func (s *Server) handleInternalStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := s.deps.Snapshot
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":   adminSchemaVersion,
		"version":          buildinfo.Version,
		"commit":           buildinfo.Commit,
		"build_date":       buildinfo.BuildDate,
		"config_loaded_at": snapshot.LoadedAt,
		"provider_count":   len(snapshot.ProvidersByID),
		"model_count":      len(snapshot.ModelsByID),
		"route_count":      len(snapshot.RoutesByAlias),
		"health":           s.deps.Executor.Health.Snapshot(),
		"trace":            s.deps.Executor.Trace.Stats(),
	})
}

func (s *Server) handleInternalConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"config":         config.Redacted(s.deps.Snapshot.Config),
	})
}

func (s *Server) handleInternalRoutes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"routes":         s.deps.Snapshot.RoutesByAlias,
	})
}

func (s *Server) handleInternalHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": adminSchemaVersion,
		"health":         s.deps.Executor.Health.Snapshot(),
	})
}
