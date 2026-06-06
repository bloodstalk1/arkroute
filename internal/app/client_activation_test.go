package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestPrintClientActivationIPv6(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{
			Host:      "::1",
			Port:      8000,
			ClientKey: "test-client-key",
		},
	}

	// Test Claude activation with IPv6
	var buf bytes.Buffer
	err := PrintClientActivation(&buf, cfg, "claude")
	if err != nil {
		t.Fatalf("PrintClientActivation error = %v", err)
	}

	got := buf.String()
	wantBaseURL := "http://[::1]:8000"
	if !strings.Contains(got, wantBaseURL) {
		t.Fatalf("PrintClientActivation(claude) output does not contain %q. Got:\n%s", wantBaseURL, got)
	}
}

func TestPrintClientActivationIPv4(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{
			Host:      "127.0.0.1",
			Port:      8000,
			ClientKey: "test-client-key",
		},
	}

	// Test Claude activation with IPv4
	var buf bytes.Buffer
	err := PrintClientActivation(&buf, cfg, "claude")
	if err != nil {
		t.Fatalf("PrintClientActivation error = %v", err)
	}

	got := buf.String()
	wantBaseURL := "http://127.0.0.1:8000"
	if !strings.Contains(got, wantBaseURL) {
		t.Fatalf("PrintClientActivation(claude) output does not contain %q. Got:\n%s", wantBaseURL, got)
	}
}
