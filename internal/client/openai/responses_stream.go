package openai

import (
	"fmt"
	"net/http"
	"time"

	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func writeResponsesStream(w http.ResponseWriter, stream arkruntime.StreamResult, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	id := "resp_" + fmt.Sprint(time.Now().UnixNano())
	itemID := "msg_" + fmt.Sprint(time.Now().UnixNano())
	created := time.Now().Unix()
	sequence := 1
	textStarted := false

	writeNamedSSE(w, "response.created", map[string]any{
		"type":            "response.created",
		"sequence_number": sequence,
		"response": map[string]any{
			"id":                   id,
			"object":               "response",
			"created_at":           created,
			"status":               "in_progress",
			"model":                model,
			"output":               []any{},
			"output_text":          "",
			"parallel_tool_calls":  true,
			"previous_response_id": nil,
			"store":                false,
			"error":                nil,
			"incomplete_details":   nil,
			"usage":                nil,
		},
	})
	sequence++

	for event := range stream.Events {
		switch event.Type {
		case "content_delta":
			if !textStarted {
				writeNamedSSE(w, "response.output_item.added", map[string]any{
					"type":            "response.output_item.added",
					"sequence_number": sequence,
					"output_index":    0,
					"item": map[string]any{
						"id":      itemID,
						"type":    "message",
						"status":  "in_progress",
						"role":    "assistant",
						"content": []any{},
					},
				})
				sequence++
				textStarted = true
			}
			writeNamedSSE(w, "response.output_text.delta", map[string]any{
				"type":            "response.output_text.delta",
				"sequence_number": sequence,
				"item_id":         itemID,
				"output_index":    0,
				"content_index":   0,
				"delta":           event.Delta,
			})
			sequence++
		case "message_delta":
			writeNamedSSE(w, "response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": sequence,
				"output_index":    0,
				"item": map[string]any{
					"id":      itemID,
					"type":    "message",
					"status":  "completed",
					"role":    "assistant",
					"content": []any{},
				},
			})
			sequence++
		case "error":
			writeNamedSSE(w, "response.failed", map[string]any{
				"type":            "response.failed",
				"sequence_number": sequence,
				"response": map[string]any{
					"id":     id,
					"object": "response",
					"status": "failed",
					"model":  model,
					"error":  map[string]string{"message": event.Error, "type": "api_error"},
				},
			})
			return
		}
	}

	writeNamedSSE(w, "response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": sequence,
		"response": map[string]any{
			"id":                   id,
			"object":               "response",
			"created_at":           created,
			"status":               "completed",
			"model":                model,
			"output":               []any{},
			"output_text":          "",
			"parallel_tool_calls":  true,
			"previous_response_id": nil,
			"store":                false,
			"error":                nil,
			"incomplete_details":   nil,
			"usage":                nil,
		},
	})
}

func writeNamedSSE(w http.ResponseWriter, event string, payload any) {
	fmt.Fprintf(w, "event: %s\n", event)
	writeSSEData(w, payload)
}
