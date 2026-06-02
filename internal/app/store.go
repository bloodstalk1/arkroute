package app

import (
	"os"
	"path/filepath"

	"bat.dev/arkroute/internal/config"
	"gopkg.in/yaml.v3"
)

type Store interface {
	Path() string
	Load() (config.Config, error)
	Save(config.Config) error
}

type FileStore struct {
	path string
}

func NewFileStore(path string) FileStore {
	if path == "" {
		path = DefaultConfigPath()
	}
	return FileStore{path: path}
}

func (s FileStore) Path() string {
	return s.path
}

func (s FileStore) Load() (config.Config, error) {
	return config.LoadFile(s.path)
}

func (s FileStore) Save(cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
