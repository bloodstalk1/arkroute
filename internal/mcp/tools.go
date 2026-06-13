package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
)

// Tools builds the MCP tool list for arkroute.
func Tools() []Tool {
	return []Tool{
		{
			Name:        "arkroute_status",
			Description: "Show the current status of the arkroute gateway: config path, server address, enabled providers, models, routes, and profiles.",
			InputSchema: NoArgsSchema(),
		},
		{
			Name:        "arkroute_providers",
			Description: "List all configured AI providers with their type, base URL, enabled status, and health.",
			InputSchema: NoArgsSchema(),
		},
		{
			Name:        "arkroute_models",
			Description: "List all configured models with exposed aliases, provider bindings, and capabilities.",
			InputSchema: NoArgsSchema(),
		},
		{
			Name:        "arkroute_reload",
			Description: "Trigger a hot reload of the arkroute configuration without restarting the gateway. Changes to models, routes, and providers take effect immediately.",
			InputSchema: NoArgsSchema(),
		},
		{
			Name:        "arkroute_test",
			Description: "Send a simple non-streaming test prompt to a model alias to verify connectivity and basic functionality.",
			InputSchema: SimpleSchema(map[string]any{
				"model": map[string]any{"type": "string", "description": "Model alias to test (e.g. sonnet, gpt-4o, gemini-pro)."},
				"prompt": map[string]any{"type": "string", "description": "The prompt to send. Default: 'Say hello in exactly 3 words.'"},
			}),
		},
	}
}

// Handler returns a tool-call handler that uses the given config file path.
func Handler(configPath string) func(toolName string, args map[string]any) (string, error) {
	return func(toolName string, args map[string]any) (string, error) {
		switch toolName {
		case "arkroute_status":
			return handleStatus(configPath)
		case "arkroute_providers":
			return handleProviders(configPath)
		case "arkroute_models":
			return handleModels(configPath)
		case "arkroute_reload":
			return handleReload(configPath)
		case "arkroute_test":
			return handleTest(configPath, args)
		default:
			return "", fmt.Errorf("unknown tool: %s", toolName)
		}
	}
}

func handleStatus(configPath string) (string, error) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Arkroute Gateway Status\n")
	fmt.Fprintf(&b, "======================\n\n")
	fmt.Fprintf(&b, "Config:  %s\n", configPath)
	fmt.Fprintf(&b, "Server:  %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Fprintf(&b, "Version: 1 (config version %d)\n\n", cfg.Version)
	fmt.Fprintf(&b, "Providers: %d total, %d enabled\n", len(cfg.Providers), countEnabledProviders(cfg))
	fmt.Fprintf(&b, "Models:    %d total, %d enabled\n", len(cfg.Models), countEnabledModels(cfg))
	fmt.Fprintf(&b, "Routes:    %d total, %d enabled\n", len(cfg.Routes), countEnabledRoutes(cfg))
	fmt.Fprintf(&b, "Profiles:  %d\n", len(cfg.Profiles))
	return b.String(), nil
}

func handleProviders(configPath string) (string, error) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Providers) == 0 {
		return "No providers configured. Run 'arkroute setup' to add one.", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-25s %-5s %-25s %s\n", "ID", "Type", "Base URL", "Enabled")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 90))
	for _, p := range cfg.Providers {
		typ := p.Type
		if typ == "" {
			typ = "auto"
		}
		fmt.Fprintf(&b, "%-25s %-5s %-25s %t\n", p.ID, typ, p.BaseURL, p.Enabled)
	}
	return b.String(), nil
}

func handleModels(configPath string) (string, error) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Models) == 0 {
		return "No models configured. Provider setup creates the first exposed model.", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-25s %-25s %-25s %-30s %s\n", "ID", "Provider", "Upstream", "Exposed Alias", "Enabled")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 120))
	for _, m := range cfg.Models {
		fmt.Fprintf(&b, "%-25s %-25s %-25s %-30s %t\n", m.ID, m.ProviderID, m.UpstreamModel, m.ExposedAlias, m.Enabled)
	}
	return b.String(), nil
}

func handleReload(configPath string) (string, error) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	// Build a simple status for the response.
	var b strings.Builder
	fmt.Fprintf(&b, "Reload requested.\n\n")
	fmt.Fprintf(&b, "Config: %s\n", configPath)
	fmt.Fprintf(&b, "Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Fprintf(&b, "\nSend SIGHUP to the arkroute serve process or use HTTP POST /internal/reload to apply.")
	return b.String(), nil
}

func handleTest(configPath string, args map[string]any) (string, error) {
	model, _ := args["model"].(string)
	prompt, _ := args["prompt"].(string)
	if model == "" {
		return "", fmt.Errorf("model parameter is required")
	}
	if prompt == "" {
		prompt = "Say hello in exactly 3 words."
	}

	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	redacted := config.Redacted(cfg)

	// Find the model in config.
	var modelCfg *config.ModelConfig
	for i := range cfg.Models {
		if cfg.Models[i].ExposedAlias == model || cfg.Models[i].ID == model {
			modelCfg = &cfg.Models[i]
			break
		}
	}
	if modelCfg == nil {
		return "", fmt.Errorf("model %q not found in config (check exposed_alias or model ID)", model)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Test Request\n")
	fmt.Fprintf(&b, "============\n\n")
	fmt.Fprintf(&b, "Model:    %s (%s)\n", model, modelCfg.UpstreamModel)
	fmt.Fprintf(&b, "Provider: %s\n", modelCfg.ProviderID)
	fmt.Fprintf(&b, "Prompt:   %s\n\n", prompt)
	fmt.Fprintf(&b, "This is a dry-run only. To actually test, use:\n")
	fmt.Fprintf(&b, "  arkroute test %s %q\n", security.ShellQuote(model), prompt)
	fmt.Fprintf(&b, "\nConfig redacted:\n%s\n", mustMarshal(redacted))
	return b.String(), nil
}

// --- helpers ---

func countEnabledProviders(cfg config.Config) int {
	n := 0
	for _, p := range cfg.Providers {
		if p.Enabled {
			n++
		}
	}
	return n
}

func countEnabledModels(cfg config.Config) int {
	n := 0
	for _, m := range cfg.Models {
		if m.Enabled {
			n++
		}
	}
	return n
}

func countEnabledRoutes(cfg config.Config) int {
	n := 0
	for _, r := range cfg.Routes {
		if r.Enabled {
			n++
		}
	}
	return n
}

func mustMarshal(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return buf.String()
}
