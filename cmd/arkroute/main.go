package main

import (
	"os"

	"bat.dev/arkroute/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdout, os.Stderr))
}
