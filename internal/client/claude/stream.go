package claude

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func writeSSE(w http.ResponseWriter, event string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) writeStreamingResponse(w http.ResponseWriter, stream arkruntime.StreamResult, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	for event := range stream.Events {
		writeAnthropicStreamEvent(w, event, model)
	}
}

func writeAnthropicStreamEvent(w http.ResponseWriter, event protocol.StreamEvent, model string) {
	switch event.Type {
	case "message_start":
		writeSSE(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_stream", "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 0, "output_tokens": 0}}})
	case "content_block_start":
		contentBlock := map[string]any{"type": "text", "text": ""}
		if event.Block.Type == "tool_use" {
			contentBlock = map[string]any{"type": "tool_use", "id": event.Block.ID, "name": event.Block.Name, "input": map[string]any{}}
		}
		if event.Block.Type == "thinking" {
			contentBlock = map[string]any{"type": "thinking", "thinking": ""}
			if event.Block.Signature != "" {
				contentBlock["signature"] = event.Block.Signature
			}
		}
		writeSSE(w, "content_block_start", map[string]any{"type": "content_block_start", "index": event.Index, "content_block": contentBlock})
	case "content_delta":
		writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": event.Index, "delta": map[string]any{"type": "text_delta", "text": event.Delta}})
	case "thinking_delta":
		writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": event.Index, "delta": map[string]any{"type": "thinking_delta", "thinking": event.Delta}})
	case "signature_delta":
		writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": event.Index, "delta": map[string]any{"type": "signature_delta", "signature": event.Delta}})
	case "tool_input_delta":
		writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": event.Index, "delta": map[string]any{"type": "input_json_delta", "partial_json": event.Delta}})
	case "content_block_stop":
		writeSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": event.Index})
	case "message_delta":
		stopReason := event.StopReason
		if stopReason == "" {
			stopReason = "end_turn"
		}
		writeSSE(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 0}})
	case "message_stop":
		writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
	case "error":
		writeSSE(w, "error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": event.Error}})
	}
}
