package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func writeChatCompletionStream(w http.ResponseWriter, stream arkruntime.StreamResult, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	id := "chatcmpl_" + fmt.Sprint(time.Now().UnixNano())
	created := time.Now().Unix()
	doneSent := false
	for event := range stream.Events {
		if writeChatStreamEvent(w, id, created, model, event) {
			doneSent = true
		}
	}
	if !doneSent {
		writeSSEData(w, "[DONE]")
	}
}

func writeChatStreamEvent(w http.ResponseWriter, id string, created int64, model string, event protocol.StreamEvent) bool {
	switch event.Type {
	case "message_start":
		writeChatChunk(w, id, created, model, map[string]any{"role": "assistant"}, nil)
	case "content_delta":
		writeChatChunk(w, id, created, model, map[string]any{"content": event.Delta}, nil)
	case "content_block_start":
		if event.Block.Type == "tool_use" {
			writeChatChunk(w, id, created, model, map[string]any{
				"tool_calls": []map[string]any{{
					"index": event.Index,
					"id":    event.Block.ID,
					"type":  "function",
					"function": map[string]any{
						"name":      event.Block.Name,
						"arguments": "",
					},
				}},
			}, nil)
		}
	case "tool_input_delta":
		writeChatChunk(w, id, created, model, map[string]any{
			"tool_calls": []map[string]any{{
				"index": event.Index,
				"function": map[string]any{
					"arguments": event.Delta,
				},
			}},
		}, nil)
	case "message_delta":
		finishReason := mapStopReason(event.StopReason)
		writeChatChunk(w, id, created, model, map[string]any{}, &finishReason)
	case "message_stop":
		writeSSEData(w, "[DONE]")
		return true
	case "error":
		writeSSEData(w, errorBody{Error: errorDetail{Message: event.Error, Type: "api_error", Code: "stream_error"}})
		writeSSEData(w, "[DONE]")
		return true
	}
	return false
}

func writeChatChunk(w http.ResponseWriter, id string, created int64, model string, delta map[string]any, finishReason *string) {
	choice := map[string]any{
		"index": 0,
		"delta": delta,
	}
	if finishReason != nil {
		choice["finish_reason"] = *finishReason
	}
	writeSSEData(w, map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]any{choice},
	})
}

func writeSSEData(w http.ResponseWriter, payload any) {
	if text, ok := payload.(string); ok {
		fmt.Fprintf(w, "data: %s\n\n", text)
	} else {
		data, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
