package failure

type ErrorClass string

const (
	ErrorInvalidRequest        ErrorClass = "invalid_request"
	ErrorRouteNotFound         ErrorClass = "route_not_found"
	ErrorUnsupportedCapability ErrorClass = "unsupported_capability"
	ErrorGatewayAuth           ErrorClass = "gateway_auth"
	ErrorUpstreamAuth          ErrorClass = "upstream_auth"
	ErrorUpstreamRateLimit     ErrorClass = "upstream_rate_limit"
	ErrorUpstreamTimeout       ErrorClass = "upstream_timeout"
	ErrorUpstreamRetryable     ErrorClass = "upstream_retryable"
	ErrorUpstreamFatal         ErrorClass = "upstream_fatal"
	ErrorStream                ErrorClass = "stream_error"
)

const (
	ErrorConfigReloadFailed            ErrorClass = "config_reload_failed"
	ErrorConfigValidationFailed        ErrorClass = "config_validation_failed"
	ErrorConfigReadFailed              ErrorClass = "config_read_failed"
	ErrorListenerChangeRequiresRestart ErrorClass = "listener_change_requires_restart"
	ErrorAdminAuthFailed               ErrorClass = "admin_auth_failed"
	ErrorAdminMalformedResponse        ErrorClass = "admin_malformed_response"
	ErrorServerUnreachable             ErrorClass = "server_unreachable"
)

func ClassifyStatus(status int) ErrorClass {
	switch status {
	case 400:
		return ErrorInvalidRequest
	case 401, 403:
		return ErrorUpstreamAuth
	case 408, 500, 502, 503, 504:
		return ErrorUpstreamRetryable
	case 429:
		return ErrorUpstreamRateLimit
	default:
		if status >= 500 {
			return ErrorUpstreamRetryable
		}
		return ErrorUpstreamFatal
	}
}

func (c ErrorClass) Retryable() bool {
	switch c {
	case ErrorUpstreamRateLimit, ErrorUpstreamRetryable, ErrorUpstreamTimeout:
		return true
	default:
		return false
	}
}
