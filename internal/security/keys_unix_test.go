//go:build !windows

package security

import "testing"

func TestShellQuote(t *testing.T) {
	got := ShellQuote("a'b")
	if got != "'a'\"'\"'b'" {
		t.Fatalf("ShellQuote() = %q", got)
	}
}
