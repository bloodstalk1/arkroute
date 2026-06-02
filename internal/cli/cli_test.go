package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage: arkroute <command>") {
		t.Fatalf("stdout missing usage: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "missing"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: missing") {
		t.Fatalf("stderr missing unknown command: %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != "arkroute dev" {
		t.Fatalf("stdout = %q, want version", stdout.String())
	}
}

func TestRunActivateClaudePrintsExports(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "activate", "claude", "--host", "127.0.0.1", "--port", "20128", "--client-key", "local-key"}, &stdout, &stderr)
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
	code := Run([]string{"arkroute", "validate", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "validate failed") {
		t.Fatalf("stderr = %q, want validate failed", stderr.String())
	}
}

func TestRunDoctor(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "doctor", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "doctor failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunLogsMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "logs", "--file", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "logs failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunTestMissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "test"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: arkroute test <model> <prompt>") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunConfigPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "config", "path"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ".arkroute") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunProviderListMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "provider", "list", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "provider list failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunStatusMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "status", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "status failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunVersionDebug(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkroute", "version", "--debug"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"version:", "commit:", "build_date:", "go:", "os_arch:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %s: %q", want, stdout.String())
		}
	}
}
