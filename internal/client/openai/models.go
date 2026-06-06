package openai

import (
	"net/http"
	"sort"
)

type modelListResponse struct {
	Object string              `json:"object"`
	Data   []openAIModelObject `json:"data"`
}

type openAIModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "", "method not allowed")
		return
	}
	gen := generationFromRequest(r)
	if gen == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "missing_generation", "", "missing runtime generation")
		return
	}
	snapshot := gen.Snapshot()
	ids := map[string]bool{}
	for alias := range snapshot.RoutesByAlias {
		ids[alias] = true
	}
	for alias := range snapshot.ModelsByExposedAlias {
		ids[alias] = true
	}
	models := make([]openAIModelObject, 0, len(ids))
	for _, id := range sortedModelIDs(ids) {
		models = append(models, modelObject(id))
	}
	writeJSON(w, http.StatusOK, modelListResponse{Object: "list", Data: models})
}

func sortedModelIDs(ids map[string]bool) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func modelObject(id string) openAIModelObject {
	return openAIModelObject{ID: id, Object: "model", Created: 0, OwnedBy: "arkroute"}
}
