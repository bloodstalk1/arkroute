package anthropic

type Model struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Object        string `json:"object,omitempty"`
	DisplayName   string `json:"display_name"`
	ContextWindow int    `json:"context_window,omitempty"`
	Created       int64  `json:"created,omitempty"`
	OwnedBy       string `json:"owned_by,omitempty"`
}

type ModelsResponse struct {
	Object  string  `json:"object,omitempty"`
	Data    []Model `json:"data"`
	HasMore bool    `json:"has_more"`
	FirstID string  `json:"first_id,omitempty"`
	LastID  string  `json:"last_id,omitempty"`
}

func ModelsResponseFor(models []Model) ModelsResponse {
	firstID := ""
	lastID := ""
	if len(models) > 0 {
		firstID = models[0].ID
		lastID = models[len(models)-1].ID
	}
	for i := range models {
		models[i].Type = "model"
		models[i].Object = "model"
		models[i].OwnedBy = "arkroute"
	}
	return ModelsResponse{Object: "list", Data: models, HasMore: false, FirstID: firstID, LastID: lastID}
}
