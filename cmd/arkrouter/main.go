package main

import (
	"os"

	"bat.dev/arkrouter/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdout, os.Stderr))
}
