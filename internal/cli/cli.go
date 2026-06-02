package cli

import (
	"fmt"
	"io"

	"bat.dev/arkrouter/internal/app"
	"bat.dev/arkrouter/internal/config"
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
	case "serve":
		if err := app.Serve(flagValue(args[2:], "--config")); err != nil {
			fmt.Fprintf(stderr, "serve failed: %v\n", err)
			return 1
		}
		return 0
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
		if err := app.PrintStatus(flagValue(args[2:], "--config"), stdout); err != nil {
			fmt.Fprintf(stderr, "status failed: %v\n", err)
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
			fmt.Fprintln(stderr, "usage: arkrouter test <model> <prompt>")
			return 2
		}
		if err := app.TestRoute(flagValue(args[4:], "--config"), args[2], args[3], stdout); err != nil {
			fmt.Fprintf(stderr, "test failed: %v\n", err)
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
	fmt.Fprintln(w, "Usage: arkrouter <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init              Create a local config")
	fmt.Fprintln(w, "  validate          Validate config")
	fmt.Fprintln(w, "  serve             Start the local Claude Code gateway")
	fmt.Fprintln(w, "  activate claude   Print Claude Code environment exports")
	fmt.Fprintln(w, "  status            Show route and upstream health")
	fmt.Fprintln(w, "  doctor            Diagnose local setup")
	fmt.Fprintln(w, "  config path       Print default config file path")
	fmt.Fprintln(w, "  provider list     List configured providers")
	fmt.Fprintln(w, "  model list        List configured models")
	fmt.Fprintln(w, "  route list        List configured routes")
	fmt.Fprintln(w, "  test              Test a model route")
	fmt.Fprintln(w, "  logs              Print JSONL trace logs")
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

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && args[0] == "path" {
		if err := app.ConfigPath(stdout); err != nil {
			fmt.Fprintf(stderr, "config path failed: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "usage: arkrouter config path\n")
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
	fmt.Fprintf(stderr, "usage: arkrouter provider list\n")
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
	fmt.Fprintf(stderr, "usage: arkrouter model list\n")
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
	fmt.Fprintf(stderr, "usage: arkrouter route list\n")
	return 2
}
