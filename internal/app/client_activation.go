package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
)

const (
	ClientProfileClaude   = "claude"
	ClientProfileOpenCode = "opencode"
	ClientProfileCodex   = "codex"
	ClientProfileDroid   = "droid"
)

func PrintClientActivation(w io.Writer, cfg config.Config, profile string) error {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		return fmt.Errorf("client profile is required")
	}
	if !isKnownClientProfile(profile) {
		return fmt.Errorf("unknown client profile %q", profile)
	}
	if err := validateActivationConfig(cfg); err != nil {
		return err
	}
	switch profile {
	case ClientProfileClaude:
		PrintClaudeActivation(w, cfg)
	case ClientProfileOpenCode, ClientProfileCodex:
		printOpenAIClientActivation(w, cfg)
	case ClientProfileDroid:
		printDroidClientActivation(w, cfg)
	}
	return nil
}

func isKnownClientProfile(profile string) bool {
	switch profile {
	case ClientProfileClaude, ClientProfileOpenCode, ClientProfileCodex, ClientProfileDroid:
		return true
	default:
		return false
	}
}

func validateActivationConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		return fmt.Errorf("server.client_key is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if !security.IsLoopbackHost(cfg.Server.Host) {
		return fmt.Errorf("server.host must be loopback for activation")
	}
	return nil
}

func localGatewayBaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

func localOpenAIBaseURL(cfg config.Config) string {
	return localGatewayBaseURL(cfg) + "/v1"
}
