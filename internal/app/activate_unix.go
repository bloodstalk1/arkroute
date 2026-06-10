//go:build !windows

package app

import (
	"fmt"
	"io"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
)

func PrintClaudeActivation(w io.Writer, cfg config.Config) {
	baseURL := localGatewayBaseURL(cfg)
	fmt.Fprintf(w, "export ANTHROPIC_BASE_URL=%s\n", security.ShellQuote(baseURL))
	fmt.Fprintf(w, "export ANTHROPIC_AUTH_TOKEN=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=%s\n", security.ShellQuote("1"))
	fmt.Fprintf(w, "export CLAUDE_CODE_AUTO_COMPACT_WINDOW=%s\n", security.ShellQuote(claudeAutoCompactWindow))
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

func printOpenAIClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintln(w, "# OPENAI_API_KEY below is Arkroute's local gateway token (server.client_key),")
	fmt.Fprintln(w, "# sent as Bearer auth to the local /v1 gateway. It is NOT an upstream provider key.")
	fmt.Fprintf(w, "export OPENAI_BASE_URL=%s\n", security.ShellQuote(localOpenAIBaseURL(cfg)))
	fmt.Fprintf(w, "export OPENAI_API_KEY=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export OPENAI_MODEL=%s\n", security.ShellQuote("sonnet"))
}

func printDroidClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintln(w, "# OPENAI_API_KEY below is Arkroute's local gateway token (server.client_key),")
	fmt.Fprintln(w, "# sent as Bearer auth to the local /v1 gateway. It is NOT an upstream provider key.")
	fmt.Fprintf(w, "export OPENAI_API_KEY=%s\n", security.ShellQuote(cfg.Server.ClientKey))
	fmt.Fprintf(w, "export ARKROUTE_OPENAI_BASE_URL=%s\n", security.ShellQuote(localOpenAIBaseURL(cfg)))
	fmt.Fprintf(w, "export ARKROUTE_OPENAI_MODEL=%s\n", security.ShellQuote("sonnet"))
	fmt.Fprintln(w, "# droidrun run --provider OpenAILike --model \"$ARKROUTE_OPENAI_MODEL\" --api_base \"$ARKROUTE_OPENAI_BASE_URL\" \"Open the settings app\"")
}
