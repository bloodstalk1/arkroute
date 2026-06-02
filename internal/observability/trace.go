package observability

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/bloodstalk1/arkroute/internal/security"
)

const SchemaVersion = 1

type EventName string

const (
	EventRequestStarted         EventName = "request_started"
	EventRoutePlanned           EventName = "route_planned"
	EventTargetSelected         EventName = "target_selected"
	EventUpstreamRequestStarted EventName = "upstream_request_started"
	EventUpstreamResponse       EventName = "upstream_response"
	EventFallback               EventName = "fallback"
	EventStreamStarted          EventName = "stream_started"
	EventRequestFinished        EventName = "request_finished"
	EventRequestFailed          EventName = "request_failed"
)

type TraceEvent struct {
	SchemaVersion int               `json:"schema_version"`
	Time          time.Time         `json:"time"`
	Event         EventName         `json:"event"`
	RequestID     string            `json:"request_id"`
	Client        string            `json:"client,omitempty"`
	Route         string            `json:"route,omitempty"`
	Strategy      string            `json:"strategy,omitempty"`
	Provider      string            `json:"provider,omitempty"`
	ProviderType  string            `json:"provider_type,omitempty"`
	Model         string            `json:"model,omitempty"`
	UpstreamModel string            `json:"upstream_model,omitempty"`
	Status        int               `json:"status,omitempty"`
	LatencyMS     int64             `json:"latency_ms,omitempty"`
	Retryable     bool              `json:"retryable,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	ErrorClass    string            `json:"error_class,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type Stats struct {
	Emitted      int64 `json:"emitted"`
	FailedWrites int64 `json:"failed_writes"`
	Dropped      int64 `json:"dropped"`
}

type TraceSink interface {
	Emit(event TraceEvent)
	Stats() Stats
}

type JSONLSink struct {
	mu    sync.Mutex
	w     io.Writer
	stats Stats
}

func NewJSONLSink(w io.Writer) *JSONLSink {
	return &JSONLSink{w: w}
}

func (s *JSONLSink) Emit(event TraceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.SchemaVersion = SchemaVersion
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.Headers != nil {
		event.Headers = security.RedactMap(event.Headers)
	}
	data, err := json.Marshal(event)
	if err != nil {
		s.stats.FailedWrites++
		return
	}
	if _, err := s.w.Write(append(data, '\n')); err != nil {
		s.stats.FailedWrites++
		return
	}
	s.stats.Emitted++
}

func (s *JSONLSink) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

type NoopSink struct {
	mu    sync.Mutex
	stats Stats
}

func NewNoopSink() *NoopSink {
	return &NoopSink{}
}

func (s *NoopSink) Emit(TraceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Dropped++
}

func (s *NoopSink) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}
