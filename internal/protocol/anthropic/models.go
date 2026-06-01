package anthropic

type Model struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	DisplayName   string `json:"display_name"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type ModelsResponse struct {
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
	}
	return ModelsResponse{Data: models, HasMore: false, FirstID: firstID, LastID: lastID}
}
