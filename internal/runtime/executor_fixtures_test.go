package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFormatUpstreamErrorAgainstRealUpstreamBodies exercises
// formatUpstreamError with HTML error pages and other unstructured
// bodies that real upstreams (Cloudflare, nginx) return.
func TestFormatUpstreamErrorAgainstRealUpstreamBodies(t *testing.T) {
	tests := []struct {
		file     string
		status   int
		contains string
	}{
		{"upstream_502.html", 502, "502 Bad Gateway"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got := formatUpstreamError(tt.status, data)
			if !strings.Contains(got, tt.contains) {
				t.Fatalf("formatUpstreamError(%d, %s) = %q; want substring %q",
					tt.status, tt.file, got, tt.contains)
			}
		})
	}
}
