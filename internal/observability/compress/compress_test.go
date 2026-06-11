package compress

import (
	"strings"
	"testing"
)

func TestLiteCollapsesWhitespace(t *testing.T) {
	input := "hello     world\n\n\n\nfoo   bar"
	got := Tidy(input, Lite)
	if strings.Count(got, "     ") > 0 {
		t.Errorf("expected collapsed spaces, got %q", got)
	}
	// Newlines still preserved (at most 2 consecutive)
}

func TestLiteTrimsTrailingBlanks(t *testing.T) {
	got := Tidy("hello\n\n\n\n\n", Lite)
	if strings.HasSuffix(got, "\n") {
		t.Errorf("expected trimmed trailing blanks, got %q", got)
	}
}

func TestAggressiveTruncates(t *testing.T) {
	long := strings.Repeat("this is a long line that goes on and on. ", 200)
	got := Tidy(long, Aggressive)
	if len(got) > 4200 {
		t.Errorf("expected truncated, got %d chars", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("expected truncation marker")
	}
}

func TestNoneIsIdentity(t *testing.T) {
	input := "hello   world"
	got := Tidy(input, None)
	if got != input {
		t.Errorf("None should be identity, got %q", got)
	}
}

func TestEmptyInput(t *testing.T) {
	got := Tidy("", Aggressive)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
