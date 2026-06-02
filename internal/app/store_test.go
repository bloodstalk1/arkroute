package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bat.dev/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

func TestFileStorePath(t *testing.T) {
	store := NewFileStore("/tmp/config.yaml")
	if store.Path() != "/tmp/config.yaml" {
		t.Fatalf("Path() = %q", store.Path())
	}
}

func TestPrintLogsTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "traces.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PrintLogsTail(path, 2, &out); err != nil {
		t.Fatalf("PrintLogsTail() error = %v", err)
	}
	if out.String() != "two\nthree\n" {
		t.Fatalf("out = %q", out.String())
	}
}

func TestDoctorMissingEnvReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.MinimalValidConfig("local-key")
	cfg.Server.Port = 1
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Doctor(path, &out); err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !strings.Contains(out.String(), "env:OPENROUTER_API_KEY missing") {
		t.Fatalf("doctor output = %q", out.String())
	}
	if !strings.Contains(out.String(), "server: unreachable") {
		t.Fatalf("doctor output = %q", out.String())
	}
	if !strings.Contains(out.String(), "port:") {
		t.Fatalf("doctor output = %q", out.String())
	}
}
