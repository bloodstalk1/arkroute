package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/router"
)

func TestExecutorStreamSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"index\":0}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	executor := testExecutorWithSink(t, upstream.URL, observability.NewNoopSink())
	stream, err := executor.Stream(context.Background(), ExecuteRequest{
		RequestID:    "req_stream",
		Client:       "claude",
		Model:        "sonnet",
		Requirements: router.Requirements{Streaming: true},
		Request:      protocol.Request{Model: "sonnet", Stream: true},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	seenStop := false
	for event := range stream.Events {
		if event.Type == "message_stop" {
			seenStop = true
		}
	}
	if !seenStop {
		t.Fatal("stream did not emit message_stop")
	}
}

func TestExecutorStreamFallbackBeforeEvents(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer second.Close()

	cfg := twoTargetConfig(first.URL, second.URL)
	executor := executorFromConfig(t, cfg, observability.NewNoopSink())
	stream, err := executor.Stream(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Requirements: router.Requirements{Streaming: true}, Request: protocol.Request{Model: "sonnet", Stream: true}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	if len(stream.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(stream.Attempts))
	}
}

func TestExecutorStreamUnsupportedCapabilityDoesNotCallUpstream(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].Type = "gemini"
	cfg.Providers[0].BaseURL = "https://example.test"
	executor := executorFromConfig(t, cfg, observability.NewNoopSink())
	_, err := executor.Stream(context.Background(), ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Requirements: router.Requirements{Streaming: true}, Request: protocol.Request{Model: "sonnet", Stream: true}})
	if err == nil {
		t.Fatal("Stream() error = nil, want unsupported capability")
	}
	var execErr *ExecutionError
	if !AsExecutionError(err, &execErr) || execErr.Class != ErrorUnsupportedCapability {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
}

func TestExecutorStreamClosesOnContextCancel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"index\":0}]}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	executor := testExecutorWithSink(t, upstream.URL, observability.NewNoopSink())
	stream, err := executor.Stream(ctx, ExecuteRequest{RequestID: "req", Client: "claude", Model: "sonnet", Requirements: router.Requirements{Streaming: true}, Request: protocol.Request{Model: "sonnet", Stream: true}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-stream.Events:
			if !ok {
				if err := stream.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
				return
			}
		case <-deadline:
			t.Fatal("stream did not close after cancel")
		}
	}
}
