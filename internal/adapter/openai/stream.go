package openai

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	oaiproto "github.com/bloodstalk1/arkroute/internal/protocol/openai"
)

type StreamMapper struct {
	started            bool
	finished           bool
	textStarted        bool
	textBlockIndex     int
	reasoning          strings.Builder
	nextBlockIndex     int
	toolBlocks         map[int]int
	toolCallIDs        map[int]string
	toolCallNames      map[int]string
	toolCallArguments  map[int]string
	openBlocks         map[int]bool
	contentBuffer      strings.Builder
	bufferingContent   bool
	syntheticToolIndex int
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
		events := []protocol.StreamEvent{}
		if m.started && !m.finished {
			m.finish(&events, "end_turn")
		}
		events = append(events, protocol.StreamEvent{Type: "message_stop"})
		return events, nil
	}
	var chunk struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string                     `json:"role"`
				Content          string                     `json:"content"`
				ReasoningContent string                     `json:"reasoning_content"`
				Reasoning        string                     `json:"reasoning"`
				ReasoningDetails []oaiproto.ReasoningDetail `json:"reasoning_details"`
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, err
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return []protocol.StreamEvent{{Type: "error", Error: formatOpenAIStreamError(chunk.Error)}}, nil
	}
	events := []protocol.StreamEvent{}
	for _, choice := range chunk.Choices {
		reasoning := reasoningFromFields(choice.Delta.ReasoningContent, choice.Delta.Reasoning, choice.Delta.ReasoningDetails)
		if reasoning != "" {
			m.reasoning.WriteString(reasoning)
		}
		if choice.Delta.Content != "" {
			m.handleContentDelta(&events, choice.Delta.Content)
		}
		for _, call := range choice.Delta.ToolCalls {
			name, arguments := normalizeToolNameAndArguments(call.Function.Name, call.Function.Arguments)
			blockIndex := m.startToolBlock(&events, call.Index, call.ID, name, arguments)
			if arguments != "" {
				events = append(events, protocol.StreamEvent{Type: "tool_input_delta", Index: blockIndex, Delta: arguments})
			}
		}
		if choice.FinishReason != "" {
			stopReason := mapFinishReason(choice.FinishReason)
			m.flushBufferedContent(&events, &stopReason)
			m.finish(&events, stopReason)
		}
	}
	return events, nil
}

func formatOpenAIStreamError(raw json.RawMessage) string {
	var message string
	if json.Unmarshal(raw, &message) == nil && message != "" {
		return message
	}
	var decoded struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if json.Unmarshal(raw, &decoded) == nil {
		if decoded.Message != "" {
			return decoded.Message
		}
		if decoded.Type != "" {
			return decoded.Type
		}
		if decoded.Code != "" {
			return decoded.Code
		}
	}
	return string(raw)
}

func (m *StreamMapper) ensureStarted(events *[]protocol.StreamEvent) {
	if m.started {
		return
	}
	m.started = true
	*events = append(*events, protocol.StreamEvent{Type: "message_start"})
}

func (m *StreamMapper) startTextBlock(events *[]protocol.StreamEvent) int {
	m.ensureStarted(events)
	if m.openBlocks == nil {
		m.openBlocks = map[int]bool{}
	}
	if !m.textStarted {
		m.textStarted = true
		m.textBlockIndex = m.nextBlockIndex
		m.nextBlockIndex++
		m.openBlocks[m.textBlockIndex] = true
		*events = append(*events, protocol.StreamEvent{Type: "content_block_start", Index: m.textBlockIndex, Block: protocol.ContentBlock{Type: "text"}})
	}
	return m.textBlockIndex
}

func (m *StreamMapper) startToolBlock(events *[]protocol.StreamEvent, toolIndex int, id string, name string, arguments string) int {
	m.ensureStarted(events)
	if m.toolBlocks == nil {
		m.toolBlocks = map[int]int{}
	}
	if m.openBlocks == nil {
		m.openBlocks = map[int]bool{}
	}
	m.recordToolCallDelta(toolIndex, id, name, arguments)
	if blockIndex, ok := m.toolBlocks[toolIndex]; ok {
		return blockIndex
	}
	blockIndex := m.nextBlockIndex
	m.nextBlockIndex++
	m.toolBlocks[toolIndex] = blockIndex
	m.openBlocks[blockIndex] = true
	*events = append(*events, protocol.StreamEvent{Type: "content_block_start", Index: blockIndex, Block: protocol.ContentBlock{Type: "tool_use", ID: id, Name: name}})
	return blockIndex
}

func (m *StreamMapper) handleContentDelta(events *[]protocol.StreamEvent, delta string) {
	if m.bufferingContent || shouldBufferContentPrefix(m.contentBuffer.String()+delta) {
		m.bufferingContent = true
		m.contentBuffer.WriteString(delta)
		if !couldBecomeStructuredContent(m.contentBuffer.String()) {
			m.flushBufferedText(events)
		}
		return
	}
	m.emitTextDelta(events, delta)
}

func shouldBufferContentPrefix(content string) bool {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	return trimmed == "" || strings.HasPrefix(trimmed, "<")
}

func couldBecomeStructuredContent(content string) bool {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"<think", "<write", "<tool_call", "<invoke"} {
		if strings.HasPrefix(lower, prefix) || strings.HasPrefix(prefix, lower) {
			return true
		}
	}
	return false
}

func (m *StreamMapper) flushBufferedText(events *[]protocol.StreamEvent) {
	if m.contentBuffer.Len() == 0 {
		m.bufferingContent = false
		return
	}
	text := m.contentBuffer.String()
	m.contentBuffer.Reset()
	m.bufferingContent = false
	m.emitTextDelta(events, text)
}

func (m *StreamMapper) flushBufferedContent(events *[]protocol.StreamEvent, stopReason *string) {
	if m.contentBuffer.Len() == 0 {
		m.bufferingContent = false
		return
	}
	content := m.contentBuffer.String()
	m.contentBuffer.Reset()
	m.bufferingContent = false
	if inlineThinking, rest, ok := splitInlineThinking(content); ok {
		if m.reasoning.Len() == 0 && inlineThinking != "" {
			m.reasoning.WriteString(inlineThinking)
		}
		content = rest
	}
	if toolBlock, ok := parsePseudoToolUse(content); ok {
		m.emitSyntheticToolUse(events, toolBlock)
		*stopReason = "tool_use"
		return
	}
	m.emitTextDelta(events, content)
}

func (m *StreamMapper) emitTextDelta(events *[]protocol.StreamEvent, delta string) {
	if delta == "" {
		return
	}
	textIndex := m.startTextBlock(events)
	*events = append(*events, protocol.StreamEvent{Type: "content_delta", Index: textIndex, Delta: delta})
}

func (m *StreamMapper) emitSyntheticToolUse(events *[]protocol.StreamEvent, block protocol.ContentBlock) {
	m.syntheticToolIndex--
	toolIndex := m.syntheticToolIndex
	blockIndex := m.startToolBlock(events, toolIndex, block.ID, block.Name, string(block.Input))
	if len(block.Input) > 0 {
		*events = append(*events, protocol.StreamEvent{Type: "tool_input_delta", Index: blockIndex, Delta: string(block.Input)})
	}
}

func (m *StreamMapper) recordToolCallDelta(toolIndex int, id string, name string, arguments string) {
	if m.toolCallIDs == nil {
		m.toolCallIDs = map[int]string{}
	}
	if m.toolCallNames == nil {
		m.toolCallNames = map[int]string{}
	}
	if m.toolCallArguments == nil {
		m.toolCallArguments = map[int]string{}
	}
	if id != "" {
		m.toolCallIDs[toolIndex] = id
	}
	if name != "" {
		m.toolCallNames[toolIndex] = name
	}
	if arguments != "" {
		m.toolCallArguments[toolIndex] += arguments
	}
}

func (m *StreamMapper) finish(events *[]protocol.StreamEvent, stopReason string) {
	if m.finished {
		return
	}
	if m.contentBuffer.Len() > 0 {
		m.flushBufferedContent(events, &stopReason)
	}
	m.ensureStarted(events)
	if m.reasoning.Len() > 0 {
		for _, call := range m.toolCallsForReasoningCache() {
			rememberReasoningForToolCall(call, m.reasoning.String())
		}
	}
	if m.openBlocks != nil {
		indices := make([]int, 0, len(m.openBlocks))
		for index := range m.openBlocks {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			*events = append(*events, protocol.StreamEvent{Type: "content_block_stop", Index: index})
			delete(m.openBlocks, index)
		}
	}
	if stopReason == "" {
		stopReason = "end_turn"
	}
	*events = append(*events, protocol.StreamEvent{Type: "message_delta", StopReason: stopReason})
	m.finished = true
}

func (m *StreamMapper) toolCallsForReasoningCache() []oaiproto.ToolCall {
	if len(m.toolCallIDs) == 0 {
		return nil
	}
	indices := make([]int, 0, len(m.toolCallIDs))
	for index := range m.toolCallIDs {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	calls := make([]oaiproto.ToolCall, 0, len(indices))
	for _, index := range indices {
		id := m.toolCallIDs[index]
		if id == "" {
			continue
		}
		calls = append(calls, oaiproto.ToolCall{
			ID:   id,
			Type: "function",
			Function: oaiproto.FunctionCall{
				Name:      m.toolCallNames[index],
				Arguments: m.toolCallArguments[index],
			},
		})
	}
	return calls
}

func mapFinishReason(reason string) string {
	switch reason {
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}
