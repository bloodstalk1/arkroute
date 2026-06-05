package config

import "fmt"

type MigrationError struct {
	Version int
}

func (e MigrationError) Error() string {
	return fmt.Sprintf("unsupported config version %d", e.Version)
}

func Migrate(cfg Config) (Config, error) {
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Version != CurrentVersion {
		return Config{}, MigrationError{Version: cfg.Version}
	}
	return cfg, nil
}
