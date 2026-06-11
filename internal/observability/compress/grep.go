package compress

import (
	"sort"
	"strings"
)

// CompressGrepOutput compresses grep results by grouping matches by file
// and deduplicating identical match lines within each file.
//
// Input format: standard grep -nH output (filename:lineno:match line)
//
// Strategy:
//   - Group by filename
//   - Deduplicate identical match text within each file
//   - Report "filename: N matches (M unique)" + first 5 unique lines
//
// Typical savings: 50-80% on large grep output.
func CompressGrepOutput(output string) string {
	if !looksLikeGrep(output) {
		return output
	}

	type fileSummary struct {
		total  int
		unique map[string]int // line text -> count
	}

	files := map[string]*fileSummary{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		filename := parts[0]
		matchText := strings.TrimSpace(parts[2])

		fs := files[filename]
		if fs == nil {
			fs = &fileSummary{unique: map[string]int{}}
			files[filename] = fs
		}
		fs.total++
		fs.unique[matchText]++
	}

	if len(files) == 0 {
		return output
	}

	var b strings.Builder
	b.WriteString("[grep output compressed]\n")
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fs := files[name]
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(itoa(fs.total))
		b.WriteString(" match")
		if fs.total > 1 {
			b.WriteString("es")
		}
		b.WriteString(" (")
		b.WriteString(itoa(len(fs.unique)))
		b.WriteString(" unique)\n")

		type kv struct{ k string; v int }
		var sorted []kv
		for k, v := range fs.unique {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })

		show := len(sorted)
		if show > 5 {
			show = 5
		}
		for i := 0; i < show; i++ {
			if sorted[i].v > 1 {
				b.WriteString("  ")
				b.WriteString(itoa(sorted[i].v))
				b.WriteString("x: ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(sorted[i].k)
			b.WriteByte('\n')
		}
		if len(sorted) > 5 {
			b.WriteString("  ... +")
			b.WriteString(itoa(len(sorted) - 5))
			b.WriteString(" more unique lines\n")
		}
	}
	return b.String()
}

func looksLikeGrep(output string) bool {
	lines := strings.SplitN(strings.TrimSpace(output), "\n", 5)
	colonLines := 0
	for _, line := range lines {
		if strings.Count(line, ":") >= 2 {
			colonLines++
		}
	}
	return colonLines >= 2
}
