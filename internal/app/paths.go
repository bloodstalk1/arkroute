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
		return ".arkroute/config.yaml"
	}
	return filepath.Join(home, ".arkroute", "config.yaml")
}

func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkroute/traces.jsonl"
	}
	return filepath.Join(home, ".arkroute", "traces.jsonl")
}
