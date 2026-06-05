package config

import "testing"

func TestMigrateVersion1(t *testing.T) {
	cfg := MinimalValidConfig("local-key")
	migrated, err := Migrate(cfg)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if migrated.Version != 1 {
		t.Fatalf("version = %d, want 1", migrated.Version)
	}
}

func TestMigrateUnknownVersion(t *testing.T) {
	cfg := MinimalValidConfig("local-key")
	cfg.Version = 99
	_, err := Migrate(cfg)
	if err == nil {
		t.Fatal("Migrate() error = nil, want error")
	}
}
