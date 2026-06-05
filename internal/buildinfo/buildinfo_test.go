package buildinfo

import "strings"
import "testing"

func TestSummary(t *testing.T) {
	got := Summary()
	if !strings.Contains(got, "arkroute") {
		t.Fatalf("Summary() = %q", got)
	}
}

func TestDebug(t *testing.T) {
	got := Debug()
	for _, want := range []string{"version:", "commit:", "build_date:", "go:", "os_arch:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Debug() missing %s: %q", want, got)
		}
	}
}
