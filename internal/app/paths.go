package app

import (
	"os"
	"path/filepath"
)

func DefaultConfigPath() string {
	if override := os.Getenv("ARKROUTER_CONFIG"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkrouter/config.yaml"
	}
	return filepath.Join(home, ".arkrouter", "config.yaml")
}

func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkrouter/traces.jsonl"
	}
	return filepath.Join(home, ".arkrouter", "traces.jsonl")
}
