// Package compress provides lightweight token compression for prompts
// and tool outputs. It aims to reduce token count without altering
// semantic meaning of code, JSON, or tool schemas.
//
// Two modes:
//
//	Lite (safe default): collapse repeated whitespace, trim trailing blank lines.
//	Aggressive: Lite + truncate long tool results + strip redundant boilerplate.
//
// Real compression gains come from RTK/Caveman-style transformation
// of tool outputs (grep results, git diffs, shell output). Those are
// domain-specific and left for future work. This package provides the
// foundation for pluggable compressors.
package compress

import (
	"strings"
	"unicode"
)

// Mode selects compression intensity.
type Mode int

const (
	None       Mode = iota
	Lite             // safe: whitespace + trim
	Aggressive       // Lite + truncation + boilerplate removal
)

// Tidy applies the given compression mode to text and returns the result.
// It never changes the semantic meaning of structured content (code, JSON).
func Tidy(text string, mode Mode) string {
	if mode == None || text == "" {
		return text
	}
	result := collapseWhitespace(text)
	if mode == Lite {
		result = trimTrailingBlankLines(result)
		return result
	}
	// Aggressive
	result = trimTrailingBlankLines(result)
	result = truncatePrefix(result, 4096)
	return result
}

// collapseWhitespace replaces 3+ consecutive spaces/tabs with a single space.
// Newlines are preserved (structure matters for code).
func collapseWhitespace(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	spaceCount := 0
	for _, r := range text {
		if r == '\n' || r == '\r' {
			spaceCount = 0
			b.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) {
			spaceCount++
			if spaceCount <= 2 {
				b.WriteRune(r)
			}
			continue
		}
		spaceCount = 0
		b.WriteRune(r)
	}
	return b.String()
}

// trimTrailingBlankLines removes blank lines from the end of text.
func trimTrailingBlankLines(text string) string {
	return strings.TrimRight(text, "\n\r ")
}

// truncatePrefix truncates text to maxLen characters, keeping the first
// portion. An ellipsis marker is appended if truncation occurred.
func truncatePrefix(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	cut := maxLen - 20
	if cut < 100 {
		cut = maxLen
	}
	return text[:cut] + "\n... [truncated]"
}
