//go:build !windows

package app

import (
	"fmt"
	"io"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
)

func PrintClaudeActivation(w io.Writer, cfg config.Config) {
	baseURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Fprintf(w, "export ANTHROPIC_BASE_URL=%s\n", security.ShellQuote(baseURL))
	fmt.Fprintf(w, "export ANTHROPIC_AUTH_TOKEN=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export ANTHROPIC_API_KEY=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=%s\n", security.ShellQuote("1"))
}

func PrintClaudeActivationSettingsWarning(w io.Writer, cfg config.Config, settingsPath string) {
	diagnosis, err := DiagnoseClaudeSettings(settingsPath, cfg)
	if err != nil {
		fmt.Fprintf(w, "# warning: Claude settings at %s could not be read; shell exports were still printed.\n", security.ShellQuote(ClaudeSettingsPath(settingsPath)))
		return
	}
	if !diagnosis.Exists || !diagnosis.HasBaseURL || !diagnosis.BaseURLMismatch {
		return
	}
	fmt.Fprintf(w, "# warning: Claude settings at %s sets ANTHROPIC_BASE_URL=%s, expected %s.\n", security.ShellQuote(diagnosis.Path), security.ShellQuote(diagnosis.BaseURL), security.ShellQuote(diagnosis.ExpectedBaseURL))
	fmt.Fprintf(w, "# fix: arkroute activate claude --write-settings")
	if settingsPath != "" {
		fmt.Fprintf(w, " --settings %s", security.ShellQuote(settingsPath))
	}
	fmt.Fprintln(w)
}
