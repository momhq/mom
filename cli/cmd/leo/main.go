package main

import (
	"os"

	"github.com/vmarinogg/leo-core/cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
