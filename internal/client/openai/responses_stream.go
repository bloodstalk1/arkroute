package openai

import (
	"fmt"
	"net/http"
	"strings"

	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func writeResponsesStream(w http.ResponseWriter, stream arkruntime.StreamResult, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	id := newOpenAIID("resp_")
	itemID := newOpenAIID("msg_")
	created := unixNow()
	sequence := 1
	textStarted := false
	itemDone := false
	var outputText strings.Builder

	if err := writeNamedSSE(w, "response.created", map[string]any{
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
	}); err != nil {
		return
	}
	sequence++

	for event := range stream.Events {
		switch event.Type {
		case "content_delta":
			if itemDone {
				continue
			}
			if !textStarted {
				if err := writeNamedSSE(w, "response.output_item.added", map[string]any{
					"type":            "response.output_item.added",
					"sequence_number": sequence,
					"output_index":    0,
					"item": responseStreamMessageItem{
						ID:      itemID,
						Type:    "message",
						Status:  "in_progress",
						Role:    "assistant",
						Content: []responseContentPart{},
					},
				}); err != nil {
					return
				}
				sequence++
				textStarted = true
			}
			outputText.WriteString(event.Delta)
			if err := writeNamedSSE(w, "response.output_text.delta", map[string]any{
				"type":            "response.output_text.delta",
				"sequence_number": sequence,
				"item_id":         itemID,
				"output_index":    0,
				"content_index":   0,
				"delta":           event.Delta,
			}); err != nil {
				return
			}
			sequence++
		case "message_delta":
			if textStarted && !itemDone {
				if err := writeResponsesOutputItemDone(w, sequence, itemID, outputText.String()); err != nil {
					return
				}
				sequence++
				itemDone = true
			}
		case "error":
			_ = writeNamedSSE(w, "response.failed", map[string]any{
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

	text := outputText.String()
	if textStarted && !itemDone {
		if err := writeResponsesOutputItemDone(w, sequence, itemID, text); err != nil {
			return
		}
		sequence++
	}
	_ = writeNamedSSE(w, "response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": sequence,
		"response": map[string]any{
			"id":                   id,
			"object":               "response",
			"created_at":           created,
			"status":               "completed",
			"model":                model,
			"output":               responseStreamOutput(itemID, text),
			"output_text":          text,
			"parallel_tool_calls":  true,
			"previous_response_id": nil,
			"store":                false,
			"error":                nil,
			"incomplete_details":   nil,
			"usage":                nil,
		},
	})
}

func writeResponsesOutputItemDone(w http.ResponseWriter, sequence int, itemID string, text string) error {
	return writeNamedSSE(w, "response.output_item.done", map[string]any{
		"type":            "response.output_item.done",
		"sequence_number": sequence,
		"output_index":    0,
		"item": responseStreamMessageItem{
			ID:      itemID,
			Type:    "message",
			Status:  "completed",
			Role:    "assistant",
			Content: responseStreamContent(text),
		},
	})
}

func responseStreamOutput(itemID string, text string) []responseOutputItem {
	if text == "" {
		return []responseOutputItem{}
	}
	return []responseOutputItem{{
		ID:      itemID,
		Type:    "message",
		Status:  "completed",
		Role:    "assistant",
		Content: responseStreamContent(text),
	}}
}

func responseStreamContent(text string) []responseContentPart {
	if text == "" {
		return []responseContentPart{}
	}
	return []responseContentPart{{Type: "output_text", Text: text, Annotations: []any{}}}
}

type responseStreamMessageItem struct {
	ID      string                `json:"id"`
	Type    string                `json:"type"`
	Status  string                `json:"status"`
	Role    string                `json:"role"`
	Content []responseContentPart `json:"content"`
}

func writeNamedSSE(w http.ResponseWriter, event string, payload any) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	return writeSSEData(w, payload)
}
