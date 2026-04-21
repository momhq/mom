package main

import (
	"fmt"
	"os"

	"github.com/momhq/mom/cli/internal/cmd"
)

func main() {
	fmt.Fprintln(os.Stderr, "Warning: 'leo' is now 'mom'. Update your scripts.")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
