package anthropic

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/adapter"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

type StreamMapper struct{}

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
		Type         string          `json:"type"`
		Index        int             `json:"index"`
		ContentBlock streamBlock     `json:"content_block"`
		Delta        streamDelta     `json:"delta"`
		Usage        streamUsage     `json:"usage"`
		Error        json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, err
	}

	switch chunk.Type {
	case "message_start":
		return []protocol.StreamEvent{{Type: "message_start"}}, nil
	case "content_block_start":
		return []protocol.StreamEvent{{
			Type:  "content_block_start",
			Index: chunk.Index,
			Block: mapStreamContentBlock(chunk.ContentBlock),
		}}, nil
	case "content_block_delta":
		return mapStreamDelta(chunk.Index, chunk.Delta), nil
	case "content_block_stop":
		return []protocol.StreamEvent{{Type: "content_block_stop", Index: chunk.Index}}, nil
	case "message_delta":
		return []protocol.StreamEvent{{
			Type:       "message_delta",
			StopReason: chunk.Delta.StopReason,
			Usage:      protocol.Usage{OutputTokens: chunk.Usage.OutputTokens},
		}}, nil
	case "message_stop":
		return []protocol.StreamEvent{{Type: "message_stop"}}, nil
	case "error":
		return []protocol.StreamEvent{{Type: "error", Error: formatAnthropicStreamError(chunk.Error)}}, nil
	case "ping":
		return nil, nil
	default:
		return nil, nil
	}
}

type streamBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
}

type streamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	Signature   string `json:"signature"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}

type streamUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func mapStreamContentBlock(block streamBlock) protocol.ContentBlock {
	return protocol.ContentBlock{
		Type:      block.Type,
		Text:      block.Text,
		ID:        block.ID,
		Name:      block.Name,
		Input:     block.Input,
		Thinking:  block.Thinking,
		Signature: block.Signature,
	}
}

func mapStreamDelta(index int, delta streamDelta) []protocol.StreamEvent {
	switch delta.Type {
	case "text_delta":
		return []protocol.StreamEvent{{Type: "content_delta", Index: index, Delta: delta.Text}}
	case "thinking_delta":
		return []protocol.StreamEvent{{Type: "thinking_delta", Index: index, Delta: delta.Thinking}}
	case "signature_delta":
		return []protocol.StreamEvent{{Type: "signature_delta", Index: index, Delta: delta.Signature}}
	case "input_json_delta":
		return []protocol.StreamEvent{{Type: "tool_input_delta", Index: index, Delta: delta.PartialJSON}}
	default:
		return nil
	}
}

func formatAnthropicStreamError(raw json.RawMessage) string {
	return adapter.FormatStreamError(raw)
}
