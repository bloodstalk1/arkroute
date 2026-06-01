package runtime

import (
	"errors"
	"time"

	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
	"bat.dev/arkrouter/internal/router"
)

type ErrorClass = failure.ErrorClass

const (
	ErrorInvalidRequest        = failure.ErrorInvalidRequest
	ErrorRouteNotFound         = failure.ErrorRouteNotFound
	ErrorUnsupportedCapability = failure.ErrorUnsupportedCapability
	ErrorGatewayAuth           = failure.ErrorGatewayAuth
	ErrorUpstreamAuth          = failure.ErrorUpstreamAuth
	ErrorUpstreamRateLimit     = failure.ErrorUpstreamRateLimit
	ErrorUpstreamTimeout       = failure.ErrorUpstreamTimeout
	ErrorUpstreamRetryable     = failure.ErrorUpstreamRetryable
	ErrorUpstreamFatal         = failure.ErrorUpstreamFatal
	ErrorStream                = failure.ErrorStream
)

type ExecuteRequest struct {
	RequestID    string
	Client       string
	Model        string
	Requirements router.Requirements
	Request      protocol.Request
}

type ExecuteResult struct {
	Response protocol.Response
	Target   router.Target
	Attempts []Attempt
}

type Attempt struct {
	Target       router.Target
	StatusCode   int
	Latency      time.Duration
	Retryable    bool
	ErrorClass   ErrorClass
	ErrorMessage string
}

type ExecutionError struct {
	Class    ErrorClass
	Message  string
	Attempts []Attempt
}

func (e *ExecutionError) Error() string {
	return e.Message
}

func AsExecutionError(err error, target **ExecutionError) bool {
	var execErr *ExecutionError
	if errors.As(err, &execErr) {
		*target = execErr
		return true
	}
	return false
}
