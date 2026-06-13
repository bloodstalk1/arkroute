// Package strutil collects small string helpers shared across packages
// that have no natural home elsewhere.
package strutil

import "strings"

// FirstNonEmpty returns the first value that contains at least one
// non-whitespace rune, with surrounding whitespace stripped. Returns ""
// when none of the inputs are non-blank.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
