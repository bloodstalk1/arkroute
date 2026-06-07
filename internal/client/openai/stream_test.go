package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func TestNewOpenAIIDUsesRandomHexSuffix(t *testing.T) {
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`^resp_[0-9a-f]{32}$`)
	for range 128 {
		id := newOpenAIID("resp_")
		if !pattern.MatchString(id) {
			t.Fatalf("id = %q, want resp_ plus 32 hex chars", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = true
	}
}

func TestWriteSSEDataReturnsWriteError(t *testing.T) {
	err := writeSSEData(errorResponseWriter{}, "hello")
	if err == nil {
		t.Fatal("writeSSEData() error = nil, want write error")
	}
}

func TestWriteResponsesStreamIncludesCompletedOutput(t *testing.T) {
	events := make(chan protocol.StreamEvent, 3)
	events <- protocol.StreamEvent{Type: "content_delta", Delta: "hel"}
	events <- protocol.StreamEvent{Type: "content_delta", Delta: "lo"}
	events <- protocol.StreamEvent{Type: "message_delta", StopReason: "end_turn"}
	close(events)

	rec := httptest.NewRecorder()
	writeResponsesStream(rec, arkruntime.StreamResult{Events: events}, "sonnet")

	body := rec.Body.String()
	for _, want := range []string{
		`"delta":"hel"`,
		`"delta":"lo"`,
		`"content":[]`,
		`"content":[{"type":"output_text","text":"hello","annotations":[]}]`,
		`"output_text":"hello"`,
		`"output":[{"id":"msg_`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response stream missing %s: %s", want, body)
		}
	}
}

func TestWriteResponsesStreamCompletesOpenItemWhenMessageDeltaMissing(t *testing.T) {
	events := make(chan protocol.StreamEvent, 1)
	events <- protocol.StreamEvent{Type: "content_delta", Delta: "hello"}
	close(events)

	rec := httptest.NewRecorder()
	writeResponsesStream(rec, arkruntime.StreamResult{Events: events}, "sonnet")

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_item.done") {
		t.Fatalf("response stream did not complete output item: %s", body)
	}
	if !strings.Contains(body, `"output_text":"hello"`) {
		t.Fatalf("response stream did not complete output text: %s", body)
	}
}

func TestWriteChatCompletionStreamStopsAfterDone(t *testing.T) {
	events := make(chan protocol.StreamEvent, 2)
	events <- protocol.StreamEvent{Type: "message_stop"}
	events <- protocol.StreamEvent{Type: "content_delta", Delta: "late"}
	close(events)

	rec := httptest.NewRecorder()
	writeChatCompletionStream(rec, arkruntime.StreamResult{Events: events}, "sonnet")

	body := rec.Body.String()
	if strings.Contains(body, "late") {
		t.Fatalf("chat stream wrote event after [DONE]: %s", body)
	}
}

func TestWriteResponsesStreamIgnoresDeltaAfterItemDone(t *testing.T) {
	events := make(chan protocol.StreamEvent, 3)
	events <- protocol.StreamEvent{Type: "content_delta", Delta: "hello"}
	events <- protocol.StreamEvent{Type: "message_delta", StopReason: "end_turn"}
	events <- protocol.StreamEvent{Type: "content_delta", Delta: "late"}
	close(events)

	rec := httptest.NewRecorder()
	writeResponsesStream(rec, arkruntime.StreamResult{Events: events}, "sonnet")

	body := rec.Body.String()
	if strings.Contains(body, "late") {
		t.Fatalf("response stream wrote delta after item.done: %s", body)
	}
}

type errorResponseWriter struct{}

func (errorResponseWriter) Header() http.Header {
	return http.Header{}
}

func (errorResponseWriter) WriteHeader(int) {}

func (errorResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
