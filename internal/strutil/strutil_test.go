package strutil

import "testing"

func TestFirstNonEmptySkipsBlankAndTrims(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{"all empty", []string{"", "  ", "\t\n"}, ""},
		{"first wins", []string{"  alpha  ", "beta"}, "alpha"},
		{"skips blanks", []string{"", "\t", "  hello  ", "world"}, "hello"},
		{"no args", nil, ""},
		{"single value", []string{"   "}, ""},
		{"single non-blank", []string{" x "}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstNonEmpty(tt.input...); got != tt.want {
				t.Fatalf("FirstNonEmpty(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
