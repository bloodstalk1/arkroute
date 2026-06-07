package openai

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func writeChatCompletionStream(w http.ResponseWriter, stream arkruntime.StreamResult, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	id := newOpenAIID("chatcmpl_")
	created := unixNow()
	for event := range stream.Events {
		done, err := writeChatStreamEvent(w, id, created, model, event)
		if err != nil {
			return
		}
		if done {
			return
		}
	}
	_ = writeSSEData(w, "[DONE]")
}

func writeChatStreamEvent(w http.ResponseWriter, id string, created int64, model string, event protocol.StreamEvent) (bool, error) {
	switch event.Type {
	case "message_start":
		return false, writeChatChunk(w, id, created, model, map[string]any{"role": "assistant"}, nil)
	case "content_delta":
		return false, writeChatChunk(w, id, created, model, map[string]any{"content": event.Delta}, nil)
	case "content_block_start":
		if event.Block.Type == "tool_use" {
			return false, writeChatChunk(w, id, created, model, map[string]any{
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
		return false, writeChatChunk(w, id, created, model, map[string]any{
			"tool_calls": []map[string]any{{
				"index": event.Index,
				"function": map[string]any{
					"arguments": event.Delta,
				},
			}},
		}, nil)
	case "message_delta":
		finishReason := mapStopReason(event.StopReason)
		return false, writeChatChunk(w, id, created, model, map[string]any{}, &finishReason)
	case "message_stop":
		return true, writeSSEData(w, "[DONE]")
	case "error":
		if err := writeSSEData(w, errorBody{Error: errorDetail{Message: event.Error, Type: "api_error", Code: "stream_error"}}); err != nil {
			return true, err
		}
		return true, writeSSEData(w, "[DONE]")
	}
	return false, nil
}

func writeChatChunk(w http.ResponseWriter, id string, created int64, model string, delta map[string]any, finishReason *string) error {
	choice := map[string]any{
		"index": 0,
		"delta": delta,
	}
	if finishReason != nil {
		choice["finish_reason"] = *finishReason
	}
	return writeSSEData(w, map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]any{choice},
	})
}

func writeSSEData(w http.ResponseWriter, payload any) error {
	if text, ok := payload.(string); ok {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", text); err != nil {
			return err
		}
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
