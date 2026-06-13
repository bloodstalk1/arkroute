package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bloodstalk1/arkroute/internal/adapter"
	"github.com/bloodstalk1/arkroute/internal/adapter/builtin"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/router"
)

type Deps struct {
	Snapshot config.Snapshot
	Router   *router.Router
	Adapters adapter.Registry
	Health   *router.HealthStore
	Trace    observability.TraceSink
	Client   *http.Client
}

type Executor struct {
	Snapshot config.Snapshot
	Router   *router.Router
	Adapters adapter.Registry
	Health   *router.HealthStore
	Trace    observability.TraceSink
	Client   *http.Client
}

func NewExecutor(deps Deps) *Executor {
	if deps.Health == nil {
		deps.Health = router.NewHealthStore()
	}
	if deps.Trace == nil {
		deps.Trace = observability.NewNoopSink()
	}
	if deps.Adapters == nil {
		deps.Adapters = builtin.DefaultRegistry()
	}
	if deps.Client == nil {
		deps.Client = &http.Client{Timeout: time.Duration(deps.Snapshot.Config.Server.UpstreamTimeoutSeconds) * time.Second}
	}
	if deps.Router == nil {
		deps.Router = router.New(deps.Snapshot, deps.Health)
	}
	return &Executor{Snapshot: deps.Snapshot, Router: deps.Router, Adapters: deps.Adapters, Health: deps.Health, Trace: deps.Trace, Client: deps.Client}
}

func (e *Executor) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	e.emit(req, observability.EventRequestStarted, observability.TraceEvent{})
	plan, err := e.Router.Plan(req.Model, req.Requirements)
	if err != nil {
		execErr := &ExecutionError{Class: ErrorRouteNotFound, Message: err.Error()}
		e.emit(req, observability.EventRequestFailed, observability.TraceEvent{ErrorClass: string(execErr.Class), Reason: execErr.Message})
		return ExecuteResult{}, execErr
	}
	e.emit(req, observability.EventRoutePlanned, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy})
	targets, reason := router.PolicyFor(plan.Strategy).Select(plan, e.Health.Snapshot())
	attempts := []Attempt{}
	for i, target := range targets {
		e.emit(req, observability.EventTargetSelected, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Reason: reason}))
		resp, attempt, err := e.executeTarget(ctx, req, target)
		attempts = append(attempts, attempt)
		if err == nil {
			e.Health.Update(router.Update{ProviderID: target.Provider.ID, UpstreamModel: target.Model.UpstreamModel, Status: "ok", StatusCode: attempt.StatusCode, Latency: attempt.Latency})
			e.emit(req, observability.EventRequestFinished, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: attempt.StatusCode, LatencyMS: attempt.Latency.Milliseconds()}))
			return ExecuteResult{Response: resp, Target: target, Attempts: attempts}, nil
		}
		e.Health.Update(router.Update{ProviderID: target.Provider.ID, UpstreamModel: target.Model.UpstreamModel, Status: statusForAttempt(attempt), StatusCode: attempt.StatusCode, ErrorClass: string(attempt.ErrorClass), ErrorMessage: attempt.ErrorMessage, Latency: attempt.Latency})
		if !attempt.Retryable || i == len(targets)-1 {
			execErr := &ExecutionError{Class: attempt.ErrorClass, Message: attempt.ErrorMessage, Attempts: attempts}
			e.emit(req, observability.EventRequestFailed, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, ErrorClass: string(execErr.Class), Reason: execErr.Message}))
			return ExecuteResult{}, execErr
		}
		e.emit(req, observability.EventFallback, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: attempt.StatusCode, Retryable: true, ErrorClass: string(attempt.ErrorClass), Reason: attempt.ErrorMessage}))
	}
	return ExecuteResult{}, &ExecutionError{Class: ErrorRouteNotFound, Message: "route has no targets"}
}

func (e *Executor) executeTarget(ctx context.Context, req ExecuteRequest, target router.Target) (protocol.Response, Attempt, error) {
	start := time.Now()
	providerType := resolveProviderType(target.Provider, target.Model)
	providerAdapter, ok := e.Adapters.Get(providerType)
	if !ok {
		attempt := Attempt{Target: target, ErrorClass: ErrorUpstreamFatal, ErrorMessage: "unsupported provider type " + providerType}
		return protocol.Response{}, attempt, errors.New(attempt.ErrorMessage)
	}
	upstreamReq, err := providerAdapter.BuildRequest(req.Request, target.Provider, target.Model)
	if err != nil {
		attempt := Attempt{Target: target, ErrorClass: ErrorInvalidRequest, ErrorMessage: err.Error(), Latency: time.Since(start)}
		return protocol.Response{}, attempt, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
	if err != nil {
		attempt := Attempt{Target: target, ErrorClass: ErrorInvalidRequest, ErrorMessage: err.Error(), Latency: time.Since(start)}
		return protocol.Response{}, attempt, err
	}
	httpReq.Header = upstreamReq.Headers.Clone()
	e.emit(req, observability.EventUpstreamRequestStarted, traceForTarget(target, observability.TraceEvent{}))
	upstreamResp, err := e.Client.Do(httpReq)
	if err != nil {
		class := ErrorUpstreamFatal
		if ctx.Err() != nil {
			class = ErrorUpstreamTimeout
		}
		attempt := Attempt{Target: target, ErrorClass: class, ErrorMessage: err.Error(), Retryable: class.Retryable(), Latency: time.Since(start)}
		return protocol.Response{}, attempt, err
	}
	defer upstreamResp.Body.Close()
	body, _ := io.ReadAll(upstreamResp.Body)
	attempt := Attempt{Target: target, StatusCode: upstreamResp.StatusCode, Latency: time.Since(start)}
	if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
		class := providerAdapter.ClassifyError(upstreamResp.StatusCode, body)
		attempt.ErrorClass = class
		attempt.Retryable = class.Retryable()
		attempt.ErrorMessage = formatUpstreamError(upstreamResp.StatusCode, body)
		e.emit(req, observability.EventUpstreamResponse, traceForTarget(target, observability.TraceEvent{Status: upstreamResp.StatusCode, LatencyMS: attempt.Latency.Milliseconds(), Retryable: attempt.Retryable, ErrorClass: string(class)}))
		return protocol.Response{}, attempt, errors.New(attempt.ErrorMessage)
	}
	e.emit(req, observability.EventUpstreamResponse, traceForTarget(target, observability.TraceEvent{Status: upstreamResp.StatusCode, LatencyMS: attempt.Latency.Milliseconds()}))
	resp, err := providerAdapter.MapResponse(body)
	if err != nil {
		attempt.ErrorClass = ErrorUpstreamFatal
		attempt.ErrorMessage = err.Error()
		attempt.Retryable = false
		return protocol.Response{}, attempt, err
	}
	return resp, attempt, nil
}

func (e *Executor) emit(req ExecuteRequest, event observability.EventName, base observability.TraceEvent) {
	base.Event = event
	base.RequestID = req.RequestID
	base.Client = req.Client
	if base.Model == "" {
		base.Model = req.Model
	}
	e.Trace.Emit(base)
}

func traceForTarget(target router.Target, event observability.TraceEvent) observability.TraceEvent {
	event.Provider = target.Provider.ID
	event.ProviderType = resolveProviderType(target.Provider, target.Model)
	event.Model = target.Model.ID
	event.UpstreamModel = target.Model.UpstreamModel
	return event
}

func statusForAttempt(attempt Attempt) string {
	if attempt.Retryable {
		return "degraded"
	}
	return "unhealthy"
}

func (e *Executor) Stream(ctx context.Context, req ExecuteRequest) (StreamResult, error) {
	req.Request.Stream = true
	e.emit(req, observability.EventRequestStarted, observability.TraceEvent{})
	plan, err := e.Router.Plan(req.Model, req.Requirements)
	if err != nil {
		return StreamResult{}, &ExecutionError{Class: ErrorRouteNotFound, Message: err.Error()}
	}
	targets, reason := router.PolicyFor(plan.Strategy).Select(plan, e.Health.Snapshot())
	attempts := []Attempt{}
	for i, target := range targets {
		providerType := resolveProviderType(target.Provider, target.Model)
		providerAdapter, ok := e.Adapters.Get(providerType)
		if !ok {
			return StreamResult{}, &ExecutionError{Class: ErrorUpstreamFatal, Message: "unsupported provider type " + providerType, Attempts: attempts}
		}
		mapper, ok := providerAdapter.NewStreamMapper()
		if !ok {
			return StreamResult{}, &ExecutionError{Class: ErrorUnsupportedCapability, Message: "provider does not support streaming", Attempts: attempts}
		}
		upstreamReq, err := providerAdapter.BuildRequest(req.Request, target.Provider, target.Model)
		if err != nil {
			return StreamResult{}, &ExecutionError{Class: ErrorInvalidRequest, Message: err.Error(), Attempts: attempts}
		}
		start := time.Now()
		streamCtx, cancel := context.WithCancel(ctx)
		httpReq, err := http.NewRequestWithContext(streamCtx, upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
		if err != nil {
			cancel()
			return StreamResult{}, &ExecutionError{Class: ErrorInvalidRequest, Message: err.Error(), Attempts: attempts}
		}
		httpReq.Header = upstreamReq.Headers.Clone()
		e.emit(req, observability.EventTargetSelected, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Reason: reason}))
		upstreamResp, err := e.Client.Do(httpReq)
		if err != nil {
			class := ErrorUpstreamFatal
			if streamCtx.Err() != nil {
				class = ErrorUpstreamTimeout
			}
			attempt := Attempt{Target: target, ErrorClass: class, ErrorMessage: err.Error(), Retryable: class.Retryable(), Latency: time.Since(start)}
			attempts = append(attempts, attempt)
			if !attempt.Retryable || i == len(targets)-1 {
				cancel()
				return StreamResult{}, &ExecutionError{Class: class, Message: err.Error(), Attempts: attempts}
			}
			cancel()
			continue
		}
		if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
			body, _ := io.ReadAll(upstreamResp.Body)
			_ = upstreamResp.Body.Close()
			cancel()
			class := providerAdapter.ClassifyError(upstreamResp.StatusCode, body)
			attempt := Attempt{Target: target, StatusCode: upstreamResp.StatusCode, ErrorClass: class, ErrorMessage: formatUpstreamError(upstreamResp.StatusCode, body), Retryable: class.Retryable(), Latency: time.Since(start)}
			attempts = append(attempts, attempt)
			if !attempt.Retryable || i == len(targets)-1 {
				return StreamResult{}, &ExecutionError{Class: class, Message: attempt.ErrorMessage, Attempts: attempts}
			}
			e.emit(req, observability.EventFallback, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: attempt.StatusCode, Retryable: true, ErrorClass: string(class), Reason: attempt.ErrorMessage}))
			continue
		}
		events := make(chan protocol.StreamEvent, 16)
		done := make(chan struct{})
		var closeBody sync.Once
		closeBodyOnce := func() {
			closeBody.Do(func() {
				_ = upstreamResp.Body.Close()
			})
		}
		go func() {
			defer close(events)
			defer close(done)
			defer closeBodyOnce()
			scanner := bufio.NewScanner(withIdleWatchdog(upstreamResp.Body, 2*time.Minute, closeBodyOnce))
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				mapped, err := mapper.MapLine(scanner.Bytes())
				if err != nil {
					select {
					case events <- protocol.StreamEvent{Type: "error", Error: err.Error()}:
					case <-streamCtx.Done():
					}
					return
				}
				for _, event := range mapped {
					select {
					case events <- event:
					case <-streamCtx.Done():
						return
					}
				}
				select {
				case <-streamCtx.Done():
					return
				default:
				}
			}
			if err := scanner.Err(); err != nil && streamCtx.Err() == nil {
				select {
				case events <- protocol.StreamEvent{Type: "error", Error: err.Error()}:
				case <-streamCtx.Done():
				}
			}
		}()
		closeFn := func() error {
			cancel()
			closeBodyOnce()
			<-done
			return nil
		}
		e.Health.Update(router.Update{ProviderID: target.Provider.ID, UpstreamModel: target.Model.UpstreamModel, Status: "ok", StatusCode: upstreamResp.StatusCode, Latency: time.Since(start)})
		e.emit(req, observability.EventStreamStarted, traceForTarget(target, observability.TraceEvent{Route: plan.Alias, Strategy: plan.Strategy, Status: upstreamResp.StatusCode}))
		return StreamResult{Target: target, Attempts: append(attempts, Attempt{Target: target, StatusCode: upstreamResp.StatusCode, Latency: time.Since(start)}), Events: events, Close: closeFn}, nil
	}
	return StreamResult{}, &ExecutionError{Class: ErrorRouteNotFound, Message: "route has no targets", Attempts: attempts}
}
func formatUpstreamError(status int, body []byte) string {
	base := fmt.Sprintf("upstream returned %d", status)
	message := extractUpstreamErrorMessage(body)
	if message == "" {
		return base
	}
	return base + ": " + message
}

func extractUpstreamErrorMessage(body []byte) string {
	var decoded any
	if json.Unmarshal(body, &decoded) == nil {
		if message := findMessageField(decoded); message != "" {
			return limitErrorMessage(message)
		}
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	return limitErrorMessage(text)
}

func findMessageField(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if message, ok := typed["message"].(string); ok && strings.TrimSpace(message) != "" {
			return strings.TrimSpace(message)
		}
		for _, child := range typed {
			if message := findMessageField(child); message != "" {
				return message
			}
		}
	case []any:
		for _, child := range typed {
			if message := findMessageField(child); message != "" {
				return message
			}
		}
	}
	return ""
}

func limitErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 500 {
		return message
	}
	return message[:500] + "..."
}
