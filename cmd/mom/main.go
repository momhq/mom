package main

import (
	"os"

	"github.com/momhq/mom/ingress/cli"
	"github.com/momhq/mom/ops/daemon"
)

func main() {
	// On Windows, when this process was launched by the Service Control
	// Manager, hands control to it and never returns. No-op elsewhere.
	daemon.RunWindowsServiceIfNeeded()

	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
