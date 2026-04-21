package main

import (
	"os"

	"github.com/momhq/mom/cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
