package claude

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	aproto "github.com/bloodstalk1/arkroute/internal/protocol/anthropic"
	"github.com/bloodstalk1/arkroute/internal/router"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
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
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	normalized := protocol.Request{
		Model:           anthropicReq.Model,
		MaxTokens:       anthropicReq.MaxTokens,
		Stream:          anthropicReq.Stream,
		Tools:           mapAnthropicTools(anthropicReq.Tools),
		System:          mapAnthropicSystem(anthropicReq.System),
		ToolChoice:      decodeToolChoice(anthropicReq.ToolChoice),
		Temperature:     anthropicReq.Temperature,
		Messages:        mapAnthropicMessages(anthropicReq.Messages),
		Thinking:        mapAnthropicThinking(anthropicReq.Thinking),
		ReasoningEffort: mapAnthropicOutputEffort(anthropicReq.OutputConfig),
	}
	requirements := router.Requirements{Streaming: anthropicReq.Stream, Tools: len(anthropicReq.Tools) > 0}
	if bypass, ok := bypassClaudeHousekeeping(normalized); ok {
		if _, err := gen.Plan(anthropicReq.Model, requirements); err != nil {
			writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, mapNormalizedResponse(bypass, anthropicReq.Model))
		return
	}
	if anthropicReq.Stream {
		stream, err := gen.Stream(r.Context(), arkruntime.ExecuteRequest{
			RequestID:    requestID(r),
			Client:       "claude",
			Model:        anthropicReq.Model,
			Requirements: requirements,
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
	result, err := gen.Execute(r.Context(), arkruntime.ExecuteRequest{
		RequestID:    requestID(r),
		Client:       "claude",
		Model:        anthropicReq.Model,
		Requirements: requirements,
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
	gen := generationFromRequest(r)
	if gen == nil {
		writeJSON(w, http.StatusInternalServerError, anthropicError("api_error", "missing runtime generation"))
		return
	}
	if _, err := gen.Plan(req.Model, router.Requirements{Tools: len(req.Tools) > 0}); err != nil {
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
		if execErr.Class == arkruntime.ErrorUpstreamRateLimit {
			status = http.StatusTooManyRequests
		}
		if execErr.Class == arkruntime.ErrorUpstreamAuth {
			status = http.StatusForbidden
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

func bypassClaudeHousekeeping(req protocol.Request) (protocol.Response, bool) {
	if req.Stream || len(req.Tools) > 0 || req.MaxTokens > 128 {
		return protocol.Response{}, false
	}
	text := firstUserText(req.Messages)
	if text == "" {
		return protocol.Response{}, false
	}
	lower := strings.ToLower(text)
	switch {
	case looksLikeClaudeTitleRequest(lower):
		return housekeepingResponse("ArkRoute Session"), true
	case looksLikeClaudeWarmupRequest(lower):
		return housekeepingResponse("ok"), true
	default:
		return protocol.Response{}, false
	}
}

func firstUserText(messages []protocol.Message) string {
	for _, msg := range messages {
		if msg.Role != protocol.RoleUser {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				return strings.TrimSpace(block.Text)
			}
		}
	}
	return ""
}

func looksLikeClaudeTitleRequest(text string) bool {
	return strings.Contains(text, "generate") &&
		strings.Contains(text, "title") &&
		strings.Contains(text, "conversation") &&
		strings.Contains(text, "concise") &&
		strings.Contains(text, "return only")
}

func looksLikeClaudeWarmupRequest(text string) bool {
	return (strings.Contains(text, "warmup request") || strings.Contains(text, "warm-up request")) &&
		strings.Contains(text, "respond") &&
		strings.Contains(text, "only") &&
		strings.Contains(text, "ok")
}

func housekeepingResponse(text string) protocol.Response {
	return protocol.Response{
		ID:         "msg_arkroute_bypass",
		Role:       protocol.RoleAssistant,
		Content:    []protocol.ContentBlock{{Type: "text", Text: text}},
		StopReason: "end_turn",
		Usage:      protocol.Usage{InputTokens: 1, OutputTokens: 1},
	}
}

func mapAnthropicTools(tools []aproto.Tool) []protocol.Tool {
	out := make([]protocol.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, protocol.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return out
}

func mapAnthropicThinking(thinking *aproto.ThinkingConfig) protocol.ThinkingConfig {
	if thinking == nil {
		return protocol.ThinkingConfig{}
	}
	return protocol.ThinkingConfig{Type: thinking.Type, BudgetTokens: thinking.BudgetTokens}
}

func mapAnthropicOutputEffort(output *aproto.OutputConfig) string {
	if output == nil {
		return ""
	}
	return output.Effort
}

func mapAnthropicMessages(messages []aproto.Message) []protocol.Message {
	out := make([]protocol.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, protocol.Message{Role: protocol.Role(msg.Role), Content: mapAnthropicContent(msg.Content)})
	}
	return out
}

func mapAnthropicContent(raw json.RawMessage) []protocol.ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return []protocol.ContentBlock{{Type: "text", Text: text}}
		}
		return nil
	}
	var blocks []aproto.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	content := make([]protocol.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		content = append(content, protocol.ContentBlock{Type: block.Type, Text: block.Text, ID: block.ID, Name: block.Name, Input: block.Input, ToolUseID: block.ToolUseID, Content: block.Content, Thinking: block.Thinking, Signature: block.Signature})
	}
	return content
}

func mapNormalizedResponse(resp protocol.Response, model string) map[string]any {
	content := make([]map[string]any, 0, len(resp.Content))
	for _, block := range resp.Content {
		if block.Type == "thinking" {
			thinking := map[string]any{"type": "thinking", "thinking": block.Thinking}
			if block.Signature != "" {
				thinking["signature"] = block.Signature
			}
			content = append(content, thinking)
		}
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

func mapAnthropicSystem(raw json.RawMessage) []protocol.ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return []protocol.ContentBlock{{Type: "text", Text: text}}
		}
		return nil
	}
	var blocks []protocol.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	return nil
}

func decodeToolChoice(raw json.RawMessage) json.RawMessage {
	return raw
}
