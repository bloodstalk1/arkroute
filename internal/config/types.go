package config

const (
	CurrentVersion    = 1
	DefaultServerPort = 2002
)

type Config struct {
	Version   int               `yaml:"version" json:"version"`
	Server    ServerConfig      `yaml:"server" json:"server"`
	Clients   ClientsConfig     `yaml:"clients" json:"clients"`
	Providers []ProviderConfig  `yaml:"providers" json:"providers"`
	Models    []ModelConfig     `yaml:"models" json:"models"`
	Routes    []RouteConfig     `yaml:"routes" json:"routes"`
	Profiles  map[string]string `yaml:"profiles" json:"profiles"`
}

type ServerConfig struct {
	Host                   string `yaml:"host" json:"host"`
	Port                   int    `yaml:"port" json:"port"`
	ClientKey              string `yaml:"client_key" json:"client_key"`
	UpstreamTimeoutSeconds int    `yaml:"upstream_timeout_seconds" json:"upstream_timeout_seconds"`
}

type ClientsConfig struct {
	Claude ClaudeClientConfig `yaml:"claude" json:"claude"`
}

type ClaudeClientConfig struct {
	Enabled        bool `yaml:"enabled" json:"enabled"`
	ModelDiscovery bool `yaml:"model_discovery" json:"model_discovery"`
}

type ProviderConfig struct {
	ID      string            `yaml:"id" json:"id"`
	Name    string            `yaml:"name" json:"name"`
	Type    string            `yaml:"type" json:"type"`
	BaseURL string            `yaml:"base_url" json:"base_url"`
	APIKey  string            `yaml:"api_key" json:"api_key"`
	Headers map[string]string `yaml:"headers" json:"headers"`
	Enabled bool              `yaml:"enabled" json:"enabled"`
}

type ModelConfig struct {
	ID                   string          `yaml:"id" json:"id"`
	ProviderID           string          `yaml:"provider_id" json:"provider_id"`
	UpstreamModel        string          `yaml:"upstream_model" json:"upstream_model"`
	Protocol             string          `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	ExposedAlias         string          `yaml:"exposed_alias" json:"exposed_alias"`
	ClaudeDiscoveryAlias string          `yaml:"claude_discovery_alias" json:"claude_discovery_alias"`
	DisplayName          string          `yaml:"display_name" json:"display_name"`
	Capabilities         Capabilities    `yaml:"capabilities" json:"capabilities"`
	Reasoning            ReasoningConfig `yaml:"reasoning" json:"reasoning"`
	Enabled              bool            `yaml:"enabled" json:"enabled"`
}

type ReasoningConfig struct {
	Mode               string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Enabled            *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Effort             string `yaml:"effort,omitempty" json:"effort,omitempty"`
	Replay             *bool  `yaml:"replay,omitempty" json:"replay,omitempty"`
	OmitToolChoice     *bool  `yaml:"omit_tool_choice,omitempty" json:"omit_tool_choice,omitempty"`
	FollowClaudeEffort *bool  `yaml:"follow_claude_effort,omitempty" json:"follow_claude_effort,omitempty"`
}

type Capabilities struct {
	Streaming       bool `yaml:"streaming" json:"streaming"`
	Tools           bool `yaml:"tools" json:"tools"`
	ToolResults     bool `yaml:"tool_results" json:"tool_results"`
	Vision          bool `yaml:"vision" json:"vision"`
	SystemMessages  bool `yaml:"system_messages" json:"system_messages"`
	PromptCache     bool `yaml:"prompt_cache" json:"prompt_cache"`
	Reasoning       bool `yaml:"reasoning" json:"reasoning"`
	ContextWindow   int  `yaml:"context_window" json:"context_window"`
	MaxOutputTokens int  `yaml:"max_output_tokens" json:"max_output_tokens"`
}

type RouteConfig struct {
	Alias                string        `yaml:"alias" json:"alias"`
	ClaudeDiscoveryAlias string        `yaml:"claude_discovery_alias" json:"claude_discovery_alias"`
	Strategy             string        `yaml:"strategy" json:"strategy"`
	Targets              []RouteTarget `yaml:"targets" json:"targets"`
	Enabled              bool          `yaml:"enabled" json:"enabled"`
}

type RouteTarget struct {
	ModelID string `yaml:"model_id" json:"model_id"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
}
