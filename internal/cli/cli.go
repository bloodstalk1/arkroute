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
