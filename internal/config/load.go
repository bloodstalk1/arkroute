package config

import (
	"fmt"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

func LoadFile(path string) (Config, error) {
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err == nil {
			if !info.IsDir() {
				if mode := info.Mode().Perm(); mode&0o077 != 0 {
					return Config{}, fmt.Errorf("config file %s has permissive mode %#o (expected 0o600 or stricter): refusing to load", path, mode)
				}
			}
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return LoadBytes(data)
}

func LoadBytes(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg, err := Migrate(cfg)
	if err != nil {
		return Config{}, err
	}
	ApplyDefaults(&cfg)
	return cfg, nil
}

func ApplyDefaults(cfg *Config) {
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = DefaultServerPort
	}
	if cfg.Server.UpstreamTimeoutSeconds == 0 {
		cfg.Server.UpstreamTimeoutSeconds = 600
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]string{}
	}
}

func MinimalValidConfig(clientKey string) Config {
	return Config{
		Version: CurrentVersion,
		Server: ServerConfig{
			Host:                   "127.0.0.1",
			Port:                   DefaultServerPort,
			ClientKey:              clientKey,
			UpstreamTimeoutSeconds: 600,
		},
		Clients: ClientsConfig{Claude: ClaudeClientConfig{Enabled: true, ModelDiscovery: true}},
		Providers: []ProviderConfig{{
			ID:      "openrouter",
			Name:    "OpenRouter",
			Type:    "openai_compatible",
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  "env:OPENROUTER_API_KEY",
			Headers: map[string]string{"X-OpenRouter-Title": "Arkroute"},
			Enabled: true,
		}},
		Models: []ModelConfig{{
			ID:                   "openrouter-sonnet",
			ProviderID:           "openrouter",
			UpstreamModel:        "anthropic/claude-sonnet-4.5",
			ExposedAlias:         "sonnet-or",
			ClaudeDiscoveryAlias: "claude-sonnet-4-20250514",
			DisplayName:          "Claude Sonnet via OpenRouter",
			Capabilities: Capabilities{
				Streaming:       true,
				Tools:           true,
				ToolResults:     true,
				SystemMessages:  true,
				ContextWindow:   200000,
				MaxOutputTokens: 8192,
			},
			Enabled: true,
		}},
		Routes: []RouteConfig{{
			Alias:                "sonnet",
			ClaudeDiscoveryAlias: "claude-sonnet-4-20250514",
			Strategy:             "fallback",
			Targets:              []RouteTarget{{ModelID: "openrouter-sonnet", Enabled: true}},
			Enabled:              true,
		}},
		Profiles: map[string]string{"default": "sonnet", "best": "sonnet"},
	}
}

func BootstrapLocalConfig(clientKey string) Config {
	return Config{
		Version: CurrentVersion,
		Server: ServerConfig{
			Host:                   "127.0.0.1",
			Port:                   DefaultServerPort,
			ClientKey:              clientKey,
			UpstreamTimeoutSeconds: 600,
		},
		Clients:   ClientsConfig{Claude: ClaudeClientConfig{Enabled: true, ModelDiscovery: true}},
		Providers: []ProviderConfig{},
		Models:    []ModelConfig{},
		Routes:    []RouteConfig{},
		Profiles:  map[string]string{},
	}
}
