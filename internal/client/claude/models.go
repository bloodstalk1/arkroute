package claude

import (
	"net/http"
	"sort"

	aproto "github.com/bloodstalk1/arkroute/internal/protocol/anthropic"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	gen := s.deps.State.Current()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"loaded_at":  gen.LoadedAt(),
		"generation": gen.Number(),
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	snapshot := gen.Snapshot()
	entries := map[string]aproto.Model{}
	for _, model := range snapshot.ModelsByExposedAlias {
		entries[model.ExposedAlias] = aproto.Model{
			ID:            model.ExposedAlias,
			DisplayName:   model.DisplayName,
			ContextWindow: model.Capabilities.ContextWindow,
		}
		if model.ClaudeDiscoveryAlias != "" {
			entries[model.ClaudeDiscoveryAlias] = aproto.Model{
				ID:            model.ClaudeDiscoveryAlias,
				DisplayName:   model.DisplayName,
				ContextWindow: model.Capabilities.ContextWindow,
			}
		}
	}
	for _, route := range snapshot.RoutesByAlias {
		display := route.Alias
		context := 0
		if len(route.Targets) > 0 {
			if model, ok := snapshot.ModelsByID[route.Targets[0].ModelID]; ok {
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
