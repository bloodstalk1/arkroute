package openai

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	oaiproto "github.com/bloodstalk1/arkroute/internal/protocol/openai"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type responseObject struct {
	ID              string               `json:"id"`
	Object          string               `json:"object"`
	CreatedAt       int64                `json:"created_at"`
	Status          string               `json:"status"`
	Model           string               `json:"model"`
	Output          []responseOutputItem `json:"output"`
	OutputText      string               `json:"output_text"`
	Usage           responseUsage        `json:"usage"`
	Error           any                  `json:"error"`
	Incomplete      any                  `json:"incomplete_details"`
	Instructions    any                  `json:"instructions,omitempty"`
	PreviousID      any                  `json:"previous_response_id"`
	ParallelToolUse bool                 `json:"parallel_tool_calls"`
	Store           bool                 `json:"store"`
}

type responseOutputItem struct {
	ID        string                `json:"id"`
	Type      string                `json:"type"`
	Status    string                `json:"status,omitempty"`
	Role      string                `json:"role,omitempty"`
	Content   []responseContentPart `json:"content,omitempty"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments string                `json:"arguments,omitempty"`
}

type responseContentPart struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type responseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "", "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "read_failed", "", "read request failed")
		return
	}
	responsesReq, err := oaiproto.DecodeResponsesRequest(body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "invalid Responses request")
		return
	}
	normalized, requirements, err := oaiproto.NormalizeResponsesRequest(responsesReq)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "unsupported_feature", "", err.Error())
		return
	}
	gen := generationFromRequest(r)
	if gen == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "missing_generation", "", "missing runtime generation")
		return
	}
	if responsesReq.Stream {
		stream, err := gen.Stream(r.Context(), arkruntime.ExecuteRequest{
			RequestID:    requestID(r),
			Client:       "openai-responses",
			Model:        responsesReq.Model,
			Requirements: requirements,
			Request:      normalized,
		})
		if err != nil {
			writeExecutionError(w, err)
			return
		}
		defer stream.Close()
		writeResponsesStream(w, stream, responsesReq.Model)
		return
	}
	result, err := gen.Execute(r.Context(), arkruntime.ExecuteRequest{
		RequestID:    requestID(r),
		Client:       "openai-responses",
		Model:        responsesReq.Model,
		Requirements: requirements,
		Request:      normalized,
	})
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapResponseObject(result.Response, responsesReq.Model))
}

func mapResponseObject(resp protocol.Response, model string) responseObject {
	output, outputText := responseOutput(resp)
	return responseObject{
		ID:              responseIDWithPrefix(resp.ID, "resp_"),
		Object:          "response",
		CreatedAt:       time.Now().Unix(),
		Status:          "completed",
		Model:           model,
		Output:          output,
		OutputText:      outputText,
		Usage:           responseUsage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, TotalTokens: resp.Usage.InputTokens + resp.Usage.OutputTokens},
		Error:           nil,
		Incomplete:      nil,
		PreviousID:      nil,
		ParallelToolUse: true,
		Store:           false,
	}
}

func responseOutput(resp protocol.Response) ([]responseOutputItem, string) {
	items := []responseOutputItem{}
	var textParts []string
	var messageContent []responseContentPart
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
				messageContent = append(messageContent, responseContentPart{Type: "output_text", Text: block.Text, Annotations: []any{}})
			}
		case "tool_use":
			items = append(items, responseOutputItem{
				ID:        responseIDWithPrefix(block.ID, "fc_"),
				Type:      "function_call",
				CallID:    block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			})
		}
	}
	if len(messageContent) > 0 {
		items = append([]responseOutputItem{{
			ID:      responseIDWithPrefix(resp.ID, "msg_"),
			Type:    "message",
			Status:  "completed",
			Role:    "assistant",
			Content: messageContent,
		}}, items...)
	}
	return items, strings.Join(textParts, "\n")
}

func responseIDWithPrefix(id string, prefix string) string {
	if id != "" && strings.HasPrefix(id, prefix) {
		return id
	}
	if id != "" {
		return prefix + strings.TrimPrefix(id, "chatcmpl_")
	}
	return newOpenAIID(prefix)
}
