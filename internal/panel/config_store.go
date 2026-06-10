package panel

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
	"gopkg.in/yaml.v3"
)

const defaultBackupLimit = 20

type ConfigStore struct {
	Path        string
	BackupDir   string
	BackupLimit int
	Now         func() time.Time
}

type ConfigSaveResult struct {
	BackupPath string `json:"backup_path,omitempty"`
}

func NewConfigStore(path string) ConfigStore {
	return ConfigStore{Path: path, BackupLimit: defaultBackupLimit}
}

func (s ConfigStore) LoadOrBootstrap() (config.Config, error) {
	cfg, err := config.LoadFile(s.Path)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, err
	}
	key, err := security.GenerateClientKey()
	if err != nil {
		return config.Config{}, err
	}
	return config.BootstrapLocalConfig(key), nil
}

func (s ConfigStore) ParseImport(data []byte) (config.Config, error) {
	cfg, err := config.LoadBytes(data)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func (s ConfigStore) Export(redacted bool) ([]byte, error) {
	cfg, err := config.LoadFile(s.Path)
	if err != nil {
		return nil, err
	}
	if redacted {
		cfg = config.Redacted(cfg)
	}
	return yaml.Marshal(cfg)
}

func (s ConfigStore) Save(cfg config.Config) (ConfigSaveResult, error) {
	if err := cfg.Validate(); err != nil {
		return ConfigSaveResult{}, err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return ConfigSaveResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return ConfigSaveResult{}, err
	}
	backupPath, err := s.createBackup()
	if err != nil {
		return ConfigSaveResult{}, err
	}
	if err := atomicWriteFile(s.Path, data, 0o600); err != nil {
		return ConfigSaveResult{}, err
	}
	if err := s.PruneBackups(); err != nil {
		return ConfigSaveResult{}, err
	}
	return ConfigSaveResult{BackupPath: backupPath}, nil
}

func (s ConfigStore) SaveAndReload(cfg config.Config, onSave func() error) (ConfigSaveResult, error) {
	if err := cfg.Validate(); err != nil {
		return ConfigSaveResult{}, err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return ConfigSaveResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return ConfigSaveResult{}, err
	}
	hasPrevious := true
	if _, err := os.Stat(s.Path); errors.Is(err, os.ErrNotExist) {
		hasPrevious = false
	}
	backupPath, err := s.createBackup()
	if err != nil {
		return ConfigSaveResult{}, err
	}
	if err := atomicWriteFile(s.Path, data, 0o600); err != nil {
		return ConfigSaveResult{}, err
	}
	if onSave != nil {
		if err := onSave(); err != nil {
			if rollbackErr := s.rollbackSave(hasPrevious, backupPath); rollbackErr != nil {
				return ConfigSaveResult{}, fmt.Errorf("reload failed: %w; rollback failed: %v", err, rollbackErr)
			}
			return ConfigSaveResult{}, fmt.Errorf("reload failed: %w (config rolled back)", err)
		}
	}
	if err := s.PruneBackups(); err != nil {
		return ConfigSaveResult{BackupPath: backupPath}, err
	}
	return ConfigSaveResult{BackupPath: backupPath}, nil
}

func (s ConfigStore) rollbackSave(hasPrevious bool, backupPath string) error {
	if !hasPrevious {
		if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove new config: %w", err)
		}
		return nil
	}
	if backupPath == "" {
		return errors.New("missing backup for previous config")
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	if err := atomicWriteFile(s.Path, backupData, 0o600); err != nil {
		return fmt.Errorf("restore backup: %w", err)
	}
	return nil
}

func (s ConfigStore) PruneBackups() error {
	limit := s.limit()
	if limit < 1 {
		return nil
	}
	dir := s.backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "config-") && strings.HasSuffix(name, ".yaml") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) <= limit {
		return nil
	}
	for _, name := range names[:len(names)-limit] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func (s ConfigStore) createBackup() (string, error) {
	current, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	dir := s.backupDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := "config-" + s.now().Format("20060102-150405") + ".yaml"
	path := filepath.Join(dir, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("config-%s-%02d.yaml", s.now().Format("20060102-150405"), i))
	}
	if err := os.WriteFile(path, current, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s ConfigStore) backupDir() string {
	if s.BackupDir != "" {
		return s.BackupDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(filepath.Dir(s.Path), "backups")
	}
	return filepath.Join(home, ".arkroute", "backups")
}

func (s ConfigStore) limit() int {
	if s.BackupLimit == 0 {
		return defaultBackupLimit
	}
	return s.BackupLimit
}

func (s ConfigStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
