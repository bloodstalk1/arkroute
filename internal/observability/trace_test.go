package observability

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteTraceRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTrace(&buf, TraceEvent{
		Time:      time.Unix(0, 0).UTC(),
		RequestID: "req_1",
		Route:     "sonnet",
		Provider:  "openrouter",
		Status:    200,
		Headers:   map[string]string{"Authorization": "Bearer secret", "X-OpenRouter-Title": "Arkrouter"},
	})
	if err != nil {
		t.Fatalf("WriteTrace() error = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Bearer secret") {
		t.Fatalf("trace leaked secret: %s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("trace missing redaction: %s", out)
	}
}
