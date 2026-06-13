//go:build windows

package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type shellType int

const (
	shellCMD shellType = iota
	shellPowerShell
	shellUnix
)

func detectShell() shellType {
	ppid := os.Getppid()
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return shellCMD
	}
	defer syscall.CloseHandle(snapshot)

	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	err = syscall.Process32First(snapshot, &pe)
	if err != nil {
		return shellCMD
	}

	for {
		if int(pe.ProcessID) == ppid {
			var chars []rune
			for _, c := range pe.ExeFile {
				if c == 0 {
					break
				}
				chars = append(chars, rune(c))
			}
			name := strings.ToLower(string(chars))
			if strings.Contains(name, "powershell") || strings.Contains(name, "pwsh") {
				return shellPowerShell
			}
			if strings.Contains(name, "bash") || strings.Contains(name, "zsh") || strings.Contains(name, "sh") {
				return shellUnix
			}
			break
		}
		err = syscall.Process32Next(snapshot, &pe)
		if err != nil {
			break
		}
	}
	return shellCMD
}

func printEnvVar(w io.Writer, shell shellType, name, value string) {
	switch shell {
	case shellPowerShell:
		escaped := strings.ReplaceAll(value, "\"", "`\"")
		fmt.Fprintf(w, "$env:%s = \"%s\"\n", name, escaped)
	case shellUnix:
		escaped := strings.ReplaceAll(value, "\"", "\\\"")
		fmt.Fprintf(w, "export %s=\"%s\"\n", name, escaped)
	default:
		// cmd.exe: set "NAME=VALUE"
		fmt.Fprintf(w, "set \"%s=%s\"\n", name, value)
	}
}

func printComment(w io.Writer, shell shellType, comment string) {
	switch shell {
	case shellPowerShell, shellUnix:
		fmt.Fprintf(w, "# %s\n", comment)
	default:
		fmt.Fprintf(w, "REM %s\n", comment)
	}
}

func PrintClaudeActivation(w io.Writer, cfg config.Config) {
	shell := detectShell()
	baseURL := config.LocalGatewayBaseURL(cfg)
	printEnvVar(w, shell, "ANTHROPIC_BASE_URL", baseURL)
	printEnvVar(w, shell, "ANTHROPIC_AUTH_TOKEN", cfg.Server.ClientKey)
	printEnvVar(w, shell, "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY", "1")
	printEnvVar(w, shell, "CLAUDE_CODE_AUTO_COMPACT_WINDOW", claudeAutoCompactWindow)
}

func PrintClaudeActivationSettingsWarning(w io.Writer, cfg config.Config, settingsPath string) {
	shell := detectShell()
	diagnosis, err := DiagnoseClaudeSettings(settingsPath, cfg)
	if err != nil {
		printComment(w, shell, fmt.Sprintf("warning: Claude settings at %s could not be read; environment commands were still printed.", ClaudeSettingsPath(settingsPath)))
		return
	}
	if !diagnosis.Exists || !diagnosis.HasBaseURL || !diagnosis.BaseURLMismatch {
		return
	}
	printComment(w, shell, fmt.Sprintf("warning: Claude settings at %s sets ANTHROPIC_BASE_URL=%s, expected %s.", diagnosis.Path, diagnosis.BaseURL, diagnosis.ExpectedBaseURL))
	fixCmd := "arkroute activate claude --write-settings"
	if settingsPath != "" {
		fixCmd = fmt.Sprintf("arkroute activate claude --write-settings --settings %s", settingsPath)
	}
	printComment(w, shell, fmt.Sprintf("fix: %s", fixCmd))
}

func printOpenAIClientActivation(w io.Writer, cfg config.Config) {
	shell := detectShell()
	printComment(w, shell, "OPENAI_API_KEY below is Arkroute's local gateway token (server.client_key),")
	printComment(w, shell, "sent as Bearer auth to the local /v1 gateway. It is NOT an upstream provider key.")
	printEnvVar(w, shell, "OPENAI_BASE_URL", localOpenAIBaseURL(cfg))
	printEnvVar(w, shell, "OPENAI_API_KEY", cfg.Server.ClientKey)
	printEnvVar(w, shell, "OPENAI_MODEL", "sonnet")
}

func printDroidClientActivation(w io.Writer, cfg config.Config) {
	shell := detectShell()
	printComment(w, shell, "OPENAI_API_KEY below is Arkroute's local gateway token (server.client_key),")
	printComment(w, shell, "sent as Bearer auth to the local /v1 gateway. It is NOT an upstream provider key.")
	printEnvVar(w, shell, "OPENAI_API_KEY", cfg.Server.ClientKey)
	printEnvVar(w, shell, "ARKROUTE_OPENAI_BASE_URL", localOpenAIBaseURL(cfg))
	printEnvVar(w, shell, "ARKROUTE_OPENAI_MODEL", "sonnet")
	switch shell {
	case shellPowerShell:
		printComment(w, shell, `droidrun run --provider OpenAILike --model $env:ARKROUTE_OPENAI_MODEL --api_base $env:ARKROUTE_OPENAI_BASE_URL "Open the settings app"`)
	case shellUnix:
		printComment(w, shell, `droidrun run --provider OpenAILike --model "$ARKROUTE_OPENAI_MODEL" --api_base "$ARKROUTE_OPENAI_BASE_URL" "Open the settings app"`)
	default:
		printComment(w, shell, `droidrun run --provider OpenAILike --model "%ARKROUTE_OPENAI_MODEL%" --api_base "%ARKROUTE_OPENAI_BASE_URL%" "Open the settings app"`)
	}
}
