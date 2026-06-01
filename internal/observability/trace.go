package observability

import (
	"encoding/json"
	"io"
	"time"

	"bat.dev/arkrouter/internal/security"
)

type TraceEvent struct {
	Time      time.Time         `json:"time"`
	RequestID string            `json:"request_id"`
	Route     string            `json:"route"`
	Provider  string            `json:"provider"`
	Model     string            `json:"model,omitempty"`
	Status    int               `json:"status"`
	LatencyMS int64             `json:"latency_ms,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

func WriteTrace(w io.Writer, event TraceEvent) error {
	if event.Headers != nil {
		event.Headers = security.RedactMap(event.Headers)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}
