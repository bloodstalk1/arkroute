package claude

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"bat.dev/arkroute/internal/protocol"
	aproto "bat.dev/arkroute/internal/protocol/anthropic"
	"bat.dev/arkroute/internal/router"
	arkruntime "bat.dev/arkroute/internal/runtime"
)

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "read request failed"))
		return
	}
	anthropicReq, err := aproto.DecodeMessageRequest(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "invalid Anthropic request"))
		return
	}
	normalized := protocol.Request{
		Model:     anthropicReq.Model,
		MaxTokens: anthropicReq.MaxTokens,
		Stream:    anthropicReq.Stream,
		Tools:     mapAnthropicTools(anthropicReq.Tools),
		Messages:  mapAnthropicMessages(anthropicReq.Messages),
	}
	if anthropicReq.Stream {
		stream, err := s.deps.Executor.Stream(r.Context(), arkruntime.ExecuteRequest{
			RequestID:    requestID(r),
			Client:       "claude",
			Model:        anthropicReq.Model,
			Requirements: router.Requirements{Streaming: true, Tools: len(anthropicReq.Tools) > 0},
			Request:      normalized,
		})
		if err != nil {
			writeExecutionError(w, err)
			return
		}
		defer stream.Close()
		s.writeStreamingResponse(w, stream, stream.Target.Model.ExposedAlias)
		return
	}
	result, err := s.deps.Executor.Execute(r.Context(), arkruntime.ExecuteRequest{
		RequestID:    requestID(r),
		Client:       "claude",
		Model:        anthropicReq.Model,
		Requirements: router.Requirements{Streaming: anthropicReq.Stream, Tools: len(anthropicReq.Tools) > 0},
		Request:      normalized,
	})
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapNormalizedResponse(result.Response, result.Target.Model.ExposedAlias))
}

func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "read request failed"))
		return
	}
	req, err := aproto.DecodeMessageRequest(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "invalid Anthropic request"))
		return
	}
	if _, err := s.deps.Executor.Router.Plan(req.Model, router.Requirements{Tools: len(req.Tools) > 0}); err != nil {
		writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"input_tokens": estimateInputTokens(body)})
}

func requestID(r *http.Request) string {
	if value := r.Header.Get("x-request-id"); value != "" {
		return value
	}
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func writeExecutionError(w http.ResponseWriter, err error) {
	var execErr *arkruntime.ExecutionError
	if arkruntime.AsExecutionError(err, &execErr) {
		status := http.StatusBadGateway
		if execErr.Class == arkruntime.ErrorRouteNotFound {
			status = http.StatusNotFound
		}
		if execErr.Class == arkruntime.ErrorInvalidRequest || execErr.Class == arkruntime.ErrorUnsupportedCapability {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, anthropicError(string(execErr.Class), execErr.Message))
		return
	}
	writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
}

func estimateInputTokens(body []byte) int {
	return (len(body)*2+4)/5 + 32
}

func anthropicError(errorType string, message string) map[string]any {
	return map[string]any{"type": "error", "error": map[string]string{"type": errorType, "message": message}}
}

func mapAnthropicTools(tools []aproto.Tool) []protocol.Tool {
	out := make([]protocol.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, protocol.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return out
}

func mapAnthropicMessages(messages []aproto.Message) []protocol.Message {
	out := make([]protocol.Message, 0, len(messages))
	for _, msg := range messages {
		var blocks []aproto.ContentBlock
		_ = json.Unmarshal(msg.Content, &blocks)
		content := make([]protocol.ContentBlock, 0, len(blocks))
		for _, block := range blocks {
			content = append(content, protocol.ContentBlock{Type: block.Type, Text: block.Text})
		}
		out = append(out, protocol.Message{Role: protocol.Role(msg.Role), Content: content})
	}
	return out
}

func mapNormalizedResponse(resp protocol.Response, model string) map[string]any {
	content := make([]map[string]any, 0, len(resp.Content))
	for _, block := range resp.Content {
		if block.Type == "text" {
			content = append(content, map[string]any{"type": "text", "text": block.Text})
		}
		if block.Type == "tool_use" {
			content = append(content, map[string]any{"type": "tool_use", "id": block.ID, "name": block.Name, "input": block.Input})
		}
	}
	return map[string]any{
		"id":            resp.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   resp.StopReason,
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		},
	}
}
