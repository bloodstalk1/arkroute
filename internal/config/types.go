// Package config defines arkroute's YAML configuration schema, the
// load/validate/migrate pipeline that turns a file on disk into a
// runtime [Snapshot], and the helpers used elsewhere to project a
// [Config] into provider/model/route indexes.
package config

import (
	"fmt"
	"strings"
)

const (
	// CurrentVersion is the schema version this build of arkroute writes
	// and understands. Older files are migrated to it by [Migrate].
	CurrentVersion = 1

	// DefaultServerPort is the loopback port the serve command binds to
	// when the user has not overridden it in the config file.
	DefaultServerPort = 2002
)

// LocalGatewayBaseURL returns the loopback HTTP URL of the gateway
// described by cfg (e.g. "http://127.0.0.1:2002" or "http://[::1]:2002").
// IPv6 hosts are wrapped in brackets.
func LocalGatewayBaseURL(cfg Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

// Config is the on-disk YAML representation of an arkroute deployment.
// It is loaded with [LoadFile] / [LoadBytes], validated with
// [Config.Validate], and projected into a [Snapshot] via [BuildSnapshot]
// before being handed to the runtime.
type Config struct {
	Version               int                         `yaml:"version" json:"version"`
	Server                ServerConfig                `yaml:"server" json:"server"`
	Clients               ClientsConfig               `yaml:"clients" json:"clients"`
	Providers             []ProviderConfig            `yaml:"providers" json:"providers"`
	Models                []ModelConfig               `yaml:"models" json:"models"`
	Routes                []RouteConfig               `yaml:"routes" json:"routes"`
	Profiles              map[string]string           `yaml:"profiles" json:"profiles"`
	CompatibilityPolicies []CompatibilityPolicyConfig `yaml:"compatibility_policies,omitempty" json:"compatibility_policies,omitempty"`
}

// ServerConfig is the network-facing half of [Config]. ClientKey is the
// bearer token Claude Code (and any other local CLI) must send.
type ServerConfig struct {
	Host                   string `yaml:"host" json:"host"`
	Port                   int    `yaml:"port" json:"port"`
	ClientKey              string `yaml:"client_key" json:"client_key"`
	UpstreamTimeoutSeconds int    `yaml:"upstream_timeout_seconds" json:"upstream_timeout_seconds"`
	RateLimitRPM           int    `yaml:"rate_limit_rpm" json:"rate_limit_rpm"` // 0 = disabled
}

// ClientsConfig holds per-CLI behaviour switches. New clients add
// their own sub-structs here.
type ClientsConfig struct {
	Claude ClaudeClientConfig `yaml:"claude" json:"claude"`
}

// ClaudeClientConfig toggles Claude Code integrations. ModelDiscovery
// exposes arkroute's aliases under Claude's /v1/models response.
type ClaudeClientConfig struct {
	Enabled        bool `yaml:"enabled" json:"enabled"`
	ModelDiscovery bool `yaml:"model_discovery" json:"model_discovery"`
}

// ProviderConfig describes one upstream API. Type selects the adapter
// ("openai_compatible", "anthropic", "gemini"); BaseURL and APIKey are
// adapter-specific.
type ProviderConfig struct {
	ID      string            `yaml:"id" json:"id"`
	Name    string            `yaml:"name" json:"name"`
	Type    string            `yaml:"type" json:"type"`
	BaseURL string            `yaml:"base_url" json:"base_url"`
	APIKey  string            `yaml:"api_key" json:"api_key"`
	Headers map[string]string `yaml:"headers" json:"headers"`
	Enabled bool              `yaml:"enabled" json:"enabled"`
}

// ModelConfig is a single (provider, upstream-model) exposed under a
// stable alias. Capabilities feed the router's filter; Reasoning
// overrides the upstream's default behaviour.
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

// ReasoningConfig lets operators pre-decide reasoning behaviour for a
// model so callers do not have to. Pointer fields distinguish "unset"
// from the zero value of the underlying type.
type ReasoningConfig struct {
	Mode               string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Enabled            *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Effort             string `yaml:"effort,omitempty" json:"effort,omitempty"`
	AutoEnable         *bool  `yaml:"auto_enable,omitempty" json:"auto_enable,omitempty"`
	AutoEffort         string `yaml:"auto_effort,omitempty" json:"auto_effort,omitempty"`
	Replay             *bool  `yaml:"replay,omitempty" json:"replay,omitempty"`
	OmitToolChoice     *bool  `yaml:"omit_tool_choice,omitempty" json:"omit_tool_choice,omitempty"`
	FollowClaudeEffort *bool  `yaml:"follow_claude_effort,omitempty" json:"follow_claude_effort,omitempty"`
}

// CompatibilityPolicyConfig rewrites reasoning fields for upstream
// models that need a specific shape (for example, OpenRouter's
// `reasoning.effort` rather than Anthropic's `thinking`).
type CompatibilityPolicyConfig struct {
	ID        string                       `yaml:"id" json:"id"`
	Match     CompatibilityMatchConfig     `yaml:"match" json:"match"`
	Reasoning CompatibilityReasoningConfig `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
}

// CompatibilityMatchConfig describes when a [CompatibilityPolicyConfig]
// applies. All set lists are OR-ed; all set fields within a list are
// AND-ed.
type CompatibilityMatchConfig struct {
	ProviderIDs           []string `yaml:"provider_ids,omitempty" json:"provider_ids,omitempty"`
	ProviderIDContains    []string `yaml:"provider_id_contains,omitempty" json:"provider_id_contains,omitempty"`
	ProviderTypeContains  []string `yaml:"provider_type_contains,omitempty" json:"provider_type_contains,omitempty"`
	UpstreamModels        []string `yaml:"upstream_models,omitempty" json:"upstream_models,omitempty"`
	UpstreamModelContains []string `yaml:"upstream_model_contains,omitempty" json:"upstream_model_contains,omitempty"`
	UpstreamModelPatterns []string `yaml:"upstream_model_patterns,omitempty" json:"upstream_model_patterns,omitempty"`
}

// CompatibilityReasoningConfig is the override applied when a
// compatibility policy matches.
type CompatibilityReasoningConfig struct {
	AutoEnable     *bool  `yaml:"auto_enable,omitempty" json:"auto_enable,omitempty"`
	AutoEffort     string `yaml:"auto_effort,omitempty" json:"auto_effort,omitempty"`
	Replay         *bool  `yaml:"replay,omitempty" json:"replay,omitempty"`
	OmitToolChoice *bool  `yaml:"omit_tool_choice,omitempty" json:"omit_tool_choice,omitempty"`
}

// Capabilities are the router-visible features of a model. ContextWindow
// and MaxOutputTokens are advisory and are surfaced to the admin UI.
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

// RouteConfig groups one or more [RouteTarget]s under a single alias
// the caller can request. Strategy is one of "priority", "fallback",
// "round_robin", or "weighted".
type RouteConfig struct {
	Alias                string        `yaml:"alias" json:"alias"`
	ClaudeDiscoveryAlias string        `yaml:"claude_discovery_alias" json:"claude_discovery_alias"`
	Strategy             string        `yaml:"strategy" json:"strategy"`
	Targets              []RouteTarget `yaml:"targets" json:"targets"`
	Enabled              bool          `yaml:"enabled" json:"enabled"`
}

// RouteTarget is one (model, weight) inside a [RouteConfig]. Weight is
// only consulted for the "weighted" strategy.
type RouteTarget struct {
	ModelID string `yaml:"model_id" json:"model_id"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Weight  int    `yaml:"weight" json:"weight,omitempty"`
}
