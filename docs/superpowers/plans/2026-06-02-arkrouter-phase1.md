# Arkrouter Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Arkrouter phase 1: a local Go router that lets Claude Code CLI use OpenAI-compatible, Gemini, and Anthropic upstreams through Anthropic-compatible endpoints.

**Architecture:** The core is protocol-neutral: Claude handlers decode Anthropic wire requests into normalized protocol structs, the router resolves aliases and targets from an immutable snapshot, and provider adapters map normalized requests to upstream APIs. Config, health, logs, and adapters are separate packages so future OpenCode/Cursor/Codex ingress can be added without rewriting the core.

**Tech Stack:** Go 1.23+, standard `net/http`, `gopkg.in/yaml.v3`, JSONL trace logs, table-driven tests, `httptest` fake upstreams.

---

## Source Spec

Implement from `docs/superpowers/specs/2026-06-02-arkrouter-design.md`.

Phase 1 acceptance criteria:

- `arkrouter init` creates a valid local config.
- `arkrouter validate` accepts generated config and rejects invalid references.
- `arkrouter activate claude` prints working Claude Code environment exports.
- `arkrouter serve` starts on loopback with the configured client key.
- `/v1/models` returns route aliases and Claude discovery aliases.
- `/v1/messages` can complete non-streaming and streaming OpenAI-compatible upstream requests.
- Tool use round-trips through an OpenAI-compatible provider fixture.
- Fallback tries the second target on timeout, `429`, and `5xx`.
- Auth/config failures do not fallback.
- Logs show routing, latency, status, and fallback reason without prompt or secret data.
- `go test ./...` passes.

## File Structure

Create these files and keep responsibilities narrow:

- `go.mod`: Go module `bat.dev/arkrouter`.
- `README.md`: local usage, Claude activation, config example, development commands.
- `cmd/arkrouter/main.go`: process entrypoint only; delegates to `internal/cli`.
- `internal/cli/cli.go`: command dispatch, stdout/stderr wiring, exit codes.
- `internal/cli/cli_test.go`: command behavior with temp config paths.
- `internal/app/paths.go`: config/log path defaults and path overrides.
- `internal/app/init.go`: generated default config and secure file write.
- `internal/app/activate.go`: Claude Code env export and optional settings writer.
- `internal/app/serve.go`: load config, build runtime, start HTTP server.
- `internal/app/commands.go`: validate/status/doctor/logs/test command implementations.
- `internal/config/types.go`: YAML config structs and constants.
- `internal/config/load.go`: YAML decode, env secret resolution, default handling.
- `internal/config/validate.go`: validation and redacted validation errors.
- `internal/config/snapshot.go`: immutable runtime snapshot maps.
- `internal/config/config_test.go`: config validation and snapshot tests.
- `internal/security/keys.go`: random local client key generation, shell quoting, constant-time auth compare.
- `internal/security/redact.go`: redaction helpers for logs and errors.
- `internal/security/security_test.go`: key generation, quoting, redaction tests.
- `internal/protocol/normalized.go`: provider-neutral request, response, stream, capabilities.
- `internal/protocol/anthropic/types.go`: Anthropic Messages wire structs.
- `internal/protocol/anthropic/codec.go`: decode/encode Anthropic requests and errors.
- `internal/protocol/anthropic/models.go`: `/v1/models` response structs.
- `internal/protocol/anthropic/codec_test.go`: request decode, response encode, error shape tests.
- `internal/protocol/openai/types.go`: OpenAI chat/completions wire structs.
- `internal/router/router.go`: route resolution, capability filtering, target ordering.
- `internal/router/health.go`: thread-safe health store.
- `internal/router/errors.go`: retry classification and public error categories.
- `internal/router/router_test.go`: alias, capability, fallback ordering tests.
- `internal/adapter/adapter.go`: provider adapter interface and shared request/response structs.
- `internal/adapter/openai/url.go`: OpenAI-compatible URL normalization.
- `internal/adapter/openai/mapper.go`: normalized request/response mapping.
- `internal/adapter/openai/stream.go`: OpenAI SSE to normalized stream mapper.
- `internal/adapter/openai/openai_test.go`: non-streaming, streaming, tool, URL tests.
- `internal/adapter/gemini/mapper.go`: Gemini phase 1 text, streaming, basic tool mapping.
- `internal/adapter/gemini/gemini_test.go`: Gemini mapping fixtures.
- `internal/adapter/anthropic/passthrough.go`: Anthropic direct passthrough adapter.
- `internal/adapter/anthropic/passthrough_test.go`: model rewrite and header tests.
- `internal/client/claude/server.go`: HTTP routes and shared dependencies.
- `internal/client/claude/auth.go`: local client key auth middleware.
- `internal/client/claude/models.go`: `/v1/models`.
- `internal/client/claude/messages.go`: `/v1/messages` and `/v1/messages/count_tokens`.
- `internal/client/claude/stream.go`: write Anthropic-compatible SSE events.
- `internal/client/claude/server_test.go`: HTTP integration tests with fake upstreams.
- `internal/observability/trace.go`: trace event structs and JSONL writer.
- `internal/observability/trace_test.go`: JSONL and redaction tests.
- `internal/testutil/upstream.go`: fake upstream helpers for integration tests.

## Implementation Rules

- Do not add dashboard, OpenAI ingress, cloud sync, OAuth, MCP, or token compression in phase 1.
- Keep handlers and adapters small; move wire structs into `internal/protocol`.
- Use request contexts and explicit HTTP timeouts for all upstream calls.
- Never log prompt bodies, response bodies, API keys, or authorization headers.
- Commit after each task.
- Run `gofmt -w .` and `go test ./...` before each commit.

---

### Task 1: Repository Scaffold And CLI Shell

**Files:**
- Create: `go.mod`
- Create: `README.md`
- Create: `cmd/arkrouter/main.go`
- Create: `internal/cli/cli.go`
- Create: `internal/cli/cli_test.go`

- [ ] **Step 1: Create the Go module**

Create `go.mod`:

```go
module bat.dev/arkrouter

go 1.23

require gopkg.in/yaml.v3 v3.0.1
```

- [ ] **Step 2: Write CLI behavior tests**

Create `internal/cli/cli_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage: arkrouter <command>") {
		t.Fatalf("stdout missing usage: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "missing"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: missing") {
		t.Fatalf("stderr missing unknown command: %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != "arkrouter dev" {
		t.Fatalf("stdout = %q, want version", stdout.String())
	}
}
```

- [ ] **Step 3: Run the failing CLI tests**

Run: `go test ./internal/cli`

Expected: fails because `internal/cli` does not contain `Run`.

- [ ] **Step 4: Implement CLI shell**

Create `internal/cli/cli.go`:

```go
package cli

import (
	"fmt"
	"io"
)

const version = "dev"

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) < 2 {
		printHelp(stdout)
		return 0
	}

	switch args[1] {
	case "version":
		fmt.Fprintf(stdout, "arkrouter %s\n", version)
		return 0
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[1])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: arkrouter <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init              Create a local config")
	fmt.Fprintln(w, "  validate          Validate config")
	fmt.Fprintln(w, "  serve             Start the local Claude Code gateway")
	fmt.Fprintln(w, "  activate claude   Print Claude Code environment exports")
	fmt.Fprintln(w, "  status            Show route and upstream health")
	fmt.Fprintln(w, "  doctor            Diagnose local setup")
	fmt.Fprintln(w, "  test              Test a model route")
	fmt.Fprintln(w, "  logs              Print JSONL trace logs")
	fmt.Fprintln(w, "  version           Print version")
}
```

Create `cmd/arkrouter/main.go`:

```go
package main

import (
	"os"

	"bat.dev/arkrouter/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdout, os.Stderr))
}
```

- [ ] **Step 5: Add README baseline**

Create `README.md`:

```markdown
# Arkrouter

Arkrouter is a local AI model router for coding tools. Phase 1 focuses on Claude Code CLI through an Anthropic-compatible local gateway.

## Development

```sh
go test ./...
go run ./cmd/arkrouter help
```

## Phase 1 Commands

```sh
arkrouter init
arkrouter validate
arkrouter serve
eval "$(arkrouter activate claude)"
claude
```
```

- [ ] **Step 6: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
go run ./cmd/arkrouter version
```

Expected:

```text
ok  	bat.dev/arkrouter/internal/cli
arkrouter dev
```

Commit:

```sh
git add go.mod README.md cmd/arkrouter/main.go internal/cli/cli.go internal/cli/cli_test.go
git commit -m "chore: scaffold arkrouter cli"
```

---

### Task 2: Config Types, Validation, And Snapshots

**Files:**
- Create: `internal/config/types.go`
- Create: `internal/config/load.go`
- Create: `internal/config/validate.go`
- Create: `internal/config/snapshot.go`
- Create: `internal/config/config_test.go`
- Create: `internal/security/redact.go`
- Create: `internal/security/security_test.go`

- [ ] **Step 1: Write config validation tests**

Create `internal/config/config_test.go`:

```go
package config

import (
	"strings"
	"testing"
)

func TestValidateAcceptsMinimalGeneratedConfig(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsNonLoopbackHost(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Server.Host = "0.0.0.0"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "server.host") {
		t.Fatalf("error = %q, want server.host", err.Error())
	}
}

func TestValidateRejectsBrokenReferences(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	cfg.Models[0].ProviderID = "missing"
	cfg.Routes[0].Targets[0].ModelID = "missing-model"
	cfg.Profiles["default"] = "missing-route"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	for _, want := range []string{"models[0].provider_id", "routes[0].targets[0].model_id", "profiles.default"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %s", err.Error(), want)
		}
	}
}

func TestBuildSnapshotIndexesAliases(t *testing.T) {
	cfg := MinimalValidConfig("ark-local-key")
	snapshot, err := BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	if _, ok := snapshot.RoutesByAlias["sonnet"]; !ok {
		t.Fatal("RoutesByAlias missing sonnet")
	}
	if _, ok := snapshot.RoutesByDiscoveryAlias["claude-sonnet-4-20250514"]; !ok {
		t.Fatal("RoutesByDiscoveryAlias missing Claude alias")
	}
	if _, ok := snapshot.ModelsByExposedAlias["sonnet-or"]; !ok {
		t.Fatal("ModelsByExposedAlias missing sonnet-or")
	}
}
```

- [ ] **Step 2: Write redaction tests**

Create `internal/security/security_test.go`:

```go
package security

import "testing"

func TestRedactSecret(t *testing.T) {
	got := Redact("sk-or-secret")
	if got != "[redacted]" {
		t.Fatalf("Redact() = %q, want [redacted]", got)
	}
}

func TestRedactMap(t *testing.T) {
	headers := map[string]string{
		"Authorization":      "Bearer secret",
		"X-OpenRouter-Title": "Arkrouter",
	}
	got := RedactMap(headers)
	if got["Authorization"] != "[redacted]" {
		t.Fatalf("Authorization = %q, want redacted", got["Authorization"])
	}
	if got["X-OpenRouter-Title"] != "Arkrouter" {
		t.Fatalf("X-OpenRouter-Title = %q", got["X-OpenRouter-Title"])
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/config ./internal/security`

Expected: fails because config and security packages are not implemented.

- [ ] **Step 4: Implement config structs**

Create `internal/config/types.go`:

```go
package config

const CurrentVersion = 1

type Config struct {
	Version   int              `yaml:"version" json:"version"`
	Server    ServerConfig     `yaml:"server" json:"server"`
	Clients   ClientsConfig    `yaml:"clients" json:"clients"`
	Providers []ProviderConfig `yaml:"providers" json:"providers"`
	Models    []ModelConfig    `yaml:"models" json:"models"`
	Routes    []RouteConfig    `yaml:"routes" json:"routes"`
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
	ID                   string       `yaml:"id" json:"id"`
	ProviderID           string       `yaml:"provider_id" json:"provider_id"`
	UpstreamModel        string       `yaml:"upstream_model" json:"upstream_model"`
	ExposedAlias         string       `yaml:"exposed_alias" json:"exposed_alias"`
	ClaudeDiscoveryAlias string       `yaml:"claude_discovery_alias" json:"claude_discovery_alias"`
	DisplayName          string       `yaml:"display_name" json:"display_name"`
	Capabilities         Capabilities `yaml:"capabilities" json:"capabilities"`
	Enabled              bool         `yaml:"enabled" json:"enabled"`
}

type Capabilities struct {
	Streaming       bool `yaml:"streaming" json:"streaming"`
	Tools           bool `yaml:"tools" json:"tools"`
	ToolResults     bool `yaml:"tool_results" json:"tool_results"`
	Vision          bool `yaml:"vision" json:"vision"`
	SystemMessages  bool `yaml:"system_messages" json:"system_messages"`
	PromptCache     bool `yaml:"prompt_cache" json:"prompt_cache"`
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
```

- [ ] **Step 5: Implement minimal config defaults**

Create `internal/config/load.go`:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
		cfg.Server.Port = 20128
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
			Port:                   20128,
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
			Headers: map[string]string{"X-OpenRouter-Title": "Arkrouter"},
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
```

- [ ] **Step 6: Implement validation and snapshots**

Create `internal/config/validate.go` and `internal/config/snapshot.go` with these public contracts:

```go
package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type ValidationError struct {
	Fields map[string]string
}

func (e ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for key := range e.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+e.Fields[key])
	}
	return "config validation failed: " + strings.Join(parts, "; ")
}

func (cfg Config) Validate() error {
	fields := map[string]string{}
	if cfg.Version != CurrentVersion {
		fields["version"] = "must be 1"
	}
	if cfg.Server.Host != "127.0.0.1" && cfg.Server.Host != "localhost" && cfg.Server.Host != "::1" {
		fields["server.host"] = "must be loopback"
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		fields["server.port"] = "must be between 1 and 65535"
	}
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		fields["server.client_key"] = "must be non-empty"
	}

	providers := map[string]ProviderConfig{}
	enabledProviders := map[string]ProviderConfig{}
	for i, provider := range cfg.Providers {
		path := fmt.Sprintf("providers[%d]", i)
		if provider.ID == "" {
			fields[path+".id"] = "must be non-empty"
		} else if _, exists := providers[provider.ID]; exists {
			fields[path+".id"] = "must be unique"
		}
		if provider.Type != "openai_compatible" && provider.Type != "gemini" && provider.Type != "anthropic" {
			fields[path+".type"] = "unsupported provider type"
		}
		if _, err := url.ParseRequestURI(provider.BaseURL); err != nil {
			fields[path+".base_url"] = "must be an absolute URL"
		}
		providers[provider.ID] = provider
		if provider.Enabled {
			enabledProviders[provider.ID] = provider
		}
	}

	models := map[string]ModelConfig{}
	enabledModels := map[string]ModelConfig{}
	exposedAliases := map[string]string{}
	discoveryAliases := map[string]string{}
	for i, model := range cfg.Models {
		path := fmt.Sprintf("models[%d]", i)
		if model.ID == "" {
			fields[path+".id"] = "must be non-empty"
		} else if _, exists := models[model.ID]; exists {
			fields[path+".id"] = "must be unique"
		}
		if model.Enabled {
			if _, ok := enabledProviders[model.ProviderID]; !ok {
				fields[path+".provider_id"] = "must reference an enabled provider"
			}
		}
		if model.UpstreamModel == "" {
			fields[path+".upstream_model"] = "must be non-empty"
		}
		if model.ExposedAlias == "" {
			fields[path+".exposed_alias"] = "must be non-empty"
		} else if owner, exists := exposedAliases[model.ExposedAlias]; exists {
			fields[path+".exposed_alias"] = "must be unique; already used by " + owner
		}
		exposedAliases[model.ExposedAlias] = path
		validateDiscoveryAlias(fields, discoveryAliases, path+".claude_discovery_alias", model.ClaudeDiscoveryAlias)
		models[model.ID] = model
		if model.Enabled {
			enabledModels[model.ID] = model
		}
	}

	routeAliases := map[string]string{}
	for i, route := range cfg.Routes {
		path := fmt.Sprintf("routes[%d]", i)
		if route.Alias == "" {
			fields[path+".alias"] = "must be non-empty"
		} else if owner, exists := routeAliases[route.Alias]; exists {
			fields[path+".alias"] = "must be unique; already used by " + owner
		}
		routeAliases[route.Alias] = path
		if route.Strategy != "priority" && route.Strategy != "fallback" {
			fields[path+".strategy"] = "must be priority or fallback"
		}
		if len(route.Targets) == 0 {
			fields[path+".targets"] = "must contain at least one target"
		}
		for j, target := range route.Targets {
			if route.Enabled && target.Enabled {
				if _, ok := enabledModels[target.ModelID]; !ok {
					fields[fmt.Sprintf("%s.targets[%d].model_id", path, j)] = "must reference an enabled model"
				}
			}
		}
		validateDiscoveryAlias(fields, discoveryAliases, path+".claude_discovery_alias", route.ClaudeDiscoveryAlias)
	}

	for name, alias := range cfg.Profiles {
		if _, routeOK := routeAliases[alias]; !routeOK {
			if _, modelOK := exposedAliases[alias]; !modelOK {
				fields["profiles."+name] = "must reference a route alias or exposed model alias"
			}
		}
	}
	if len(fields) > 0 {
		return ValidationError{Fields: fields}
	}
	return nil
}

func validateDiscoveryAlias(fields map[string]string, seen map[string]string, path string, value string) {
	if value == "" {
		return
	}
	if !strings.HasPrefix(value, "claude") && !strings.HasPrefix(value, "anthropic") {
		fields[path] = "must start with claude or anthropic"
	}
	if owner, exists := seen[value]; exists {
		fields[path] = "must be unique; already used by " + owner
	}
	seen[value] = path
}
```

Create `internal/config/snapshot.go`:

```go
package config

import "time"

type Snapshot struct {
	LoadedAt               time.Time
	Config                 Config
	ProvidersByID          map[string]ProviderConfig
	ModelsByID             map[string]ModelConfig
	ModelsByExposedAlias   map[string]ModelConfig
	RoutesByAlias          map[string]RouteConfig
	RoutesByDiscoveryAlias map[string]RouteConfig
}

func BuildSnapshot(cfg Config) (Snapshot, error) {
	if err := cfg.Validate(); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{
		LoadedAt:               time.Now().UTC(),
		Config:                 cfg,
		ProvidersByID:          map[string]ProviderConfig{},
		ModelsByID:             map[string]ModelConfig{},
		ModelsByExposedAlias:   map[string]ModelConfig{},
		RoutesByAlias:          map[string]RouteConfig{},
		RoutesByDiscoveryAlias: map[string]RouteConfig{},
	}
	for _, provider := range cfg.Providers {
		if provider.Enabled {
			s.ProvidersByID[provider.ID] = provider
		}
	}
	for _, model := range cfg.Models {
		if model.Enabled {
			s.ModelsByID[model.ID] = model
			s.ModelsByExposedAlias[model.ExposedAlias] = model
		}
	}
	for _, route := range cfg.Routes {
		if route.Enabled {
			s.RoutesByAlias[route.Alias] = route
			if route.ClaudeDiscoveryAlias != "" {
				s.RoutesByDiscoveryAlias[route.ClaudeDiscoveryAlias] = route
			}
		}
	}
	return s, nil
}
```

- [ ] **Step 7: Implement redaction**

Create `internal/security/redact.go`:

```go
package security

import "strings"

func Redact(value string) string {
	if value == "" {
		return ""
	}
	return "[redacted]"
}

func RedactMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		if LooksSecret(key) {
			out[key] = Redact(value)
			continue
		}
		out[key] = value
	}
	return out
}

func LooksSecret(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey") ||
		strings.Contains(lower, "key")
}
```

- [ ] **Step 8: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/config internal/security go.mod go.sum
git commit -m "feat: add config validation and snapshots"
```

---

### Task 3: Init, Validate, And Claude Activation Commands

**Files:**
- Create: `internal/security/keys.go`
- Create: `internal/security/keys_test.go`
- Create: `internal/app/paths.go`
- Create: `internal/app/init.go`
- Create: `internal/app/activate.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Write tests for key generation and shell quoting**

Create `internal/security/keys_test.go`:

```go
package security

import "testing"

func TestGenerateClientKey(t *testing.T) {
	key, err := GenerateClientKey()
	if err != nil {
		t.Fatalf("GenerateClientKey() error = %v", err)
	}
	if len(key) < 32 {
		t.Fatalf("key length = %d, want at least 32", len(key))
	}
}

func TestShellQuote(t *testing.T) {
	got := ShellQuote("a'b")
	if got != "'a'\"'\"'b'" {
		t.Fatalf("ShellQuote() = %q", got)
	}
}
```

- [ ] **Step 2: Write CLI command tests**

Append to `internal/cli/cli_test.go`:

```go
func TestRunActivateClaudePrintsExports(t *testing.T) {
	t.Setenv("ARKROUTER_CONFIG", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "activate", "claude", "--host", "127.0.0.1", "--port", "20128", "--client-key", "local-key"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activate output missing %s: %q", want, out)
		}
	}
}

func TestRunValidateMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "validate", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "validate failed") {
		t.Fatalf("stderr = %q, want validate failed", stderr.String())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/cli ./internal/security`

Expected: fails because key generation and commands are not wired.

- [ ] **Step 4: Implement local key helpers**

Create `internal/security/keys.go`:

```go
package security

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
)

func GenerateClientKey() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ark_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
```

- [ ] **Step 5: Implement app paths and init**

Create `internal/app/paths.go`:

```go
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
		return ".arkrouter/config.yaml"
	}
	return filepath.Join(home, ".arkrouter", "config.yaml")
}

func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkrouter/traces.jsonl"
	}
	return filepath.Join(home, ".arkrouter", "traces.jsonl")
}
```

Create `internal/app/init.go`:

```go
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
```

- [ ] **Step 6: Implement Claude activation**

Create `internal/app/activate.go`:

```go
package app

import (
	"fmt"
	"io"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/security"
)

func PrintClaudeActivation(w io.Writer, cfg config.Config) {
	baseURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Fprintf(w, "export ANTHROPIC_BASE_URL=%s\n", security.ShellQuote(baseURL))
	fmt.Fprintf(w, "export ANTHROPIC_AUTH_TOKEN=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=%s\n", security.ShellQuote("1"))
}
```

- [ ] **Step 7: Wire `init`, `validate`, and `activate claude` in CLI**

Modify `internal/cli/cli.go` so `Run` dispatches:

```go
case "init":
	path, err := app.InitConfig(flagValue(args[2:], "--config"), hasFlag(args[2:], "--force"))
	if err != nil {
		fmt.Fprintf(stderr, "init failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "created %s\n", path)
	return 0
case "validate":
	cfg, err := config.LoadFile(flagValue(args[2:], "--config"))
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		fmt.Fprintf(stderr, "validate failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "config ok")
	return 0
case "activate":
	if len(args) >= 3 && args[2] == "claude" {
		cfg := config.MinimalValidConfig("local-key")
		if key := flagValue(args[3:], "--client-key"); key != "" {
			cfg.Server.ClientKey = key
		}
		app.PrintClaudeActivation(stdout, cfg)
		return 0
	}
	fmt.Fprintln(stderr, "usage: arkrouter activate claude")
	return 2
```

Add helper functions in `internal/cli/cli.go`:

```go
func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func flagValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}
```

Add imports for `internal/app` and `internal/config`.

- [ ] **Step 8: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/app internal/cli internal/security
git commit -m "feat: add init validate and claude activation"
```

---

### Task 4: Normalized Protocol And Anthropic Codec

**Files:**
- Create: `internal/protocol/normalized.go`
- Create: `internal/protocol/anthropic/types.go`
- Create: `internal/protocol/anthropic/codec.go`
- Create: `internal/protocol/anthropic/models.go`
- Create: `internal/protocol/anthropic/codec_test.go`

- [ ] **Step 1: Write Anthropic codec tests**

Create `internal/protocol/anthropic/codec_test.go`:

```go
package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeMessageRequestTextAndTools(t *testing.T) {
	body := []byte(`{
	  "model":"sonnet",
	  "max_tokens":1024,
	  "stream":true,
	  "system":"You are concise.",
	  "tools":[{"name":"read_file","description":"Read file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}],
	  "messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]
	}`)
	req, err := DecodeMessageRequest(body)
	if err != nil {
		t.Fatalf("DecodeMessageRequest() error = %v", err)
	}
	if req.Model != "sonnet" || !req.Stream || req.MaxTokens != 1024 {
		t.Fatalf("decoded request = %+v", req)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "read_file" {
		t.Fatalf("tools = %+v", req.Tools)
	}
}

func TestEncodeErrorShape(t *testing.T) {
	body, err := EncodeError("api_error", "upstream failed")
	if err != nil {
		t.Fatalf("EncodeError() error = %v", err)
	}
	if !strings.Contains(string(body), `"type":"error"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestModelsResponse(t *testing.T) {
	resp := ModelsResponseFor([]Model{{ID: "claude-sonnet-4-20250514", DisplayName: "Sonnet"}})
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), "claude-sonnet-4-20250514") {
		t.Fatalf("models response = %s", data)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/protocol/...`

Expected: fails because protocol packages do not exist.

- [ ] **Step 3: Implement normalized protocol types**

Create `internal/protocol/normalized.go`:

```go
package protocol

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type Request struct {
	Model       string
	System      []ContentBlock
	Messages    []Message
	Tools       []Tool
	ToolChoice  string
	MaxTokens   int
	Temperature *float64
	Stream      bool
	Metadata    map[string]string
}

type Message struct {
	Role    Role
	Content []ContentBlock
}

type ContentBlock struct {
	Type      string
	Text      string
	ID        string
	Name      string
	Input     json.RawMessage
	ToolUseID string
	Content   json.RawMessage
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type Response struct {
	ID         string
	Model      string
	Role       Role
	Content    []ContentBlock
	StopReason string
	Usage      Usage
	Metadata   map[string]string
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type StreamEvent struct {
	Type  string
	Index int
	Delta string
	Block ContentBlock
	Usage Usage
	Error string
}

type Capabilities struct {
	Streaming       bool
	Tools           bool
	ToolResults     bool
	Vision          bool
	SystemMessages  bool
	PromptCache     bool
	ContextWindow   int
	MaxOutputTokens int
}
```

- [ ] **Step 4: Implement Anthropic wire structs and codec**

Create `internal/protocol/anthropic/types.go`:

```go
package anthropic

import "encoding/json"

type MessageRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []Message       `json:"messages"`
	System      json.RawMessage `json:"system,omitempty"`
	Tools       []Tool          `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}
```

Create `internal/protocol/anthropic/codec.go`:

```go
package anthropic

import "encoding/json"

func DecodeMessageRequest(body []byte) (MessageRequest, error) {
	var req MessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return MessageRequest{}, err
	}
	return req, nil
}

func EncodeError(errorType string, message string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errorType,
			"message": message,
		},
	})
}
```

Create `internal/protocol/anthropic/models.go`:

```go
package anthropic

type Model struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	DisplayName   string `json:"display_name"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type ModelsResponse struct {
	Data    []Model `json:"data"`
	HasMore bool   `json:"has_more"`
	FirstID string `json:"first_id,omitempty"`
	LastID  string `json:"last_id,omitempty"`
}

func ModelsResponseFor(models []Model) ModelsResponse {
	firstID := ""
	lastID := ""
	if len(models) > 0 {
		firstID = models[0].ID
		lastID = models[len(models)-1].ID
	}
	for i := range models {
		models[i].Type = "model"
	}
	return ModelsResponse{Data: models, HasMore: false, FirstID: firstID, LastID: lastID}
}
```

- [ ] **Step 5: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/protocol
git commit -m "feat: add normalized protocol and anthropic codec"
```

---

### Task 5: Router, Health, And Claude `/healthz` + `/v1/models`

**Files:**
- Create: `internal/router/router.go`
- Create: `internal/router/health.go`
- Create: `internal/router/errors.go`
- Create: `internal/router/router_test.go`
- Create: `internal/client/claude/server.go`
- Create: `internal/client/claude/auth.go`
- Create: `internal/client/claude/models.go`
- Create: `internal/client/claude/server_test.go`

- [ ] **Step 1: Write router tests**

Create `internal/router/router_test.go`:

```go
package router

import (
	"testing"

	"bat.dev/arkrouter/internal/config"
)

func TestResolveRouteByAliasAndDiscoveryAlias(t *testing.T) {
	snapshot, err := config.BuildSnapshot(config.MinimalValidConfig("local-key"))
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	r := New(snapshot, NewHealthStore())
	for _, alias := range []string{"sonnet", "claude-sonnet-4-20250514"} {
		targets, err := r.Resolve(alias, Requirements{Streaming: true, Tools: true})
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", alias, err)
		}
		if len(targets) != 1 || targets[0].Model.ID != "openrouter-sonnet" {
			t.Fatalf("targets = %+v", targets)
		}
	}
}

func TestResolveRejectsMissingCapability(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	cfg.Models[0].Capabilities.Tools = false
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	_, err = New(snapshot, NewHealthStore()).Resolve("sonnet", Requirements{Tools: true})
	if err == nil {
		t.Fatal("Resolve() error = nil, want unsupported capability")
	}
}

func TestRetryableStatus(t *testing.T) {
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		if !IsRetryableStatus(status) {
			t.Fatalf("%d should be retryable", status)
		}
	}
	for _, status := range []int{400, 401, 403, 404} {
		if IsRetryableStatus(status) {
			t.Fatalf("%d should not be retryable", status)
		}
	}
}
```

- [ ] **Step 2: Write Claude models endpoint tests**

Create `internal/client/claude/server_test.go`:

```go
package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/router"
)

func TestModelsRequiresAuth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestModelsReturnsRouteAliases(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"sonnet", "claude-sonnet-4-20250514"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("models response missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestHealthz(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health body = %s", rec.Body.String())
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.MinimalValidConfig("local-key")
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	return NewServer(Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, router.NewHealthStore()),
	})
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/router ./internal/client/claude`

Expected: fails because router and Claude server are not implemented.

- [ ] **Step 4: Implement router contracts**

Create `internal/router/router.go`:

```go
package router

import (
	"fmt"

	"bat.dev/arkrouter/internal/config"
)

type Requirements struct {
	Streaming bool
	Tools     bool
	Vision    bool
}

type Target struct {
	Model    config.ModelConfig
	Provider config.ProviderConfig
	Route    config.RouteConfig
}

type Router struct {
	snapshot config.Snapshot
	health   *HealthStore
}

func New(snapshot config.Snapshot, health *HealthStore) *Router {
	return &Router{snapshot: snapshot, health: health}
}

func (r *Router) Resolve(alias string, req Requirements) ([]Target, error) {
	if route, ok := r.snapshot.RoutesByAlias[alias]; ok {
		return r.resolveRoute(route, req)
	}
	if route, ok := r.snapshot.RoutesByDiscoveryAlias[alias]; ok {
		return r.resolveRoute(route, req)
	}
	if model, ok := r.snapshot.ModelsByExposedAlias[alias]; ok {
		provider := r.snapshot.ProvidersByID[model.ProviderID]
		if !supports(model.Capabilities, req) {
			return nil, fmt.Errorf("model %s does not support requested capabilities", model.ID)
		}
		return []Target{{Model: model, Provider: provider}}, nil
	}
	return nil, fmt.Errorf("model or route %q not found", alias)
}

func (r *Router) resolveRoute(route config.RouteConfig, req Requirements) ([]Target, error) {
	targets := []Target{}
	for _, routeTarget := range route.Targets {
		if !routeTarget.Enabled {
			continue
		}
		model, ok := r.snapshot.ModelsByID[routeTarget.ModelID]
		if !ok || !model.Enabled || !supports(model.Capabilities, req) {
			continue
		}
		provider, ok := r.snapshot.ProvidersByID[model.ProviderID]
		if !ok || !provider.Enabled {
			continue
		}
		targets = append(targets, Target{Model: model, Provider: provider, Route: route})
		if route.Strategy == "priority" {
			break
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("route %s has no target matching requested capabilities", route.Alias)
	}
	return targets, nil
}

func supports(cap config.Capabilities, req Requirements) bool {
	if req.Streaming && !cap.Streaming {
		return false
	}
	if req.Tools && !cap.Tools {
		return false
	}
	if req.Vision && !cap.Vision {
		return false
	}
	return true
}
```

Create `internal/router/errors.go`:

```go
package router

func IsRetryableStatus(status int) bool {
	switch status {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
```

Create `internal/router/health.go`:

```go
package router

import "sync"

type Health struct {
	Status string `json:"status"`
}

type HealthStore struct {
	mu        sync.RWMutex
	upstreams map[string]Health
}

func NewHealthStore() *HealthStore {
	return &HealthStore{upstreams: map[string]Health{}}
}

func (s *HealthStore) Set(id string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upstreams[id] = Health{Status: status}
}

func (s *HealthStore) Snapshot() map[string]Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Health, len(s.upstreams))
	for id, health := range s.upstreams {
		out[id] = health
	}
	return out
}
```

- [ ] **Step 5: Implement Claude server for health and models**

Create `internal/client/claude/server.go`:

```go
package claude

import (
	"encoding/json"
	"net/http"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/router"
)

type Deps struct {
	Snapshot config.Snapshot
	Router   *router.Router
	Health   *router.HealthStore
}

type Server struct {
	deps Deps
}

func NewServer(deps Deps) *Server {
	if deps.Health == nil {
		deps.Health = router.NewHealthStore()
	}
	return &Server{deps: deps}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
```

Create `internal/client/claude/auth.go`:

```go
package claude

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.deps.Snapshot.Config.Server.ClientKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"type": "error",
				"error": map[string]string{
					"type":    "authentication_error",
					"message": "invalid local client key",
				},
			})
			return
		}
		next(w, r)
	}
}
```

Create `internal/client/claude/models.go`:

```go
package claude

import (
	"net/http"
	"sort"

	aproto "bat.dev/arkrouter/internal/protocol/anthropic"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"loaded_at": s.deps.Snapshot.LoadedAt,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	entries := map[string]aproto.Model{}
	for _, route := range s.deps.Snapshot.RoutesByAlias {
		display := route.Alias
		context := 0
		if len(route.Targets) > 0 {
			if model, ok := s.deps.Snapshot.ModelsByID[route.Targets[0].ModelID]; ok {
				display = model.DisplayName
				context = model.Capabilities.ContextWindow
			}
		}
		entries[route.Alias] = aproto.Model{ID: route.Alias, DisplayName: display, ContextWindow: context}
		if route.ClaudeDiscoveryAlias != "" {
			entries[route.ClaudeDiscoveryAlias] = aproto.Model{ID: route.ClaudeDiscoveryAlias, DisplayName: display, ContextWindow: context}
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	models := make([]aproto.Model, 0, len(keys))
	for _, key := range keys {
		models = append(models, entries[key])
	}
	writeJSON(w, http.StatusOK, aproto.ModelsResponseFor(models))
}
```

- [ ] **Step 6: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/router internal/client/claude
git commit -m "feat: add router and claude model discovery"
```

---

### Task 6: Adapter Interface And OpenAI-Compatible Non-Streaming

**Files:**
- Create: `internal/adapter/adapter.go`
- Create: `internal/adapter/openai/url.go`
- Create: `internal/adapter/openai/mapper.go`
- Create: `internal/adapter/openai/openai_test.go`
- Create: `internal/protocol/openai/types.go`

- [ ] **Step 1: Write OpenAI adapter tests**

Create `internal/adapter/openai/openai_test.go`:

```go
package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

func TestChatCompletionsURL(t *testing.T) {
	for _, baseURL := range []string{
		"https://openrouter.ai/api/v1",
		"https://example.test",
		"https://example.test/v1/",
	} {
		got, err := ChatCompletionsURL(baseURL)
		if err != nil {
			t.Fatalf("ChatCompletionsURL(%q) error = %v", baseURL, err)
		}
		if strings.Contains(got, "/v1/v1/") {
			t.Fatalf("url duplicated v1: %s", got)
		}
		if !strings.HasSuffix(got, "/chat/completions") {
			t.Fatalf("url = %s, want chat completions suffix", got)
		}
	}
}

func TestBuildRequestMapsTextAndTools(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:     "sonnet",
		MaxTokens: 512,
		Tools: []protocol.Tool{{
			Name:        "read_file",
			Description: "Read file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Messages: []protocol.Message{{
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "text", Text: "hello"}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "anthropic/claude-sonnet-4.5"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if out.Method != "POST" || !strings.HasSuffix(out.URL, "/chat/completions") {
		t.Fatalf("upstream request = %+v", out)
	}
	if out.Headers.Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", out.Headers.Get("Authorization"))
	}
	if !strings.Contains(string(out.Body), `"model":"anthropic/claude-sonnet-4.5"`) {
		t.Fatalf("body = %s", out.Body)
	}
	if !strings.Contains(string(out.Body), `"tools"`) {
		t.Fatalf("body missing tools = %s", out.Body)
	}
}

func TestMapResponse(t *testing.T) {
	adapter := Adapter{}
	resp, err := adapter.MapResponse([]byte(`{"id":"chatcmpl_1","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	if err != nil {
		t.Fatalf("MapResponse() error = %v", err)
	}
	if resp.Content[0].Text != "hello" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("response = %+v", resp)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/openai`

Expected: fails because adapter packages do not exist.

- [ ] **Step 3: Define adapter interface**

Create `internal/adapter/adapter.go`:

```go
package adapter

import (
	"net/http"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

type UpstreamRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

type ProviderAdapter interface {
	BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (UpstreamRequest, error)
	MapResponse(body []byte) (protocol.Response, error)
}
```

- [ ] **Step 4: Define OpenAI wire structs**

Create `internal/protocol/openai/types.go`:

```go
package openai

import "encoding/json"

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []Message     `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	ToolChoice  any           `json:"tool_choice,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
```

- [ ] **Step 5: Implement OpenAI-compatible adapter**

Create `internal/adapter/openai/url.go` and `internal/adapter/openai/mapper.go`:

```go
package openai

import (
	"net/url"
	"strings"
)

func ChatCompletionsURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	if strings.HasSuffix(path, "/chat/completions") {
		parsed.Path = path
		return parsed.String(), nil
	}
	parsed.Path = path + "/chat/completions"
	return parsed.String(), nil
}
```

```go
package openai

import (
	"encoding/json"
	"net/http"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
	oai "bat.dev/arkrouter/internal/protocol/openai"
)

type Adapter struct{}

func (a Adapter) BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (adapter.UpstreamRequest, error) {
	url, err := ChatCompletionsURL(provider.BaseURL)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	body := oai.ChatRequest{
		Model:       model.UpstreamModel,
		Messages:    mapMessages(req.Messages),
		Tools:       mapTools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+provider.APIKey)
	headers.Set("Content-Type", "application/json")
	for key, value := range provider.Headers {
		headers.Set(key, value)
	}
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: url, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded oai.ChatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return protocol.Response{}, err
	}
	resp := protocol.Response{ID: decoded.ID, Role: protocol.RoleAssistant}
	if len(decoded.Choices) > 0 {
		choice := decoded.Choices[0]
		resp.StopReason = choice.FinishReason
		if choice.Message.Content != "" {
			resp.Content = append(resp.Content, protocol.ContentBlock{Type: "text", Text: choice.Message.Content})
		}
		for _, call := range choice.Message.ToolCalls {
			resp.Content = append(resp.Content, protocol.ContentBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: json.RawMessage(call.Function.Arguments)})
		}
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens}
	return resp, nil
}

func mapMessages(messages []protocol.Message) []oai.Message {
	out := make([]oai.Message, 0, len(messages))
	for _, msg := range messages {
		text := ""
		for _, block := range msg.Content {
			if block.Type == "text" {
				text += block.Text
			}
		}
		out = append(out, oai.Message{Role: string(msg.Role), Content: text})
	}
	return out
}

func mapTools(tools []protocol.Tool) []oai.Tool {
	out := make([]oai.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, oai.Tool{Type: "function", Function: oai.FunctionDef{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema}})
	}
	return out
}
```

- [ ] **Step 6: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/adapter internal/protocol/openai
git commit -m "feat: add openai compatible non-streaming adapter"
```

---

### Task 7: Claude `/v1/messages` Non-Streaming Integration

**Files:**
- Modify: `internal/client/claude/server.go`
- Create: `internal/client/claude/messages.go`
- Create: `internal/testutil/upstream.go`
- Modify: `internal/client/claude/server_test.go`
- Create: `internal/app/serve.go`
- Modify: `internal/cli/cli.go`

- [ ] **Step 1: Write integration test for non-streaming messages**

Append to `internal/client/claude/server_test.go`:

```go
func TestMessagesNonStreamingOpenAICompatible(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("upstream path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("upstream auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	srv := NewServer(Deps{Snapshot: snapshot, Router: router.New(snapshot, router.NewHealthStore())})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"text":"pong"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCountTokensReturnsLocalEstimate(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"sonnet","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"input_tokens"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/claude -run TestMessagesNonStreamingOpenAICompatible -v`

Expected: fails because `/v1/messages` is not registered.

- [ ] **Step 3: Implement non-streaming message handler**

Modify `internal/client/claude/server.go` to register `/v1/messages`:

```go
mux.HandleFunc("/v1/messages", s.withAuth(s.handleMessages))
mux.HandleFunc("/v1/messages/count_tokens", s.withAuth(s.handleCountTokens))
```

Create `internal/client/claude/messages.go`:

```go
package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	openaiadapter "bat.dev/arkrouter/internal/adapter/openai"
	"bat.dev/arkrouter/internal/protocol"
	aproto "bat.dev/arkrouter/internal/protocol/anthropic"
	"bat.dev/arkrouter/internal/router"
)

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "read request failed"))
		return
	}
	anthropicReq, err := aproto.DecodeMessageRequest(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "invalid Anthropic request"))
		return
	}
	targets, err := s.deps.Router.Resolve(anthropicReq.Model, router.Requirements{Streaming: anthropicReq.Stream, Tools: len(anthropicReq.Tools) > 0})
	if err != nil {
		writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
		return
	}
	target := targets[0]
	normalized := protocol.Request{
		Model:     anthropicReq.Model,
		MaxTokens: anthropicReq.MaxTokens,
		Stream:    anthropicReq.Stream,
		Tools:     mapAnthropicTools(anthropicReq.Tools),
		Messages:  mapAnthropicMessages(anthropicReq.Messages),
	}
	adapter := openaiadapter.Adapter{}
	upstreamReq, err := adapter.BuildRequest(normalized, target.Provider, target.Model)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
		return
	}
	client := &http.Client{Timeout: time.Duration(s.deps.Snapshot.Config.Server.UpstreamTimeoutSeconds) * time.Second}
	httpReq, err := http.NewRequestWithContext(r.Context(), upstreamReq.Method, upstreamReq.URL, bytes.NewReader(upstreamReq.Body))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
		return
	}
	httpReq.Header = upstreamReq.Headers.Clone()
	upstreamResp, err := client.Do(httpReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
		return
	}
	defer upstreamResp.Body.Close()
	upstreamBody, _ := io.ReadAll(upstreamResp.Body)
	if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, anthropicError("api_error", "upstream returned non-success status"))
		return
	}
	mapped, err := adapter.MapResponse(upstreamBody)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, mapNormalizedResponse(mapped, target.Model.ExposedAlias))
}

func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "read request failed"))
		return
	}
	req, err := aproto.DecodeMessageRequest(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError("invalid_request_error", "invalid Anthropic request"))
		return
	}
	if _, err := s.deps.Router.Resolve(req.Model, router.Requirements{Tools: len(req.Tools) > 0}); err != nil {
		writeJSON(w, http.StatusNotFound, anthropicError("not_found_error", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"input_tokens": estimateInputTokens(body)})
}

func estimateInputTokens(body []byte) int {
	return (len(body)*2+4)/5 + 32
}

func anthropicError(errorType string, message string) map[string]any {
	return map[string]any{"type": "error", "error": map[string]string{"type": errorType, "message": message}}
}

func mapAnthropicTools(tools []aproto.Tool) []protocol.Tool {
	out := make([]protocol.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, protocol.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return out
}

func mapAnthropicMessages(messages []aproto.Message) []protocol.Message {
	out := make([]protocol.Message, 0, len(messages))
	for _, msg := range messages {
		var blocks []aproto.ContentBlock
		_ = json.Unmarshal(msg.Content, &blocks)
		content := make([]protocol.ContentBlock, 0, len(blocks))
		for _, block := range blocks {
			content = append(content, protocol.ContentBlock{Type: block.Type, Text: block.Text})
		}
		out = append(out, protocol.Message{Role: protocol.Role(msg.Role), Content: content})
	}
	return out
}

func mapNormalizedResponse(resp protocol.Response, model string) map[string]any {
	content := make([]map[string]any, 0, len(resp.Content))
	for _, block := range resp.Content {
		if block.Type == "text" {
			content = append(content, map[string]any{"type": "text", "text": block.Text})
		}
		if block.Type == "tool_use" {
			content = append(content, map[string]any{"type": "tool_use", "id": block.ID, "name": block.Name, "input": block.Input})
		}
	}
	return map[string]any{
		"id":            resp.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   resp.StopReason,
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		},
	}
}
```

- [ ] **Step 4: Wire `serve` command minimally**

Create `internal/app/serve.go`:

```go
package app

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"bat.dev/arkrouter/internal/client/claude"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/router"
)

func Serve(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		return err
	}
	health := router.NewHealthStore()
	server := claude.NewServer(claude.Deps{Snapshot: snapshot, Router: router.New(snapshot, health), Health: health})
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	fmt.Printf("arkrouter listening on http://%s\n", addr)
	return http.ListenAndServe(addr, server.Routes())
}
```

Wire `serve` in `internal/cli/cli.go`:

```go
case "serve":
	if err := app.Serve(flagValue(args[2:], "--config")); err != nil {
		fmt.Fprintf(stderr, "serve failed: %v\n", err)
		return 1
	}
	return 0
```

- [ ] **Step 5: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/client/claude internal/app internal/cli
git commit -m "feat: handle claude non-streaming messages"
```

---

### Task 8: OpenAI-Compatible Streaming SSE

**Files:**
- Create: `internal/adapter/openai/stream.go`
- Modify: `internal/adapter/openai/openai_test.go`
- Create: `internal/client/claude/stream.go`
- Modify: `internal/client/claude/messages.go`
- Modify: `internal/client/claude/server_test.go`

- [ ] **Step 1: Write streaming adapter tests**

Append to `internal/adapter/openai/openai_test.go`:

```go
func TestStreamMapperTextDeltas(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"hel"},"index":0}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatal("events empty")
	}
	events, err = mapper.MapLine([]byte(`data: {"choices":[{"delta":{"content":"lo"},"index":0,"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == "content_delta" && event.Delta == "lo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events missing content delta: %+v", events)
	}
}

func TestStreamMapperDone(t *testing.T) {
	mapper := NewStreamMapper()
	events, err := mapper.MapLine([]byte("data: [DONE]"))
	if err != nil {
		t.Fatalf("MapLine() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "message_stop" {
		t.Fatalf("events = %+v", events)
	}
}
```

- [ ] **Step 2: Write Claude streaming integration test**

Append to `internal/client/claude/server_test.go`:

```go
func TestMessagesStreamingOpenAICompatible(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"index\":0}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers[0].BaseURL = upstream.URL + "/api/v1"
	cfg.Providers[0].APIKey = "sk-test"
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	srv := NewServer(Deps{Snapshot: snapshot, Router: router.New(snapshot, router.NewHealthStore())})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"event: message_start", "event: content_block_delta", "event: message_stop"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("stream missing %s: %s", want, rec.Body.String())
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/adapter/openai ./internal/client/claude -run Streaming -v`

Expected: fails because stream mapper is not implemented.

- [ ] **Step 4: Implement OpenAI stream mapper**

Create `internal/adapter/openai/stream.go`:

```go
package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"bat.dev/arkrouter/internal/protocol"
)

type StreamMapper struct {
	started bool
}

func NewStreamMapper() *StreamMapper {
	return &StreamMapper{}
}

func (m *StreamMapper) MapLine(line []byte) ([]protocol.StreamEvent, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil
	}
	if !bytes.HasPrefix(line, []byte("data:")) {
		return nil, nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(string(line), "data:"))
	if payload == "[DONE]" {
		return []protocol.StreamEvent{{Type: "message_stop"}}, nil
	}
	var chunk struct {
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, err
	}
	events := []protocol.StreamEvent{}
	if !m.started {
		m.started = true
		events = append(events, protocol.StreamEvent{Type: "message_start"})
		events = append(events, protocol.StreamEvent{Type: "content_block_start", Index: 0, Block: protocol.ContentBlock{Type: "text"}})
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			events = append(events, protocol.StreamEvent{Type: "content_delta", Index: choice.Index, Delta: choice.Delta.Content})
		}
		if choice.FinishReason != "" {
			events = append(events, protocol.StreamEvent{Type: "content_block_stop", Index: choice.Index})
			events = append(events, protocol.StreamEvent{Type: "message_delta"})
		}
	}
	return events, nil
}
```

- [ ] **Step 5: Implement Claude SSE writer and route streaming**

Create `internal/client/claude/stream.go`:

```go
package claude

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bat.dev/arkrouter/internal/protocol"
)

func writeSSE(w http.ResponseWriter, event string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeAnthropicStreamEvent(w http.ResponseWriter, event protocol.StreamEvent, model string) {
	switch event.Type {
	case "message_start":
		writeSSE(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_stream", "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil}})
	case "content_block_start":
		writeSSE(w, "content_block_start", map[string]any{"type": "content_block_start", "index": event.Index, "content_block": map[string]any{"type": "text", "text": ""}})
	case "content_delta":
		writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": event.Index, "delta": map[string]any{"type": "text_delta", "text": event.Delta}})
	case "content_block_stop":
		writeSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": event.Index})
	case "message_delta":
		writeSSE(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 0}})
	case "message_stop":
		writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
	}
}
```

Modify `handleMessages` so when `anthropicReq.Stream` is true it scans upstream SSE lines with `bufio.Scanner`, calls `scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)` to allow large provider chunks, maps each line using `openai.NewStreamMapper()`, and writes events with `writeAnthropicStreamEvent`.

- [ ] **Step 6: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/adapter/openai internal/client/claude
git commit -m "feat: stream openai compatible responses to claude"
```

---

### Task 9: Tool Results, Fallback Routing, Health, And Trace Logs

**Files:**
- Modify: `internal/adapter/openai/mapper.go`
- Modify: `internal/adapter/openai/openai_test.go`
- Modify: `internal/client/claude/messages.go`
- Modify: `internal/router/health.go`
- Create: `internal/observability/trace.go`
- Create: `internal/observability/trace_test.go`
- Modify: `internal/client/claude/server_test.go`

- [ ] **Step 1: Add tool result and fallback tests**

Append to `internal/adapter/openai/openai_test.go`:

```go
func TestBuildRequestMapsToolResults(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model: "sonnet",
		Messages: []protocol.Message{{
			Role: protocol.RoleUser,
			Content: []protocol.ContentBlock{{Type: "tool_result", ToolUseID: "toolu_1", Content: json.RawMessage(`"file contents"`)}},
		}},
	}
	provider := config.ProviderConfig{BaseURL: "https://example.test/v1", APIKey: "sk-test"}
	model := config.ModelConfig{UpstreamModel: "model"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(string(out.Body), `"tool_call_id":"toolu_1"`) {
		t.Fatalf("body missing tool result mapping: %s", out.Body)
	}
}
```

Append to `internal/client/claude/server_test.go`:

```go
func TestMessagesFallbackOnRetryableStatus(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate limited`))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"fallback ok"},"finish_reason":"stop"}]}`))
	}))
	defer second.Close()

	cfg := config.MinimalValidConfig("local-key")
	cfg.Providers = append(cfg.Providers, cfg.Providers[0])
	cfg.Providers[0].ID = "first"
	cfg.Providers[0].BaseURL = first.URL + "/v1"
	cfg.Providers[0].APIKey = "sk-first"
	cfg.Providers[1].ID = "second"
	cfg.Providers[1].BaseURL = second.URL + "/v1"
	cfg.Providers[1].APIKey = "sk-second"
	cfg.Models = append(cfg.Models, cfg.Models[0])
	cfg.Models[0].ID = "first-model"
	cfg.Models[0].ProviderID = "first"
	cfg.Models[1].ID = "second-model"
	cfg.Models[1].ProviderID = "second"
	cfg.Routes[0].Targets = []config.RouteTarget{{ModelID: "first-model", Enabled: true}, {ModelID: "second-model", Enabled: true}}
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	health := router.NewHealthStore()
	srv := NewServer(Deps{Snapshot: snapshot, Router: router.New(snapshot, health), Health: health})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"sonnet","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}]}`))
	req.Header.Set("Authorization", "Bearer local-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fallback ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Add JSONL trace tests**

Create `internal/observability/trace_test.go`:

```go
package observability

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteTraceRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTrace(&buf, TraceEvent{
		Time:      time.Unix(0, 0).UTC(),
		RequestID: "req_1",
		Route:     "sonnet",
		Provider:  "openrouter",
		Status:    200,
		Headers:   map[string]string{"Authorization": "Bearer secret", "X-OpenRouter-Title": "Arkrouter"},
	})
	if err != nil {
		t.Fatalf("WriteTrace() error = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Bearer secret") {
		t.Fatalf("trace leaked secret: %s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("trace missing redaction: %s", out)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/adapter/openai ./internal/client/claude ./internal/observability`

Expected: fails because fallback, tool result mapping, and traces are incomplete.

- [ ] **Step 4: Implement tool result mapping**

Modify `mapMessages` in `internal/adapter/openai/mapper.go`:

```go
if block.Type == "tool_result" {
	out = append(out, oai.Message{Role: "tool", ToolCallID: block.ToolUseID, Content: string(block.Content)})
	continue
}
```

Ensure normal text blocks still append user/assistant messages once per role.

- [ ] **Step 5: Implement fallback loop and health updates**

Modify `handleMessages` so it loops through `targets`. For each non-streaming target:

```go
for i, target := range targets {
	resp, status, err := s.callTarget(r.Context(), normalized, target)
	if err == nil {
		s.deps.Health.Set(target.Provider.ID, "ok")
		writeJSON(w, http.StatusOK, mapNormalizedResponse(resp, target.Model.ExposedAlias))
		return
	}
	if !router.IsRetryableStatus(status) || i == len(targets)-1 {
		s.deps.Health.Set(target.Provider.ID, "unhealthy")
		writeJSON(w, http.StatusBadGateway, anthropicError("api_error", err.Error()))
		return
	}
	s.deps.Health.Set(target.Provider.ID, "degraded")
}
```

Extract upstream request execution into:

```go
func (s *Server) callTarget(ctx context.Context, req protocol.Request, target router.Target) (protocol.Response, int, error)
```

Return upstream status codes so retry classification is deterministic.

For streaming requests, use the same target loop until an upstream returns a `2xx` response. If the upstream returns a retryable status before any SSE event is written, try the next target. After the first SSE event is written to Claude Code, stop fallback attempts for that request and surface stream errors as Anthropic-compatible SSE error events.

- [ ] **Step 6: Implement JSONL trace writer**

Create `internal/observability/trace.go`:

```go
package observability

import (
	"encoding/json"
	"io"
	"time"

	"bat.dev/arkrouter/internal/security"
)

type TraceEvent struct {
	Time      time.Time         `json:"time"`
	RequestID string            `json:"request_id"`
	Route     string            `json:"route"`
	Provider  string            `json:"provider"`
	Model     string            `json:"model,omitempty"`
	Status    int               `json:"status"`
	LatencyMS int64             `json:"latency_ms,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

func WriteTrace(w io.Writer, event TraceEvent) error {
	if event.Headers != nil {
		event.Headers = security.RedactMap(event.Headers)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}
```

- [ ] **Step 7: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/adapter/openai internal/client/claude internal/router internal/observability
git commit -m "feat: add fallback health and trace logging"
```

---

### Task 10: Gemini And Anthropic Provider Adapters

**Files:**
- Create: `internal/adapter/gemini/mapper.go`
- Create: `internal/adapter/gemini/gemini_test.go`
- Create: `internal/adapter/anthropic/passthrough.go`
- Create: `internal/adapter/anthropic/passthrough_test.go`
- Modify: `internal/client/claude/messages.go`

- [ ] **Step 1: Write Gemini adapter tests**

Create `internal/adapter/gemini/gemini_test.go`:

```go
package gemini

import (
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

func TestBuildRequest(t *testing.T) {
	adapter := Adapter{}
	req := protocol.Request{
		Model:     "gemini-pro",
		MaxTokens: 512,
		Messages: []protocol.Message{{Role: protocol.RoleUser, Content: []protocol.ContentBlock{{Type: "text", Text: "hello"}}}},
	}
	provider := config.ProviderConfig{BaseURL: "https://generativelanguage.googleapis.com/v1beta", APIKey: "AIza-test"}
	model := config.ModelConfig{UpstreamModel: "gemini-2.5-pro"}
	out, err := adapter.BuildRequest(req, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.Contains(out.URL, "/models/gemini-2.5-pro:generateContent") {
		t.Fatalf("url = %s", out.URL)
	}
	if !strings.Contains(out.URL, "key=AIza-test") {
		t.Fatalf("url missing key: %s", out.URL)
	}
	if !strings.Contains(string(out.Body), `"text":"hello"`) {
		t.Fatalf("body = %s", out.Body)
	}
}
```

- [ ] **Step 2: Write Anthropic passthrough tests**

Create `internal/adapter/anthropic/passthrough_test.go`:

```go
package anthropic

import (
	"strings"
	"testing"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

func TestBuildRequest(t *testing.T) {
	adapter := Adapter{}
	provider := config.ProviderConfig{BaseURL: "https://api.anthropic.com", APIKey: "sk-ant-test"}
	model := config.ModelConfig{UpstreamModel: "claude-sonnet-4-20250514"}
	out, err := adapter.BuildRequest(protocol.Request{Model: "sonnet", MaxTokens: 128, Messages: []protocol.Message{{Role: protocol.RoleUser, Content: []protocol.ContentBlock{{Type: "text", Text: "hi"}}}}}, provider, model)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if !strings.HasSuffix(out.URL, "/v1/messages") {
		t.Fatalf("url = %s", out.URL)
	}
	if out.Headers.Get("x-api-key") != "sk-ant-test" {
		t.Fatalf("x-api-key = %q", out.Headers.Get("x-api-key"))
	}
	if !strings.Contains(string(out.Body), `"model":"claude-sonnet-4-20250514"`) {
		t.Fatalf("body = %s", out.Body)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/adapter/gemini ./internal/adapter/anthropic`

Expected: fails because provider adapters are not implemented.

- [ ] **Step 4: Implement Gemini adapter**

Create `internal/adapter/gemini/mapper.go`:

```go
package gemini

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

type Adapter struct{}

func (a Adapter) BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (adapter.UpstreamRequest, error) {
	endpoint, err := geminiURL(provider.BaseURL, model.UpstreamModel, req.Stream, provider.APIKey)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	body := map[string]any{
		"contents":         mapMessages(req.Messages),
		"generationConfig": map[string]any{"maxOutputTokens": req.MaxTokens},
	}
	if len(req.Tools) > 0 {
		body["tools"] = []any{map[string]any{"functionDeclarations": mapTools(req.Tools)}}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: endpoint, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		Usage struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return protocol.Response{}, err
	}
	resp := protocol.Response{Role: protocol.RoleAssistant}
	if len(decoded.Candidates) > 0 {
		candidate := decoded.Candidates[0]
		resp.StopReason = strings.ToLower(candidate.FinishReason)
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				resp.Content = append(resp.Content, protocol.ContentBlock{Type: "text", Text: part.Text})
			}
		}
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.PromptTokenCount, OutputTokens: decoded.Usage.CandidatesTokenCount}
	return resp, nil
}

func geminiURL(baseURL string, model string, stream bool, apiKey string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	method := "generateContent"
	if stream {
		method = "streamGenerateContent"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/models/" + url.PathEscape(model) + ":" + method
	query := parsed.Query()
	query.Set("key", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func mapMessages(messages []protocol.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := "user"
		if msg.Role == protocol.RoleAssistant {
			role = "model"
		}
		parts := []map[string]string{}
		for _, block := range msg.Content {
			if block.Type == "text" && block.Text != "" {
				parts = append(parts, map[string]string{"text": block.Text})
			}
		}
		out = append(out, map[string]any{"role": role, "parts": parts})
	}
	return out
}

func mapTools(tools []protocol.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		var parameters any = map[string]any{"type": "object"}
		if len(tool.InputSchema) > 0 {
			_ = json.Unmarshal(tool.InputSchema, &parameters)
		}
		out = append(out, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  parameters,
		})
	}
	return out
}
```

- [ ] **Step 5: Implement Anthropic passthrough adapter**

Create `internal/adapter/anthropic/passthrough.go`:

```go
package anthropic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

type Adapter struct{}

func (a Adapter) BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (adapter.UpstreamRequest, error) {
	endpoint, err := messagesURL(provider.BaseURL)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	body := map[string]any{
		"model":      model.UpstreamModel,
		"max_tokens": req.MaxTokens,
		"messages":   mapMessages(req.Messages),
		"stream":     req.Stream,
	}
	if len(req.Tools) > 0 {
		body["tools"] = mapTools(req.Tools)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return adapter.UpstreamRequest{}, err
	}
	headers := http.Header{}
	headers.Set("x-api-key", provider.APIKey)
	headers.Set("anthropic-version", "2023-06-01")
	headers.Set("Content-Type", "application/json")
	return adapter.UpstreamRequest{Method: http.MethodPost, URL: endpoint, Headers: headers, Body: data}, nil
}

func (a Adapter) MapResponse(body []byte) (protocol.Response, error) {
	var decoded struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		Role       string `json:"role"`
		Content    []struct {
			Type string          `json:"type"`
			Text string          `json:"text"`
			ID   string          `json:"id"`
			Name string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return protocol.Response{}, err
	}
	resp := protocol.Response{ID: decoded.ID, Model: decoded.Model, Role: protocol.RoleAssistant, StopReason: decoded.StopReason}
	for _, block := range decoded.Content {
		resp.Content = append(resp.Content, protocol.ContentBlock{Type: block.Type, Text: block.Text, ID: block.ID, Name: block.Name, Input: block.Input})
	}
	resp.Usage = protocol.Usage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens}
	return resp, nil
}

func messagesURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/v1/messages") {
		path += "/v1/messages"
	}
	parsed.Path = path
	return parsed.String(), nil
}

func mapMessages(messages []protocol.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		content := make([]map[string]any, 0, len(msg.Content))
		for _, block := range msg.Content {
			if block.Type == "text" {
				content = append(content, map[string]any{"type": "text", "text": block.Text})
			}
		}
		out = append(out, map[string]any{"role": string(msg.Role), "content": content})
	}
	return out
}

func mapTools(tools []protocol.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
	}
	return out
}
```

- [ ] **Step 6: Route provider type to adapter**

Modify `internal/client/claude/messages.go` to choose adapter by `target.Provider.Type`:

```go
switch target.Provider.Type {
case "openai_compatible":
	adapter = openaiadapter.Adapter{}
case "gemini":
	adapter = geminiadapter.Adapter{}
case "anthropic":
	adapter = anthropicadapter.Adapter{}
default:
	return protocol.Response{}, 0, fmt.Errorf("unsupported provider type %s", target.Provider.Type)
}
```

- [ ] **Step 7: Verify and commit**

Run:

```sh
gofmt -w .
go test ./...
```

Expected: all tests pass.

Commit:

```sh
git add internal/adapter/gemini internal/adapter/anthropic internal/client/claude
git commit -m "feat: add gemini and anthropic adapters"
```

---

### Task 11: Status, Doctor, Logs, Test Command, And Documentation

**Files:**
- Create: `internal/app/commands.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `README.md`
- Create: `.gitignore`

- [ ] **Step 1: Write command tests**

Append to `internal/cli/cli_test.go`:

```go
func TestRunDoctor(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "doctor", "--config", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "doctor failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunLogsMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "logs", "--file", "/path/does/not/exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "logs failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunTestMissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"arkrouter", "test"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: arkrouter test <model> <prompt>") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli`

Expected: fails because commands are not wired.

- [ ] **Step 3: Implement app command helpers**

Create `internal/app/commands.go`:

```go
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"bat.dev/arkrouter/internal/config"
)

func ValidateConfig(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	fmt.Fprintln(w, "config ok")
	return nil
}

func Doctor(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	fmt.Fprintf(w, "config: ok\nproviders: %d\nmodels: %d\nroutes: %d\n", len(cfg.Providers), len(cfg.Models), len(cfg.Routes))
	return nil
}

func PrintLogs(path string, w io.Writer) error {
	if path == "" {
		path = DefaultLogPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func TestRoute(path string, model string, prompt string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 128,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]string{{
				"type": "text",
				"text": prompt,
			}},
		}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:%d/v1/messages", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(respBody))
	}
	_, err = w.Write(respBody)
	if err == nil {
		_, err = fmt.Fprintln(w)
	}
	return err
}

func pathOrDefault(path string) string {
	if path != "" {
		return path
	}
	return DefaultConfigPath()
}
```

- [ ] **Step 4: Wire status, doctor, logs, and test command**

Modify `internal/cli/cli.go`:

```go
case "doctor":
	if err := app.Doctor(flagValue(args[2:], "--config"), stdout); err != nil {
		fmt.Fprintf(stderr, "doctor failed: %v\n", err)
		return 1
	}
	return 0
case "logs":
	if err := app.PrintLogs(flagValue(args[2:], "--file"), stdout); err != nil {
		fmt.Fprintf(stderr, "logs failed: %v\n", err)
		return 1
	}
	return 0
case "status":
	if err := app.Doctor(flagValue(args[2:], "--config"), stdout); err != nil {
		fmt.Fprintf(stderr, "status failed: %v\n", err)
		return 1
	}
	return 0
case "test":
	if len(args) < 4 {
		fmt.Fprintln(stderr, "usage: arkrouter test <model> <prompt>")
		return 2
	}
	if err := app.TestRoute(flagValue(args[4:], "--config"), args[2], args[3], stdout); err != nil {
		fmt.Fprintf(stderr, "test failed: %v\n", err)
		return 1
	}
	return 0
```

- [ ] **Step 5: Add `.gitignore`**

Create `.gitignore`:

```gitignore
.DS_Store
arkrouter
*.test
coverage.out
dist/
```

- [ ] **Step 6: Update README**

Expand `README.md` with:

```markdown
## Claude Code Usage

```sh
arkrouter init
arkrouter validate
arkrouter serve
```

In another shell:

```sh
eval "$(arkrouter activate claude)"
claude
```

## Config

Default config path:

```text
~/.arkrouter/config.yaml
```

Generated provider keys use `env:NAME` references. Export provider keys in your shell before starting `arkrouter serve`.

## Safety

Arkrouter binds to `127.0.0.1` by default and does not log prompt or response bodies.
```

- [ ] **Step 7: Verify full acceptance**

Run:

```sh
gofmt -w .
go test ./...
go run ./cmd/arkrouter version
go run ./cmd/arkrouter help
```

Expected:

```text
arkrouter dev
Usage: arkrouter <command> [flags]
```

Commit:

```sh
git add .gitignore README.md internal/app internal/cli
git commit -m "feat: add operator commands and docs"
```

---

## Final Verification

Run these commands from `/Users/bat/RiderProjects/arkrouter`:

```sh
gofmt -w .
go test ./...
go run ./cmd/arkrouter version
go run ./cmd/arkrouter help
```

Expected:

```text
arkrouter dev
Usage: arkrouter <command> [flags]
```

Review git history:

```sh
git log --oneline --decorate -n 12
git status --short
```

Expected:

```text
git status --short
```

prints no tracked or untracked files.

## Known Phase 1 Boundaries

- OpenAI-compatible ingress for OpenCode/Cursor/Cline is not included.
- Dashboard is not included.
- Token compression is not included.
- `arkrouter test` may remain credential-dependent unless a route and provider credentials are configured.
- Gemini streaming/tool mapping is phase 1 basic and should be covered by fixtures before real-provider tuning.

## Plan Self-Review

Spec coverage:

- Claude Code endpoints are covered by Tasks 5, 7, and 8.
- Config, validation, client key, and activation are covered by Tasks 2 and 3.
- Normalized protocol and adapter boundaries are covered by Tasks 4, 6, 8, and 10.
- Fallback, health, retry classification, and logs are covered by Task 9.
- Status, doctor, logs, README, and final verification are covered by Task 11.

Placeholder scan:

- The plan contains no placeholder task bodies and no blank task bodies.
- Each task lists exact files and concrete commands.

Type consistency:

- Config uses `exposed_alias` and `capabilities` consistently.
- Router uses `Requirements`, `Target`, and `Resolve`.
- Provider adapters use `BuildRequest` and `MapResponse`.
- Claude server dependencies use `Deps`, `Snapshot`, `Router`, and `Health`.
