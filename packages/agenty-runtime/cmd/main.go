package main

import (
	"os"

	"github.com/masteryyh/agenty/pkg/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(cmd.ExitCode(err))
	}
}
