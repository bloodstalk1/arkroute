package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	openaiadapter "bat.dev/arkrouter/internal/adapter/openai"
	"bat.dev/arkrouter/internal/protocol"
	aproto "bat.dev/arkrouter/internal/protocol/anthropic"
	"bat.dev/arkrouter/internal/router"
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
	targets, err := s.deps.Router.Resolve(anthropicReq.Model, router.Requirements{Streaming: anthropicReq.Stream, Tools: len(anthropicReq.Tools) > 0})
	if err != nil {
		writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
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
		target := targets[0]
		adapter := openaiadapter.Adapter{}
		upstreamReq, err := adapter.BuildRequest(normalized, target.Provider, target.Model)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
			return
		}
		client := &http.Client{Timeout: time.Duration(s.deps.Snapshot.Config.Server.UpstreamTimeoutSeconds) * time.Second}
		httpReq, err := http.NewRequestWithContext(r.Context(), upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
			return
		}
		httpReq.Header = upstreamReq.Headers.Clone()
		upstreamResp, err := client.Do(httpReq)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
			return
		}
		defer upstreamResp.Body.Close()
		s.handleStreamingResponse(w, upstreamResp, target.Model.ExposedAlias)
		return
	}
	for i, target := range targets {
		resp, status, err := s.callTarget(r, normalized, target)
		if err == nil {
			s.deps.Health.Set(target.Provider.ID, "ok")
			writeJSON(w, http.StatusOK, mapNormalizedResponse(resp, target.Model.ExposedAlias))
			return
		}
		if !router.IsRetryableStatus(status) || i == len(targets)-1 {
			s.deps.Health.Set(target.Provider.ID, "unhealthy")
			writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
			return
		}
		s.deps.Health.Set(target.Provider.ID, "degraded")
	}
}

func (s *Server) callTarget(r *http.Request, req protocol.Request, target router.Target) (protocol.Response, int, error) {
	adapter := openaiadapter.Adapter{}
	upstreamReq, err := adapter.BuildRequest(req, target.Provider, target.Model)
	if err != nil {
		return protocol.Response{}, 0, err
	}
	client := &http.Client{Timeout: time.Duration(s.deps.Snapshot.Config.Server.UpstreamTimeoutSeconds) * time.Second}
	httpReq, err := http.NewRequestWithContext(r.Context(), upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
	if err != nil {
		return protocol.Response{}, 0, err
	}
	httpReq.Header = upstreamReq.Headers.Clone()
	upstreamResp, err := client.Do(httpReq)
	if err != nil {
		return protocol.Response{}, 0, err
	}
	defer upstreamResp.Body.Close()
	upstreamBody, _ := io.ReadAll(upstreamResp.Body)
	if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
		return protocol.Response{}, upstreamResp.StatusCode, fmt.Errorf("upstream returned %d", upstreamResp.StatusCode)
	}
	mapped, err := adapter.MapResponse(upstreamBody)
	if err != nil {
		return protocol.Response{}, 0, err
	}
	return mapped, upstreamResp.StatusCode, nil
}

func (s *Server) handleStreamingResponse(w http.ResponseWriter, upstreamResp *http.Response, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(upstreamResp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	mapper := openaiadapter.NewStreamMapper()
	for scanner.Scan() {
		events, err := mapper.MapLine(scanner.Bytes())
		if err != nil {
			return
		}
		for _, event := range events {
			writeAnthropicStreamEvent(w, event, model)
		}
	}
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
	if _, err := s.deps.Router.Resolve(req.Model, router.Requirements{Tools: len(req.Tools) > 0}); err != nil {
		writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"input_tokens": estimateInputTokens(body)})
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
