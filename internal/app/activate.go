package app

import (
	"fmt"
	"io"

	"bat.dev/arkroute/internal/config"
	"bat.dev/arkroute/internal/security"
)

func PrintClaudeActivation(w io.Writer, cfg config.Config) {
	baseURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Fprintf(w, "export ANTHROPIC_BASE_URL=%s\n", security.ShellQuote(baseURL))
	fmt.Fprintf(w, "export ANTHROPIC_AUTH_TOKEN=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=%s\n", security.ShellQuote("1"))
}
