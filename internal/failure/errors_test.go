package failure

import "testing"

func TestClassifyStatus(t *testing.T) {
	t.Parallel()
	tests := map[int]ErrorClass{
		400: ErrorInvalidRequest,
		401: ErrorUpstreamAuth,
		403: ErrorUpstreamAuth,
		408: ErrorUpstreamRetryable,
		429: ErrorUpstreamRateLimit,
		500: ErrorUpstreamRetryable,
		502: ErrorUpstreamRetryable,
		503: ErrorUpstreamRetryable,
		504: ErrorUpstreamRetryable,
	}
	for status, want := range tests {
		if got := ClassifyStatus(status); got != want {
			t.Fatalf("ClassifyStatus(%d) = %s, want %s", status, got, want)
		}
	}
}

func TestErrorClassRetryable(t *testing.T) {
	t.Parallel()
	for _, class := range []ErrorClass{ErrorUpstreamRateLimit, ErrorUpstreamRetryable, ErrorUpstreamTimeout} {
		if !class.Retryable() {
			t.Fatalf("%s should be retryable", class)
		}
	}
	for _, class := range []ErrorClass{ErrorUpstreamAuth, ErrorInvalidRequest, ErrorUnsupportedCapability} {
		if class.Retryable() {
			t.Fatalf("%s should not be retryable", class)
		}
	}
}

func TestControlPlaneErrorClassesAreNotRetryable(t *testing.T) {
	t.Parallel()
	tests := []ErrorClass{
		ErrorConfigReloadFailed,
		ErrorConfigValidationFailed,
		ErrorConfigReadFailed,
		ErrorListenerChangeRequiresRestart,
		ErrorAdminAuthFailed,
		ErrorAdminMalformedResponse,
		ErrorServerUnreachable,
	}
	for _, class := range tests {
		if class.Retryable() {
			t.Fatalf("%s should not be retryable", class)
		}
	}
}
