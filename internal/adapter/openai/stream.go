package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/protocol"
)

type StreamMapper struct {
	started bool
}

func NewStreamMapper() *StreamMapper {
	return &StreamMapper{}
}

func (m *StreamMapper) MapLine(line []byte) ([]protocol.StreamEvent, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil
	}
	if !bytes.HasPrefix(line, []byte("data:")) {
		return nil, nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(string(line), "data:"))
	if payload == "[DONE]" {
		return []protocol.StreamEvent{{Type: "message_stop"}}, nil
	}
	var chunk struct {
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, err
	}
	events := []protocol.StreamEvent{}
	if !m.started {
		m.started = true
		events = append(events, protocol.StreamEvent{Type: "message_start"})
		events = append(events, protocol.StreamEvent{Type: "content_block_start", Index: 0, Block: protocol.ContentBlock{Type: "text"}})
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			events = append(events, protocol.StreamEvent{Type: "content_delta", Index: choice.Index, Delta: choice.Delta.Content})
		}
		if choice.FinishReason != "" {
			events = append(events, protocol.StreamEvent{Type: "content_block_stop", Index: choice.Index})
			events = append(events, protocol.StreamEvent{Type: "message_delta"})
		}
	}
	return events, nil
}
