package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage: arkrouter <command>") {
		t.Fatalf("stdout missing usage: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "missing"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: missing") {
		t.Fatalf("stderr missing unknown command: %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != "arkrouter dev" {
		t.Fatalf("stdout = %q, want version", stdout.String())
	}
}

func TestRunActivateClaudePrintsExports(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "activate", "claude", "--host", "127.0.0.1", "--port", "20128", "--client-key", "local-key"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activate output missing %s: %q", want, out)
		}
	}
}

func TestRunValidateMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "validate", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "validate failed") {
		t.Fatalf("stderr = %q, want validate failed", stderr.String())
	}
}
