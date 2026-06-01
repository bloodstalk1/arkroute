package observability

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteTraceRedactsSecretsAndIncludesSchema(t *testing.T) {
	var buf bytes.Buffer
	sink := NewJSONLSink(&buf)
	sink.Emit(TraceEvent{
		Time:      time.Unix(0, 0).UTC(),
		Event:     EventRequestStarted,
		RequestID: "req_1",
		Route:     "sonnet",
		Provider:  "openrouter",
		Status:    200,
		Headers:   map[string]string{"Authorization": "Bearer secret", "X-OpenRouter-Title": "Arkrouter"},
	})

	out := buf.String()
	if strings.Contains(out, "Bearer secret") {
		t.Fatalf("trace leaked secret: %s", out)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &decoded); err != nil {
		t.Fatalf("trace is not json: %v", err)
	}
	if decoded["schema_version"].(float64) != 1 {
		t.Fatalf("schema_version = %v, want 1", decoded["schema_version"])
	}
	if decoded["event"] != string(EventRequestStarted) {
		t.Fatalf("event = %v", decoded["event"])
	}
	if sink.Stats().Emitted != 1 {
		t.Fatalf("emitted = %d, want 1", sink.Stats().Emitted)
	}
}

func TestJSONLSinkConcurrentEmit(t *testing.T) {
	var buf bytes.Buffer
	sink := NewJSONLSink(&buf)
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sink.Emit(TraceEvent{Time: time.Unix(0, 0).UTC(), Event: EventRequestStarted, RequestID: "req"})
		}()
	}
	wg.Wait()
	if sink.Stats().Emitted != 25 {
		t.Fatalf("emitted = %d, want 25", sink.Stats().Emitted)
	}
}

func TestNoopSinkCountsDropped(t *testing.T) {
	sink := NewNoopSink()
	sink.Emit(TraceEvent{Event: EventRequestStarted})
	if sink.Stats().Dropped != 1 {
		t.Fatalf("dropped = %d, want 1", sink.Stats().Dropped)
	}
}
