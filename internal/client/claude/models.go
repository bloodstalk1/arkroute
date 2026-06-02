package claude

import (
	"net/http"
	"sort"

	aproto "github.com/bloodstalk1/arkroute/internal/protocol/anthropic"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"loaded_at": s.deps.Snapshot.LoadedAt,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	entries := map[string]aproto.Model{}
	for _, route := range s.deps.Snapshot.RoutesByAlias {
		display := route.Alias
		context := 0
		if len(route.Targets) > 0 {
			if model, ok := s.deps.Snapshot.ModelsByID[route.Targets[0].ModelID]; ok {
				display = model.DisplayName
				context = model.Capabilities.ContextWindow
			}
		}
		entries[route.Alias] = aproto.Model{ID: route.Alias, DisplayName: display, ContextWindow: context}
		if route.ClaudeDiscoveryAlias != "" {
			entries[route.ClaudeDiscoveryAlias] = aproto.Model{ID: route.ClaudeDiscoveryAlias, DisplayName: display, ContextWindow: context}
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	models := make([]aproto.Model, 0, len(keys))
	for _, key := range keys {
		models = append(models, entries[key])
	}
	writeJSON(w, http.StatusOK, aproto.ModelsResponseFor(models))
}
