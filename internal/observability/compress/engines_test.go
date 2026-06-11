package compress

import (
	"strings"
	"testing"
)

func TestCompressGitDiff(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main
+import "fmt"
 
 func main() {
-    println("hello")
+    fmt.Println("hello world!")
     os.Exit(0)
@@ -10,3 +11,4 @@
 
 func helper() bool {
-    return false
+    return true
 }
`
	got := CompressGitDiff(diff)
	if !strings.Contains(got, "+import \"fmt\"") {
		t.Errorf("missing +import line: %s", got)
	}
	if !strings.Contains(got, "-    println") {
		t.Errorf("missing -println line: %s", got)
	}
	if strings.Contains(got, "os.Exit(0)") {
		t.Errorf("should not contain unchanged context line: %s", got)
	}
	if !strings.Contains(got, "[") || !strings.Contains(got, "unchanged") {
		t.Errorf("missing unchanged lines count marker: %s", got)
	}
}

func TestCompressGitDiffEmpty(t *testing.T) {
	got := CompressGitDiff("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestCompressGitDiffNoContext(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,2 +1,2 @@
-old
+new`
	got := CompressGitDiff(diff)
	if !strings.Contains(got, "+new") {
		t.Errorf("missing +new: %s", got)
	}
}

func TestCompressGrepOutput(t *testing.T) {
	output := `main.go:24:fmt.Println("hello")
main.go:25:fmt.Printf("world")
main.go:24:fmt.Println("hello")
helper.go:10:return nil
helper.go:12:return nil`
	got := CompressGrepOutput(output)
	if !strings.Contains(got, "main.go") {
		t.Errorf("missing filename: %s", got)
	}
	if !strings.Contains(got, "3 match") {
		t.Errorf("missing match count: %s", got)
	}
	if !strings.Contains(got, "2 unique") {
		t.Errorf("missing unique count: %s", got)
	}
	// "fmt.Println" appears twice, should be grouped
	if strings.Count(got, "fmt.Println") > 2 {
		t.Errorf("duplicate lines not deduplicated: %s", got)
	}
}

func TestCompressGrepNonGrep(t *testing.T) {
	output := "this is not grep output"
	got := CompressGrepOutput(output)
	if got != output {
		t.Errorf("non-grep should pass through: got %q", got)
	}
}

func TestCompressJSONLogs(t *testing.T) {
	input := `{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"hello","trace_id":"abc123","host":"server1"}
{"timestamp":"2024-01-01T00:00:01Z","level":"error","error":"something broke","service":"api","span_id":"def456"}`
	got := CompressJSONLogs(input)
	if strings.Contains(got, "timestamp") || strings.Contains(got, "trace_id") || strings.Contains(got, "host") {
		t.Errorf("metadata keys not stripped: %s", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "something broke") {
		t.Errorf("message content lost: %s", got)
	}
}

func TestCompressJSONLogsNotJSON(t *testing.T) {
	input := "this is not json"
	got := CompressJSONLogs(input)
	if got != input {
		t.Errorf("non-JSON should pass through: %q", got)
	}
}
