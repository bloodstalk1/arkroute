//go:build windows

package app

import (
	"fmt"
	"io"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func PrintClaudeActivation(w io.Writer, cfg config.Config) {
	baseURL := localGatewayBaseURL(cfg)
	fmt.Fprintf(w, "set ANTHROPIC_BASE_URL=%s\n", baseURL)
	fmt.Fprintf(w, "set ANTHROPIC_AUTH_TOKEN=%s\n", cfg.Server.ClientKey)
	fmt.Fprintf(w, "set ANTHROPIC_API_KEY=%s\n", cfg.Server.ClientKey)
	fmt.Fprintf(w, "set CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1\n")
	fmt.Fprintf(w, "set CLAUDE_CODE_AUTO_COMPACT_WINDOW=%s\n", claudeAutoCompactWindow)
}

func PrintClaudeActivationSettingsWarning(w io.Writer, cfg config.Config, settingsPath string) {
	diagnosis, err := DiagnoseClaudeSettings(settingsPath, cfg)
	if err != nil {
		fmt.Fprintf(w, "REM warning: Claude settings at %s could not be read; environment commands were still printed.\n", ClaudeSettingsPath(settingsPath))
		return
	}
	if !diagnosis.Exists || !diagnosis.HasBaseURL || !diagnosis.BaseURLMismatch {
		return
	}
	fmt.Fprintf(w, "REM warning: Claude settings at %s sets ANTHROPIC_BASE_URL=%s, expected %s.\n", diagnosis.Path, diagnosis.BaseURL, diagnosis.ExpectedBaseURL)
	fmt.Fprintf(w, "REM fix: arkroute activate claude --write-settings")
	if settingsPath != "" {
		fmt.Fprintf(w, " --settings %s", settingsPath)
	}
	fmt.Fprintln(w)
}

func printOpenAIClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "set OPENAI_BASE_URL=%s\n", localOpenAIBaseURL(cfg))
	fmt.Fprintf(w, "set OPENAI_API_KEY=%s\n", cfg.Server.ClientKey)
	fmt.Fprintf(w, "set OPENAI_MODEL=sonnet\n")
}

func printDroidClientActivation(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "set OPENAI_API_KEY=%s\n", cfg.Server.ClientKey)
	fmt.Fprintf(w, "set ARKROUTE_OPENAI_BASE_URL=%s\n", localOpenAIBaseURL(cfg))
	fmt.Fprintf(w, "set ARKROUTE_OPENAI_MODEL=sonnet\n")
	fmt.Fprintln(w, "REM droidrun run --provider OpenAILike --model \"%ARKROUTE_OPENAI_MODEL%\" --api_base \"%ARKROUTE_OPENAI_BASE_URL%\" \"Open the settings app\"")
}
