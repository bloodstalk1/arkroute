package gemini

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/protocol"
)

// StreamMapper translates Gemini SSE events to protocol.StreamEvent.
//
// Unlike OpenAI streaming which sends deltas, Gemini's streamGenerateContent
// sends the FULL accumulated state in each SSE event. We diff consecutive
// responses to produce deltas that look like Anthropic streaming events:
//
//	{text: "He"}   +   {text: "Hello"}   →   delta: "lo"
//	{text: "Hel"}  +   {text: "Hello wo"} →   delta: "wo"
//
// Tool calls and multi-part responses are not yet handled; the mapper
// currently emits a single text stream.
type StreamMapper struct {
	started  bool
	prevText string
}

func NewStreamMapper() *StreamMapper {
	return &StreamMapper{}
}

func (m *StreamMapper) MapLine(line []byte) ([]protocol.StreamEvent, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil
	}
	// Gemini SSE events may or may not have the "data:" prefix.
	payload := string(line)
	if strings.HasPrefix(payload, "data:") {
		payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
	}
	if payload == "[DONE]" {
		return []protocol.StreamEvent{{Type: "message_stop"}}, nil
	}

	var chunk struct {
		Candidates []struct {
			Content struct {
				Role  string `json:"role"`
				Parts []struct {
					Text       string `json:"text"`
					Thought    string `json:"thought"`
					ExecutableCode string `json:"executableCode"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata,omitempty"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, err
	}

	if chunk.Error != nil && chunk.Error.Message != "" {
		return []protocol.StreamEvent{{Type: "error", Error: chunk.Error.Message}}, nil
	}

	// Accumulate text from all parts in all candidates.
	var currentText strings.Builder
	finishReason := ""
	for _, candidate := range chunk.Candidates {
		if candidate.FinishReason != "" {
			finishReason = strings.ToLower(candidate.FinishReason)
		}
		for _, part := range candidate.Content.Parts {
			currentText.WriteString(part.Text)
		}
	}

	events := m.emit(currentText.String(), finishReason)

	// Attach usage on the last event if present.
	if chunk.UsageMetadata != nil && len(events) > 0 {
		last := &events[len(events)-1]
		last.Usage = protocol.Usage{
			InputTokens:  chunk.UsageMetadata.PromptTokenCount,
			OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
		}
	}

	return events, nil
}

func (m *StreamMapper) emit(currentText string, finishReason string) []protocol.StreamEvent {
	var events []protocol.StreamEvent

	// First chunk: start the message and the text block.
	if !m.started {
		m.started = true
		events = append(events, protocol.StreamEvent{Type: "message_start"})
		events = append(events, protocol.StreamEvent{
			Type:  "content_block_start",
			Index: 0,
			Block: protocol.ContentBlock{Type: "text"},
		})
	}

	// Diff: delta = currentText[len(prevText):].
	if len(currentText) > len(m.prevText) {
		delta := currentText[len(m.prevText):]
		events = append(events, protocol.StreamEvent{
			Type:  "content_delta",
			Index: 0,
			Delta: delta,
		})
	}
	m.prevText = currentText

	// Finish reason present: close the text block and the message.
	if finishReason != "" {
		stopReason := "end_turn"
		if finishReason == "max_tokens" {
			stopReason = "max_tokens"
		}
		events = append(events, protocol.StreamEvent{Type: "content_block_stop", Index: 0})
		events = append(events, protocol.StreamEvent{Type: "message_delta", StopReason: stopReason})
	}

	return events
}
