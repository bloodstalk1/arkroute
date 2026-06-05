package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bloodstalk1/arkroute/internal/config"
)

const (
	claudeEnvBaseURL        = "ANTHROPIC_BASE_URL"
	claudeEnvAuthToken      = "ANTHROPIC_AUTH_TOKEN"
	claudeEnvAPIKey         = "ANTHROPIC_API_KEY"
	claudeEnvModelDiscovery = "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"
)

type ClaudeSettingsDiagnosis struct {
	Path            string
	Exists          bool
	HasBaseURL      bool
	BaseURL         string
	ExpectedBaseURL string
	BaseURLMismatch bool
}

func DefaultClaudeSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".claude", "settings.json")
	}
	return filepath.Join(home, ".claude", "settings.json")
}

func ClaudeSettingsPath(path string) string {
	if path == "" {
		return DefaultClaudeSettingsPath()
	}
	return path
}

func WriteClaudeSettings(path string, cfg config.Config) error {
	path = ClaudeSettingsPath(path)
	settings, err := readClaudeSettings(path)
	if errors.Is(err, os.ErrNotExist) {
		settings = map[string]any{}
		err = nil
	}
	if err != nil {
		return err
	}
	env := mapFromAny(settings["env"])
	env[claudeEnvBaseURL] = claudeBaseURL(cfg)
	env[claudeEnvAuthToken] = cfg.Server.ClientKey
	env[claudeEnvAPIKey] = cfg.Server.ClientKey
	env[claudeEnvModelDiscovery] = "1"
	settings["env"] = env

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func DiagnoseClaudeSettings(path string, cfg config.Config) (ClaudeSettingsDiagnosis, error) {
	path = ClaudeSettingsPath(path)
	diagnosis := ClaudeSettingsDiagnosis{Path: path, ExpectedBaseURL: claudeBaseURL(cfg)}
	settings, err := readClaudeSettings(path)
	if errors.Is(err, os.ErrNotExist) {
		return diagnosis, nil
	}
	if err != nil {
		return diagnosis, err
	}
	diagnosis.Exists = true
	env := mapFromAny(settings["env"])
	value, ok := env[claudeEnvBaseURL].(string)
	if !ok || value == "" {
		return diagnosis, nil
	}
	diagnosis.HasBaseURL = true
	diagnosis.BaseURL = value
	diagnosis.BaseURLMismatch = value != diagnosis.ExpectedBaseURL
	return diagnosis, nil
}

func claudeBaseURL(cfg config.Config) string {
	return fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
}

func readClaudeSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, err
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

func RemoveClaudeSettings(path string, cfg config.Config) (bool, error) {
	path = ClaudeSettingsPath(path)
	settings, err := readClaudeSettings(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	env := mapFromAny(settings["env"])
	if env[claudeEnvBaseURL] != claudeBaseURL(cfg) || env[claudeEnvAuthToken] != cfg.Server.ClientKey {
		return false, nil
	}
	delete(env, claudeEnvBaseURL)
	delete(env, claudeEnvAuthToken)
	delete(env, claudeEnvAPIKey)
	delete(env, claudeEnvModelDiscovery)
	settings["env"] = env
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	return true, os.WriteFile(path, data, 0o600)
}

func mapFromAny(value any) map[string]any {
	if existing, ok := value.(map[string]any); ok {
		return existing
	}
	return map[string]any{}
}
