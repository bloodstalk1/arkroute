package compress

import (
	"strings"
)

// CompressGitDiff compresses a unified diff by removing unchanged context
// lines and keeping only additions (+), deletions (-), hunk headers (@@),
// and the file header. This is safe because AI agents typically only need
// to see what changed, not the surrounding context.
//
// Strategy:
//   - Keep diff --git / index / --- / +++ headers (file identity)
//   - Keep @@ hunk headers (location)
//   - Keep all + and - lines (actual changes)
//   - Remove unchanged context lines (lines starting with space)
//   - Remove empty lines between hunks
//
// Typical savings: 40-60% on large diffs.
func CompressGitDiff(diff string) string {
	var b strings.Builder
	b.Grow(len(diff) / 2) // pre-allocate half size

	lines := strings.Split(diff, "\n")
	kept := 0
	skipped := 0
	consecutiveSkipped := 0

	for i, line := range lines {
		isLast := i == len(lines)-1
		trimmed := strings.TrimRight(line, "\r")

		switch {
		case isHeaderLine(trimmed):
			// Always keep: diff --git, index, ---, +++, @@
			if consecutiveSkipped > 0 && kept > 0 {
				b.WriteString("... [" + itoa(skipped) + " unchanged lines]\n")
			}
			b.WriteString(trimmed)
			if !isLast {
				b.WriteByte('\n')
			}
			kept++
			consecutiveSkipped = 0
			skipped = 0

		case strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-"):
			// Keep all change lines.
			if consecutiveSkipped > 0 && kept > 0 {
				b.WriteString("... [" + itoa(skipped) + " unchanged lines]\n")
				skipped = 0
			}
			b.WriteString(trimmed)
			if !isLast {
				b.WriteByte('\n')
			}
			kept++
			consecutiveSkipped = 0

		default:
			// Unchanged context line — skip.
			skipped++
			consecutiveSkipped++
		}
	}

	if consecutiveSkipped > 0 && kept > 0 {
		b.WriteString("\n... [" + itoa(skipped) + " unchanged lines]")
	}
	return b.String()
}

func isHeaderLine(line string) bool {
	return strings.HasPrefix(line, "diff --git") ||
		strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "--- ") ||
		strings.HasPrefix(line, "+++ ") ||
		strings.HasPrefix(line, "@@")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
