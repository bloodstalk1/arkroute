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
		Headers:   map[string]string{"Authorization": "Bearer secret", "X-OpenRouter-Title": "Arkroute"},
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

func TestTraceReloadEventIncludesGenerationMetadata(t *testing.T) {
	var buf bytes.Buffer
	sink := NewJSONLSink(&buf)
	sink.Emit(TraceEvent{
		Time:                     time.Unix(0, 0).UTC(),
		Event:                    EventConfigReloadSucceeded,
		Client:                   "admin",
		ConfigGeneration:         2,
		PreviousConfigGeneration: 1,
		NextConfigGeneration:     2,
		ConfigPath:               "/Users/bat/.arkroute/config.yaml",
	})

	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &decoded); err != nil {
		t.Fatalf("trace is not json: %v", err)
	}
	if decoded["event"] != string(EventConfigReloadSucceeded) {
		t.Fatalf("event = %v, want %s", decoded["event"], EventConfigReloadSucceeded)
	}
	if decoded["config_generation"].(float64) != 2 {
		t.Fatalf("config_generation = %v, want 2", decoded["config_generation"])
	}
	if decoded["previous_config_generation"].(float64) != 1 {
		t.Fatalf("previous_config_generation = %v, want 1", decoded["previous_config_generation"])
	}
	if decoded["next_config_generation"].(float64) != 2 {
		t.Fatalf("next_config_generation = %v, want 2", decoded["next_config_generation"])
	}
	if decoded["config_path"] != "/Users/bat/.arkroute/config.yaml" {
		t.Fatalf("config_path = %v, want /Users/bat/.arkroute/config.yaml", decoded["config_path"])
	}
}
