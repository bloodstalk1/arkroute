package app

import (
	"fmt"
	"os"
	"path/filepath"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/security"
	"gopkg.in/yaml.v3"
)

func InitConfig(path string, force bool) (string, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return "", fmt.Errorf("%s already exists", path)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	key, err := security.GenerateClientKey()
	if err != nil {
		return "", err
	}
	cfg := config.MinimalValidConfig(key)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
