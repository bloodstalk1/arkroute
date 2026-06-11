package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/app"
	"github.com/bloodstalk1/arkroute/internal/buildinfo"
	"github.com/bloodstalk1/arkroute/internal/config"
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) < 2 {
		isRunningTests := func() bool {
			for _, arg := range os.Args {
				if strings.HasPrefix(arg, "-test.") {
					return true
				}
			}
			return false
		}
		// Run setup by default for a smoother onboarding experience (similar to omniroute)
		options := app.SetupOptions{
			Port:           config.DefaultServerPort,
			ExitAfterPrint: isRunningTests(),
		}
		if err := app.Setup(options, stdout); err != nil {
			fmt.Fprintf(stderr, "setup failed: %v\n", err)
			return 1
		}
		return 0
	}

	switch args[1] {
	case "version":
		if hasFlag(args[2:], "--debug") {
			fmt.Fprint(stdout, buildinfo.Debug())
			return 0
		}
		fmt.Fprintln(stdout, buildinfo.Summary())
		return 0
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	case "init":
		path, err := app.InitConfig(flagValue(args[2:], "--config"), hasFlag(args[2:], "--force"))
		if err != nil {
			fmt.Fprintf(stderr, "init failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "created %s\n", path)
		return 0
	case "validate":
		if err := app.ValidateConfig(flagValue(args[2:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "validate failed: %v\n", err)
			return 1
		}
		return 0
	case "activate":
		return runActivate(args[2:], stdout, stderr)
	case "serve":
		if err := app.Serve(flagValue(args[2:], "--config")); err != nil {
			fmt.Fprintf(stderr, "serve failed: %v\n", err)
			return 1
		}
		return 0
	case "doctor":
		options := app.DoctorOptions{
			ConfigPath:         flagValue(args[2:], "--config"),
			ClaudeSettingsPath: flagValue(args[2:], "--claude-settings"),
		}
		if err := app.DoctorWithOptions(options, stdout); err != nil {
			fmt.Fprintf(stderr, "doctor failed: %v\n", err)
			return 1
		}
		return 0
	case "logs":
		tail := intFlagValue(args[2:], "--tail", 0)
		if err := app.PrintLogsTail(flagValue(args[2:], "--file"), tail, stdout); err != nil {
			fmt.Fprintf(stderr, "logs failed: %v\n", err)
			return 1
		}
		return 0
	case "status":
		if err := app.PrintStatus(flagValue(args[2:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "status failed: %v\n", err)
			return 1
		}
		return 0
	case "reload":
		if err := app.Reload(flagValue(args[2:], "--config"), flagValue(args[2:], "--addr"), flagValue(args[2:], "--client-key"), stdout); err != nil {
			fmt.Fprintf(stderr, "reload failed: %v\n", err)
			return 1
		}
		return 0
	case "config":
		return runConfig(args[2:], stdout, stderr)
	case "provider":
		return runProvider(args[2:], stdout, stderr)
	case "model":
		return runModel(args[2:], stdout, stderr)
	case "route":
		return runRoute(args[2:], stdout, stderr)
	case "test":
		if len(args) < 4 {
			fmt.Fprintln(stderr, "usage: arkroute test <model> <prompt>")
			return 2
		}
		if err := app.TestRoute(flagValue(args[4:], "--config"), args[2], args[3], stdout); err != nil {
			fmt.Fprintf(stderr, "test failed: %v\n", err)
			return 1
		}
		return 0
	case "setup":
		options := app.SetupOptions{
			ConfigPath:     flagValue(args[2:], "--config"),
			NoBrowser:      hasFlag(args[2:], "--no-browser"),
			Host:           flagValue(args[2:], "--host"),
			Port:           intFlagValue(args[2:], "--port", config.DefaultServerPort),
			ExitAfterPrint: hasFlag(args[2:], "--exit-after-print"),
		}
		if err := app.Setup(options, stdout); err != nil {
			fmt.Fprintf(stderr, "setup failed: %v\n", err)
			return 1
		}
		return 0
	case "panel":
		options := app.PanelOptions{
			ConfigPath:     flagValue(args[2:], "--config"),
			NoBrowser:      hasFlag(args[2:], "--no-browser"),
			ExitAfterPrint: hasFlag(args[2:], "--exit-after-print"),
		}
		if err := app.Panel(options, stdout); err != nil {
			fmt.Fprintf(stderr, "panel failed: %v\n", err)
			return 1
		}
		return 0
	case "uninstall":
		options := app.UninstallOptions{
			ConfigPath:   flagValue(args[2:], "--config"),
			SettingsPath: flagValue(args[2:], "--settings"),
			Purge:        hasFlag(args[2:], "--purge"),
			Yes:          hasFlag(args[2:], "--yes"),
		}
		if err := app.Uninstall(options, stdout); err != nil {
			fmt.Fprintf(stderr, "uninstall failed: %v\n", err)
			return 1
		}
		return 0
	case "mcp":
		configPath := flagValue(args[2:], "--config")
		if configPath == "" {
			configPath = app.DefaultConfigPath()
		}
		if err := app.RunMCP(configPath); err != nil {
			fmt.Fprintf(stderr, "mcp failed: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[1])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: arkroute <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init              Create a local config")
	fmt.Fprintln(w, "  validate          Validate config")
	fmt.Fprintln(w, "  serve             Start the local Claude Code gateway")
	fmt.Fprintln(w, "  activate          Print client environment exports for claude, opencode, codex, or droid")
	fmt.Fprintln(w, "  status            Show route and upstream health")
	fmt.Fprintln(w, "  reload            Reload running server config")
	fmt.Fprintln(w, "  doctor            Diagnose local setup")
	fmt.Fprintln(w, "  config path       Print default config file path")
	fmt.Fprintln(w, "  provider list     List configured providers")
	fmt.Fprintln(w, "  model list        List configured models")
	fmt.Fprintln(w, "  route list        List configured routes")
	fmt.Fprintln(w, "  test              Test a model route")
	fmt.Fprintln(w, "  logs              Print JSONL trace logs")
	fmt.Fprintln(w, "  setup             Open local setup panel")
	fmt.Fprintln(w, "  panel             Open local control panel")
	fmt.Fprintln(w, "  uninstall         Remove Arkroute integration")
	fmt.Fprintln(w, "  mcp               Start MCP server on STDIO (JSON-RPC 2.0)")
	fmt.Fprintln(w, "  version           Print version")
}

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

func intFlagValue(args []string, name string, fallback int) int {
	value := flagValue(args, name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func runActivate(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: arkroute activate PROFILE")
		return 2
	}
	profile := strings.ToLower(strings.TrimSpace(args[0]))
	flags := args[1:]
	path := flagValue(flags, "--config")
	if path == "" {
		path = app.DefaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "activate failed: %v\n", err)
		return 1
	}
	if key := flagValue(flags, "--client-key"); key != "" {
		cfg.Server.ClientKey = key
	}
	settingsPath := flagValue(flags, "--settings")
	if hasFlag(flags, "--write-settings") {
		if profile != app.ClientProfileClaude {
			fmt.Fprintln(stderr, "activate failed: --write-settings is only supported for claude")
			return 1
		}
		if err := app.WriteClaudeSettings(settingsPath, cfg); err != nil {
			fmt.Fprintf(stderr, "activate failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "updated Claude settings: %s\n", app.ClaudeSettingsPath(settingsPath))
		return 0
	}
	if err := app.PrintClientActivation(stdout, cfg, profile); err != nil {
		if strings.Contains(err.Error(), "unknown client profile") {
			fmt.Fprintf(stderr, "activate failed: %v\n", err)
			return 2
		}
		fmt.Fprintf(stderr, "activate failed: %v\n", err)
		return 1
	}
	if profile == app.ClientProfileClaude {
		app.PrintClaudeActivationSettingsWarning(stdout, cfg, settingsPath)
	}
	return 0
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 {
		switch args[0] {
		case "path":
			if err := app.ConfigPath(stdout); err != nil {
				fmt.Fprintf(stderr, "config path failed: %v\n", err)
				return 1
			}
			return 0
		case "show":
			if err := app.ShowConfig(flagValue(args[1:], "--config"), stdout); err != nil {
				fmt.Fprintf(stderr, "config show failed: %v\n", err)
				return 1
			}
			return 0
		}
	}
	fmt.Fprintf(stderr, "usage: arkroute config path|show\n")
	return 2
}

func runProvider(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && args[0] == "list" {
		if err := app.ListProviders(flagValue(args[1:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "provider list failed: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "usage: arkroute provider list\n")
	return 2
}

func runModel(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && args[0] == "list" {
		if err := app.ListModels(flagValue(args[1:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "model list failed: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "usage: arkroute model list\n")
	return 2
}

func runRoute(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && args[0] == "list" {
		if err := app.ListRoutes(flagValue(args[1:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "route list failed: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "usage: arkroute route list\n")
	return 2
}
