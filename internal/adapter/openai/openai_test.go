package openai

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	oaiproto "github.com/bloodstalk1/arkroute/internal/protocol/openai"
)

func TestChatCompletionsURL(t *testing.T) {
	tests := map[string]string{
		"https://openrouter.ai/api/v1": "https://openrouter.ai/api/v1/chat/completions",
		"https://example.test":         "https://example.test/v1/chat/completions",
		"https://example.test/v1/":     "https://example.test/v1/chat/completions",
		"https://opencode.ai/zen/go":   "https://opencode.ai/zen/go/v1/chat/completions",
	}
	for baseURL, want := range tests {
		got, err := ChatCompletionsURL(baseURL)
		if err != nil {
			t.Fatalf("ChatCompletionsURL(%q) error = %v", baseURL, err)
		}
		if got != want {
			t.Fatalf("ChatCompletionsURL(%q) = %q, want %q", baseURL, got, want)
		}
		if strings.Contains(got, "/v1/v1/") {
			t.Fatalf("url duplicated v1: %s", got)
		}
		if !strings.HasSuffix(got, "/chat/completions") {
			t.Fatalf("url = %s, want chat completions suffix", got)
		}
	}
}

func TestBuildRequestMapsTextAndTools(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:     "sonnet",
		MaxTokens: 512,
		Tools: []protocol.Tool{{
			Name:        "read_file",
			Description: "Read file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "hello"}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "anthropic/claude-sonnet-4.5"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if out.Method != "POST" || !strings.HasSuffix(out.URL, "/chat/completions") {
		t.Fatalf("upstream request = %+v", out)
	}
	if out.Headers.Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", out.Headers.Get("Authorization"))
	}
	if !strings.Contains(string(out.Body), `"model":"anthropic/claude-sonnet-4.5"`) {
		t.Fatalf("body = %s", out.Body)
	}
	if !strings.Contains(string(out.Body), `"tools"`) {
		t.Fatalf("body missing tools = %s", out.Body)
	}
}

func TestMapResponse(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if resp.Content[0].Text != "hello" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestMapResponseMapsFinishReasons(t *testing.T) {
	adapter := Adapter{}
	tests := []struct {
		finishReason string
		want         string
	}{
		{finishReason: "stop", want: "end_turn"},
		{finishReason: "length", want: "max_tokens"},
		{finishReason: "tool_calls", want: "tool_use"},
		{finishReason: "function_call", want: "tool_use"},
		{finishReason: "content_filter", want: "end_turn"},
	}
	for _, tt := range tests {
		t.Run(tt.finishReason, func(t *testing.T) {
			body := fmt.Sprintf(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":%q}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`, tt.finishReason)
			resp, err := adapter.MapResponse([]byte(body))
			if err != nil {
				t.Fatalf("MapResponse() error = %v", err)
			}
			if resp.StopReason != tt.want {
				t.Fatalf("StopReason = %q, want %q", resp.StopReason, tt.want)
			}
		})
	}
}

func TestMapResponseKeepsReasoningContentInternal(t *testing.T) {
	resetReasoningCacheForTest()
	defer resetReasoningCacheForTest()

	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning_content":"I should inspect the docs first.","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"deepseek\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content = %+v, want only tool_use block", resp.Content)
	}
	if resp.Content[0].Type != "tool_use" || resp.Content[0].ID != "call_1" {
		t.Fatalf("tool block = %+v", resp.Content[0])
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if got := lookupReasoningForToolCalls([]oaiproto.ToolCall{testReasoningToolCall("call_1")}); got != "I should inspect the docs first." {
		t.Fatalf("cached reasoning = %q, want internal reasoning", got)
	}
}

func TestMapResponseKeepsProviderReasoningInternalForToolReplay(t *testing.T) {
	resetReasoningCacheForTest()
	defer resetReasoningCacheForTest()

	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning_content":"Provider-only reasoning.","content":"","tool_calls":[{"id":"call_future_reasoning","type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"future\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	for _, block := range resp.Content {
		if block.Type == "thinking" || block.Signature != "" {
			t.Fatalf("provider reasoning should not be exposed as Anthropic thinking: %+v", resp.Content)
		}
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "tool_use" || resp.Content[0].ID != "call_future_reasoning" {
		t.Fatalf("content = %+v, want only tool_use block", resp.Content)
	}

	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:  "tool_use",
				ID:    "call_future_reasoning",
				Name:  "search_docs",
				Input: json.RawMessage(`{"query":"future"}`),
			}},
		}, {
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "call_future_reasoning",
				Content:   json.RawMessage(`"results"`),
			}},
		}},
	}
	provider := config.ProviderConfig{ID: "future-openai", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "future-reasoning-model", Capabilities: config.Capabilities{Reasoning: true}}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"reasoning_content":"Provider-only reasoning."`) {
		t.Fatalf("body missing internal reasoning replay: %s", out.Body)
	}
}

func TestMapResponseStripsDuplicateInlineThinkFromText(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning_content":"I should obey the exact output request.","content":"<think>\nI should obey the exact output request.\n</think>\n\nMINIMAX_OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content = %+v, want text only", resp.Content)
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "MINIMAX_OK" {
		t.Fatalf("text block = %+v, want clean final text", resp.Content[0])
	}
}

func TestMapResponseStripsInlineThinkWhenNoReasoningField(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","content":"<think>\nI should answer tersely.\n</think>\n\nOK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "OK" {
		t.Fatalf("content = %+v, want clean text only", resp.Content)
	}
}

func TestMapResponseMapsPseudoWriteToolText(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","content":"<write>\n<path>/tmp/probe.txt</path>\n<content>WRITE_OK\n</content>\n</write>"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content = %+v, want one tool_use block", resp.Content)
	}
	block := resp.Content[0]
	if block.Type != "tool_use" || block.Name != "Write" || block.ID == "" {
		t.Fatalf("tool block = %+v", block)
	}
	var input map[string]string
	if err := json.Unmarshal(block.Input, &input); err != nil {
		t.Fatalf("tool input JSON error = %v", err)
	}
	if input["file_path"] != "/tmp/probe.txt" || input["content"] != "WRITE_OK\n" {
		t.Fatalf("tool input = %#v", input)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q, want tool_use", resp.StopReason)
	}
}

func TestMapResponseNormalizesJSONEncodedToolName(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"{\"name\":\"Write\",\"arguments\":{\"file_path\":\"/tmp/probe.txt\",\"content\":\"WRITE_OK\\n\"}}","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "tool_use" || resp.Content[0].Name != "Write" {
		t.Fatalf("content = %+v, want normalized Write tool_use", resp.Content)
	}
	var input map[string]string
	if err := json.Unmarshal(resp.Content[0].Input, &input); err != nil {
		t.Fatalf("tool input JSON error = %v", err)
	}
	if input["file_path"] != "/tmp/probe.txt" || input["content"] != "WRITE_OK\n" {
		t.Fatalf("tool input = %#v", input)
	}
}

func TestStreamMapperTextDeltas(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"hel"},"index":0}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatal("events empty")
	}
	events, err = mapper.MapLine([]byte(`data: {"choices":[{"delta":{"content":"lo"},"index":0,"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == "content_delta" && event.Delta == "lo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events missing content delta: %+v", events)
	}
}

func TestStreamMapperStripsDuplicateInlineThinkFromText(t *testing.T) {
	mapper := NewStreamMapper()
	lines := [][]byte{
		[]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"I should obey the exact output request.","content":"<think>\nI should obey "}}]}`),
		[]byte(`data: {"choices":[{"index":0,"delta":{"content":"the exact output request.\n</think>\n\nMINIMAX_OK"},"finish_reason":"stop"}]}`),
	}
	var all []protocol.StreamEvent
	for _, line := range lines {
		events, err := mapper.MapLine(line)
		if err != nil {
			t.Fatalf("MapLine() error = %v", err)
		}
		all = append(all, events...)
	}
	var text strings.Builder
	for _, event := range all {
		if event.Type == "content_delta" {
			text.WriteString(event.Delta)
		}
	}
	if strings.Contains(text.String(), "<think>") || text.String() != "MINIMAX_OK" {
		t.Fatalf("text deltas = %q, want clean final text", text.String())
	}
}

func TestStreamMapperMapsPseudoWriteToolText(t *testing.T) {
	mapper := NewStreamMapper()
	lines := [][]byte{
		[]byte(`data: {"choices":[{"index":0,"delta":{"content":"<write>\n<path>/tmp/probe.txt</path>\n"}}]}`),
		[]byte(`data: {"choices":[{"index":0,"delta":{"content":"<content>WRITE_OK\n</content>\n</write>"},"finish_reason":"stop"}]}`),
	}
	var all []protocol.StreamEvent
	for _, line := range lines {
		events, err := mapper.MapLine(line)
		if err != nil {
			t.Fatalf("MapLine() error = %v", err)
		}
		all = append(all, events...)
	}
	var toolStart *protocol.StreamEvent
	var input strings.Builder
	var sawToolStop bool
	var stopReason string
	for i := range all {
		event := all[i]
		switch event.Type {
		case "content_block_start":
			if event.Block.Type == "tool_use" {
				toolStart = &all[i]
			}
		case "tool_input_delta":
			input.WriteString(event.Delta)
		case "content_block_stop":
			if toolStart != nil && event.Index == toolStart.Index {
				sawToolStop = true
			}
		case "message_delta":
			stopReason = event.StopReason
		case "content_delta":
			t.Fatalf("unexpected text delta from pseudo tool: %+v", event)
		}
	}
	if toolStart == nil || toolStart.Block.Name != "Write" || toolStart.Block.ID == "" {
		t.Fatalf("events missing Write tool start: %+v", all)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(input.String()), &decoded); err != nil {
		t.Fatalf("tool input JSON error = %v; input = %q", err, input.String())
	}
	if decoded["file_path"] != "/tmp/probe.txt" || decoded["content"] != "WRITE_OK\n" {
		t.Fatalf("tool input = %#v", decoded)
	}
	if !sawToolStop || stopReason != "tool_use" {
		t.Fatalf("events missing tool stop/tool_use reason: %+v", all)
	}
}

func TestStreamMapperNormalizesJSONEncodedToolName(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"{\"name\":\"Write\",\"arguments\":{\"file_path\":\"/tmp/probe.txt\",\"content\":\"WRITE_OK\\n\"}}","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	var toolStart *protocol.StreamEvent
	var input strings.Builder
	for i := range events {
		event := events[i]
		if event.Type == "content_block_start" && event.Block.Type == "tool_use" {
			toolStart = &events[i]
		}
		if event.Type == "tool_input_delta" {
			input.WriteString(event.Delta)
		}
	}
	if toolStart == nil || toolStart.Block.Name != "Write" {
		t.Fatalf("events = %+v, want normalized Write tool start", events)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(input.String()), &decoded); err != nil {
		t.Fatalf("tool input JSON error = %v; input = %q", err, input.String())
	}
	if decoded["file_path"] != "/tmp/probe.txt" || decoded["content"] != "WRITE_OK\n" {
		t.Fatalf("tool input = %#v", decoded)
	}
}

func TestStreamMapperDone(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte("data: [DONE]"))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "message_stop" {
		t.Fatalf("events = %+v", events)
	}
}

func TestStreamMapperIgnoresDuplicateFinishReason(t *testing.T) {
	mapper := NewStreamMapper()
	if _, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":"stop"}]}`)); err != nil {
		t.Fatalf("MapLine() first finish error = %v", err)
	}
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatalf("MapLine() duplicate finish error = %v", err)
	}
	for _, event := range events {
		if event.Type == "message_delta" {
			t.Fatalf("duplicate finish emitted message_delta: %+v", events)
		}
	}
}

func TestStreamMapperMapsErrorPayload(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"error":{"type":"rate_limit_error","message":"provider overloaded"}}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "error" || !strings.Contains(events[0].Error, "provider overloaded") {
		t.Fatalf("events = %+v, want mapped error event", events)
	}
}

func TestStreamMapperKeepsReasoningContentInternal(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"I should inspect "}}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("provider reasoning-only delta should not emit client events: %+v", events)
	}

	events, err = mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"the docs."},"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatalf("MapLine() second chunk error = %v", err)
	}
	var sawThinking, sawSignature, sawStop bool
	for _, event := range events {
		if event.Type == "thinking_delta" || (event.Type == "content_block_start" && event.Block.Type == "thinking") {
			sawThinking = true
		}
		if event.Type == "signature_delta" {
			sawSignature = true
		}
		if event.Type == "message_delta" && event.StopReason == "end_turn" {
			sawStop = true
		}
	}
	if sawThinking || sawSignature || !sawStop {
		t.Fatalf("events = %+v, want no thinking/signature and end_turn stop", events)
	}
}

func TestStreamMapperKeepsProviderReasoningInternalForToolReplay(t *testing.T) {
	resetReasoningCacheForTest()
	defer resetReasoningCacheForTest()

	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"Provider stream reasoning.","tool_calls":[{"index":0,"id":"call_future_stream","type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"future\"}"}}]},"finish_reason":"tool_calls"}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	for _, event := range events {
		if event.Type == "thinking_delta" || event.Type == "signature_delta" || (event.Type == "content_block_start" && event.Block.Type == "thinking") {
			t.Fatalf("provider reasoning should not be exposed as Anthropic thinking event: %+v", events)
		}
	}
	got := lookupReasoningForToolCalls([]oaiproto.ToolCall{{
		ID:   "call_future_stream",
		Type: "function",
		Function: oaiproto.FunctionCall{
			Name:      "search_docs",
			Arguments: `{"query":"future"}`,
		},
	}})
	if got != "Provider stream reasoning." {
		t.Fatalf("cached reasoning = %q, want provider stream reasoning", got)
	}
}

func TestStreamMapperKeepsReasoningAliasInternal(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning":"GLM style reasoning"},"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	var sawThinking, sawStop bool
	for _, event := range events {
		if event.Type == "thinking_delta" || event.Type == "signature_delta" || (event.Type == "content_block_start" && event.Block.Type == "thinking") {
			sawThinking = true
		}
		if event.Type == "message_delta" && event.StopReason == "end_turn" {
			sawStop = true
		}
	}
	if sawThinking || !sawStop {
		t.Fatalf("events = %+v, want no exposed reasoning and end_turn stop", events)
	}
}

func TestStreamMapperToolCallDeltas(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\""}}]}}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("events too short: %+v", events)
	}
	if events[0].Type != "message_start" {
		t.Fatalf("first event = %+v, want message_start", events[0])
	}
	if events[1].Type != "content_block_start" || events[1].Block.Type != "tool_use" || events[1].Block.ID != "call_1" || events[1].Block.Name != "read_file" {
		t.Fatalf("tool start event = %+v", events[1])
	}
	if events[2].Type != "tool_input_delta" || events[2].Delta != `{"path":"` {
		t.Fatalf("tool input delta = %+v", events[2])
	}

	events, err = mapper.MapLine([]byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"README.md\"}"}}]},"finish_reason":"tool_calls"}]}`))
	if err != nil {
		t.Fatalf("MapLine() second chunk error = %v", err)
	}
	var sawInput, sawStop, sawMessageDelta bool
	for _, event := range events {
		if event.Type == "tool_input_delta" && event.Delta == `README.md"}` {
			sawInput = true
		}
		if event.Type == "content_block_stop" && event.Index == 0 {
			sawStop = true
		}
		if event.Type == "message_delta" && event.StopReason == "tool_use" {
			sawMessageDelta = true
		}
	}
	if !sawInput || !sawStop || !sawMessageDelta {
		t.Fatalf("events missing tool finish pieces: %+v", events)
	}
}

func TestOpenAIAdapterSupportsStreamMapper(t *testing.T) {
	adapter := Adapter{}
	mapper, ok := adapter.NewStreamMapper()
	if !ok {
		t.Fatal("OpenAI adapter should support stream mapper")
	}
	events, err := mapper.MapLine([]byte("data: [DONE]"))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "message_stop" {
		t.Fatalf("events = %+v", events)
	}
}

func TestOpenAIClassifyError(t *testing.T) {
	adapter := Adapter{}
	if got := adapter.ClassifyError(429, nil); got != "upstream_rate_limit" {
		t.Fatalf("ClassifyError(429) = %s", got)
	}
	if got := adapter.ClassifyError(401, nil); got != "upstream_auth" {
		t.Fatalf("ClassifyError(401) = %s", got)
	}
}

func TestBuildRequestMapsToolResults(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "tool_result", ToolUseID: "toolu_1", Content: json.RawMessage(`"file contents"`)}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "model"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"tool_call_id":"toolu_1"`) {
		t.Fatalf("body missing tool result mapping: %s", out.Body)
	}
	if !strings.Contains(string(out.Body), `"content":"file contents"`) {
		t.Fatalf("body did not unmarshal tool result content: %s", out.Body)
	}
	if strings.Contains(string(out.Body), `"content":"\"file contents\""`) {
		t.Fatalf("body double-quoted tool result content: %s", out.Body)
	}
}

func TestBuildRequestIncludesContentForAssistantToolCalls(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:  "tool_use",
				ID:    "toolu_1",
				Name:  "read_file",
				Input: json.RawMessage(`{"path":"README.md"}`),
			}},
		}, {
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "toolu_1",
				Content:   json.RawMessage(`"file contents"`),
			}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "model"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"role":"assistant","content":"","tool_calls"`) {
		t.Fatalf("assistant tool call message should include empty content: %s", out.Body)
	}
}

func TestBuildRequestMapsThinkingToReasoningContent(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:     "thinking",
				Thinking: "I should inspect the docs first.",
			}, {
				Type:  "tool_use",
				ID:    "toolu_1",
				Name:  "search_docs",
				Input: json.RawMessage(`{"query":"deepseek"}`),
			}},
		}, {
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "toolu_1",
				Content:   json.RawMessage(`"docs"`),
			}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "model", Capabilities: config.Capabilities{Reasoning: true}}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"reasoning_content":"I should inspect the docs first."`) {
		t.Fatalf("body missing reasoning_content: %s", out.Body)
	}
}

func TestBuildRequestStripsReasoningContentForNonReasoningModels(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:     "thinking",
				Thinking: "This should not be sent to a non-reasoning OpenAI model.",
			}, {
				Type:  "tool_use",
				ID:    "toolu_1",
				Name:  "search_docs",
				Input: json.RawMessage(`{"query":"deepseek"}`),
			}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "gpt-compatible"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if strings.Contains(string(out.Body), "reasoning_content") {
		t.Fatalf("non-reasoning request should strip reasoning_content: %s", out.Body)
	}
}

func TestBuildRequestPassthroughFollowsClaudeEffortByDefault(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:           "sonnet",
		ReasoningEffort: "xhigh",
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "use tools if needed"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: true},
		Reasoning:     config.ReasoningConfig{Mode: "passthrough"},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	if body["reasoning_effort"] != "max" {
		t.Fatalf("reasoning_effort = %#v, want xhigh mapped to max in body %s", body["reasoning_effort"], out.Body)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking = %#v, want enabled in body %s", body["thinking"], out.Body)
	}
}

func TestBuildRequestCustomReasoningUsesConfiguredEffort(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:           "sonnet",
		ReasoningEffort: "max",
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "use tools if needed"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: true},
		Reasoning:     config.ReasoningConfig{Mode: "custom", Effort: "high"},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	if body["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v, want high in body %s", body["reasoning_effort"], out.Body)
	}
}

func TestBuildRequestPassthroughCanDisableClaudeEffortFollow(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:           "sonnet",
		ReasoningEffort: "max",
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "use tools if needed"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: true},
		Reasoning:     config.ReasoningConfig{Mode: "passthrough", Effort: "high", FollowClaudeEffort: boolPtr(false)},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	if body["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v, want configured high in body %s", body["reasoning_effort"], out.Body)
	}
}

func TestBuildRequestAutoModeCanInferDeepSeekToolChoiceQuirk(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:      "sonnet",
		ToolChoice: json.RawMessage(`"auto"`),
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "use tools if needed"}},
		}},
		Tools: []protocol.Tool{{
			Name:        "search_docs",
			Description: "Search docs",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Reasoning:     config.ReasoningConfig{Mode: "auto"},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	if _, ok := body["tool_choice"]; ok {
		t.Fatalf("auto deepseek-v4 request should omit tool_choice: %s", out.Body)
	}
	if body["reasoning_effort"] != "max" {
		t.Fatalf("reasoning_effort = %#v, want auto max in body %s", body["reasoning_effort"], out.Body)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking = %#v, want enabled in body %s", body["thinking"], out.Body)
	}
}

func TestBuildRequestAutoModeDoesNotReplayReasoningFromOpenCodeProviderName(t *testing.T) {
	resetReasoningCacheForTest()
	defer resetReasoningCacheForTest()

	adapter := Adapter{}
	toolCall := oaiproto.ToolCall{
		ID:   "call_opencode_plain",
		Type: "function",
		Function: oaiproto.FunctionCall{
			Name:      "search_docs",
			Arguments: `{"query":"plain"}`,
		},
	}
	rememberReasoningForToolCall(toolCall, "cached reasoning should not replay")
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:  "tool_use",
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: json.RawMessage(toolCall.Function.Arguments),
			}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "plain-model",
		Reasoning:     config.ReasoningConfig{Mode: "auto"},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if strings.Contains(string(out.Body), "reasoning_content") {
		t.Fatalf("opencode provider name alone should not enable reasoning replay: %s", out.Body)
	}
}

func TestBuildRequestReplaysDeepSeekReasoningWithoutEnablingReasoningRequest(t *testing.T) {
	resetReasoningCacheForTest()
	defer resetReasoningCacheForTest()

	adapter := Adapter{}
	if _, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning_content":"DeepSeek cached reasoning.","content":"","tool_calls":[{"id":"call_deepseek_reasoning","type":"function","function":{"name":"Edit","arguments":"{\"file_path\":\"/tmp/a.py\",\"old_string\":\"bad\",\"new_string\":\"good\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)); err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:  "tool_use",
				ID:    "call_deepseek_reasoning",
				Name:  "Edit",
				Input: json.RawMessage(`{"file_path":"/tmp/a.py","old_string":"bad","new_string":"good"}`),
			}},
		}, {
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "call_deepseek_reasoning",
				Content:   json.RawMessage(`"edited"`),
			}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: false},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	body := string(out.Body)
	if !strings.Contains(body, `"reasoning_content":"DeepSeek cached reasoning."`) {
		t.Fatalf("body missing cached reasoning_content for DeepSeek replay: %s", body)
	}
	if strings.Contains(body, `"reasoning_effort"`) {
		t.Fatalf("DeepSeek replay should not enable reasoning_effort: %s", body)
	}
	if strings.Contains(body, `"thinking":{"type":"enabled"}`) {
		t.Fatalf("DeepSeek replay should not enable thinking request: %s", body)
	}
}

func TestBuildRequestCanFollowClaudeThinkingDisabled(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Thinking: protocol.ThinkingConfig{
			Type: "disabled",
		},
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "answer directly"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: true},
		Reasoning:     config.ReasoningConfig{Mode: "passthrough"},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking = %#v, want disabled in body %s", body["thinking"], out.Body)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("disabled reasoning request should omit reasoning_effort: %s", out.Body)
	}
}

func TestBuildRequestAdaptiveModeMapsThinkingBudget(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Thinking: protocol.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 12000,
		},
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "use tools if needed"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: true},
		Reasoning:     config.ReasoningConfig{Mode: "adaptive"},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	if body["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v, want budget mapped to high in body %s", body["reasoning_effort"], out.Body)
	}
}

func TestBuildRequestConfiguredReasoningDisabledIgnoresClaudeReasoningRequest(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:           "sonnet",
		ReasoningEffort: "xhigh",
		Thinking: protocol.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 12000,
		},
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "answer directly"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Capabilities:  config.Capabilities{Reasoning: true},
		Reasoning:     config.ReasoningConfig{Enabled: boolPtr(false), Replay: boolPtr(false), FollowClaudeEffort: boolPtr(true)},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking = %#v, want disabled in body %s", body["thinking"], out.Body)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("hard-disabled reasoning should ignore Claude effort: %s", out.Body)
	}
}

func TestBuildRequestCanDisableReasoning(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role:    protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "answer directly"}},
		}},
	}
	provider := config.ProviderConfig{ID: "opencode-go", BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{
		UpstreamModel: "deepseek-v4-pro",
		Reasoning:     config.ReasoningConfig{Enabled: boolPtr(false), Replay: boolPtr(false)},
	}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", out.Body, err)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking = %#v, want disabled in body %s", body["thinking"], out.Body)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("disabled reasoning request should omit reasoning_effort: %s", out.Body)
	}
}

func TestBuildRequestReplaysCachedReasoningForToolCalls(t *testing.T) {
	adapter := Adapter{}
	if _, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning_content":"Cached reasoning for a tool call.","content":"","tool_calls":[{"id":"call_cached_reasoning","type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"deepseek\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)); err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:  "tool_use",
				ID:    "call_cached_reasoning",
				Name:  "search_docs",
				Input: json.RawMessage(`{"query":"deepseek"}`),
			}},
		}, {
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "call_cached_reasoning",
				Content:   json.RawMessage(`"docs"`),
			}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "model", Capabilities: config.Capabilities{Reasoning: true}}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"reasoning_content":"Cached reasoning for a tool call."`) {
		t.Fatalf("body missing cached reasoning_content: %s", out.Body)
	}
}

func TestBuildRequestDoesNotReplayReasoningAcrossToolCallIDCollision(t *testing.T) {
	resetReasoningCacheForTest()
	defer resetReasoningCacheForTest()

	adapter := Adapter{}
	if _, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_a","choices":[{"message":{"role":"assistant","reasoning_content":"Reasoning for tab A.","content":"","tool_calls":[{"id":"call_shared","type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"alpha\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)); err != nil {
		t.Fatalf("MapResponse() tab A error = %v", err)
	}
	if _, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_b","choices":[{"message":{"role":"assistant","reasoning_content":"Reasoning for tab B.","content":"","tool_calls":[{"id":"call_shared","type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"beta\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)); err != nil {
		t.Fatalf("MapResponse() tab B error = %v", err)
	}

	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleAssistant,
			Content: []protocol.ContentBlock{{
				Type:  "tool_use",
				ID:    "call_shared",
				Name:  "search_docs",
				Input: json.RawMessage(`{"query":"alpha"}`),
			}},
		}, {
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "call_shared",
				Content:   json.RawMessage(`"docs"`),
			}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "model", Capabilities: config.Capabilities{Reasoning: true}}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"reasoning_content":"Reasoning for tab A."`) {
		t.Fatalf("body should replay tab A reasoning, got: %s", out.Body)
	}
	if strings.Contains(string(out.Body), "Reasoning for tab B.") {
		t.Fatalf("body leaked tab B reasoning into tab A replay: %s", out.Body)
	}
}

func TestReasoningCacheExpiresEntries(t *testing.T) {
	resetReasoningCacheForTest()
	oldTTL := reasoningCacheTTL
	reasoningCacheTTL = time.Nanosecond
	defer func() {
		reasoningCacheTTL = oldTTL
		resetReasoningCacheForTest()
	}()

	rememberReasoningForToolCall(testReasoningToolCall("call_expired_reasoning"), "expired reasoning")
	time.Sleep(time.Millisecond)

	got := lookupReasoningForToolCalls([]oaiproto.ToolCall{testReasoningToolCall("call_expired_reasoning")})
	if got != "" {
		t.Fatalf("expired cache lookup = %q, want empty", got)
	}
}

func TestReasoningCacheEvictsOldEntriesAtLimit(t *testing.T) {
	resetReasoningCacheForTest()
	oldMax := reasoningCacheMaxEntries
	reasoningCacheMaxEntries = 1
	defer func() {
		reasoningCacheMaxEntries = oldMax
		resetReasoningCacheForTest()
	}()

	rememberReasoningForToolCall(testReasoningToolCall("call_old_reasoning"), "old reasoning")
	rememberReasoningForToolCall(testReasoningToolCall("call_new_reasoning"), "new reasoning")

	if got := lookupReasoningForToolCalls([]oaiproto.ToolCall{testReasoningToolCall("call_old_reasoning")}); got != "" {
		t.Fatalf("old cache lookup = %q, want evicted", got)
	}
	if got := lookupReasoningForToolCalls([]oaiproto.ToolCall{testReasoningToolCall("call_new_reasoning")}); got != "new reasoning" {
		t.Fatalf("new cache lookup = %q, want new reasoning", got)
	}
}

func testReasoningToolCall(id string) oaiproto.ToolCall {
	return oaiproto.ToolCall{
		ID:   id,
		Type: "function",
		Function: oaiproto.FunctionCall{
			Name:      "search_docs",
			Arguments: `{"query":"deepseek"}`,
		},
	}
}

func TestMapResponseKeepsReasoningAliasesInternal(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning":"GLM style reasoning","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "hello" {
		t.Fatalf("response should expose only final text, got: %+v", resp.Content)
	}
}

func TestMapResponseKeepsReasoningDetailsInternal(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","reasoning_details":[{"text":"part one "},{"content":"part two"}],"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "hello" {
		t.Fatalf("response should expose only final text, got: %+v", resp.Content)
	}
}

func TestMapToolChoiceConvertsAnthropicShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{name: "string auto", raw: json.RawMessage(`"auto"`), want: `"auto"`},
		{name: "string any", raw: json.RawMessage(`"any"`), want: `"required"`},
		{name: "string tool", raw: json.RawMessage(`"tool"`), want: `"required"`},
		{name: "object auto", raw: json.RawMessage(`{"type":"auto"}`), want: `"auto"`},
		{name: "object any", raw: json.RawMessage(`{"type":"any"}`), want: `"required"`},
		{name: "object tool", raw: json.RawMessage(`{"type":"tool","name":"read_file"}`), want: `{"type":"function","function":{"name":"read_file"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONValue(t, mapToolChoice(tt.raw), tt.want)
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func TestExtractToolContent(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{name: "string", raw: json.RawMessage(`"file contents"`), want: "file contents"},
		{name: "text blocks", raw: json.RawMessage(`[{"type":"text","text":"one"},{"type":"text","text":"two"}]`), want: "one\ntwo"},
		{name: "fallback", raw: json.RawMessage(`{"unexpected":true}`), want: `{"unexpected":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractToolContent(tt.raw); got != tt.want {
				t.Fatalf("extractToolContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func assertJSONValue(t *testing.T, got any, want string) {
	t.Helper()
	gotData, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	var gotValue any
	if err := json.Unmarshal(gotData, &gotValue); err != nil {
		t.Fatalf("unmarshal got %s: %v", gotData, err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("unmarshal want %s: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("mapToolChoice() = %s, want %s", gotData, want)
	}
}
