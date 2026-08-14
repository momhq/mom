package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/momhq/mom/ops/daemon"
	"github.com/momhq/mom/shared/ux"
	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
	"github.com/spf13/cobra"
)

// CheckStatus is one of "pass", "fail", or "warn".
type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusFail CheckStatus = "fail"
	StatusWarn CheckStatus = "warn"
)

// Check is one entry in the doctor health report.
type Check struct {
	Name       string
	Status     CheckStatus
	Detail     string
	NextAction string
}

// HealthReport is the structured output of BuildHealthReport — the
// canonical health view shared by the CLI and (eventually) Lens.
type HealthReport struct {
	Checks []Check
}

// HasFailures returns true if any check failed.
func (r HealthReport) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// BuildHealthReport runs every doctor check against the global install
// and returns a structured report. No network calls; only local files.
func BuildHealthReport() HealthReport {
	return HealthReport{
		Checks: []Check{
			checkLedger(),
			checkWatchDaemon(),
			checkHarnessContext(),
			checkMomVersion(),
		},
	}
}

// checkLedger verifies the append-only Ledger (the source of truth)
// exists and is openable. Replaces the retired SQLite-vault check.
func checkLedger() Check {
	dir, err := librarian.LedgerDir()
	if err != nil {
		return Check{Name: "ledger", Status: StatusFail, Detail: err.Error(),
			NextAction: "run 'mom init'"}
	}
	if _, err := os.Stat(dir); err != nil {
		return Check{Name: "ledger", Status: StatusFail, Detail: "ledger directory missing",
			NextAction: "run 'mom init', then 'mom watch' to start capturing"}
	}
	l, err := ledger.Open(dir)
	if err != nil {
		return Check{Name: "ledger", Status: StatusFail,
			Detail:     "ledger present but unopenable (corrupt or unreadable)",
			NextAction: "resolve disk issues, then 'mom vault rebuild'"}
	}
	_ = l.Close()
	return Check{Name: "ledger", Status: StatusPass, Detail: dir}
}

func checkWatchDaemon() Check {
	// GlobalDaemonInstalled is platform-specific: darwin/linux stat the
	// launchd plist / systemd unit file, Windows queries the Service
	// Control Manager for the registered service.
	installed, detail, err := daemon.GlobalDaemonInstalled()
	if err != nil {
		return Check{Name: "watch daemon", Status: StatusFail, Detail: err.Error()}
	}
	if !installed {
		return Check{Name: "watch daemon", Status: StatusFail,
			Detail:     detail,
			NextAction: "run 'mom init' to install the global watch daemon"}
	}
	return Check{Name: "watch daemon", Status: StatusPass, Detail: detail}
}

func checkHarnessContext() Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return Check{Name: "harness context", Status: StatusFail, Detail: err.Error()}
	}
	// MOM is harness-agnostic: the global context block may live in any
	// supported harness's home file. Pass if the block is present in ANY of
	// them, so a Codex-only or Pi-only install is not reported as broken.
	globalContextFiles := map[string]string{
		"claude": filepath.Join(home, ".claude", "CLAUDE.md"),
		"codex":  filepath.Join(home, ".codex", "AGENTS.md"),
		"pi":     filepath.Join(home, ".pi", "AGENTS.md"),
		"droid":  filepath.Join(home, ".factory", "AGENTS.md"),
	}
	var present []string
	for name, path := range globalContextFiles {
		data, rerr := os.ReadFile(path)
		if rerr == nil && strings.Contains(string(data), "BEGIN MOM GENERATED BLOCK") {
			present = append(present, name)
		}
	}
	if len(present) == 0 {
		return Check{Name: "harness context", Status: StatusFail,
			Detail:     "no harness context file carries the MOM block",
			NextAction: "run 'mom init' (add --harnesses to pick claude, codex, or pi)"}
	}
	sort.Strings(present)
	return Check{Name: "harness context", Status: StatusPass,
		Detail: "MOM block present (" + strings.Join(present, ", ") + ")"}
}

func checkMomVersion() Check {
	return Check{Name: "mom version", Status: StatusPass,
		Detail: fmt.Sprintf("%s (%s)", Version, Commit)}
}

// ─── command wiring ──────────────────────────────────────────────────────────

func init() {
	doctorCmd.Flags().Bool("bundle", false, "Print a redacted diagnostic bundle to stdout")
	doctorCmd.Long = `Check the global MOM install for health issues.

Runs only against local files; no network calls. Use --bundle to print a
redacted diagnostic blob suitable for pasting into a bug report.`
}

// runDoctor is the cobra entry point for ` + "`" + `mom doctor` + "`" + `.
func runDoctor(cmd *cobra.Command, args []string) error {
	bundle, _ := cmd.Flags().GetBool("bundle")
	report := BuildHealthReport()
	if bundle {
		renderBundle(cmd, report)
	} else {
		renderHuman(cmd, report)
	}
	if report.HasFailures() {
		return fmt.Errorf("one or more doctor checks failed")
	}
	return nil
}

func renderHuman(cmd *cobra.Command, r HealthReport) {
	p := ux.NewPrinter(cmd.OutOrStdout())
	for _, c := range r.Checks {
		line := c.Name
		if c.Detail != "" {
			line += ": " + c.Detail
		}
		switch c.Status {
		case StatusPass:
			p.Check(line)
		case StatusWarn:
			p.Warn(line)
		case StatusFail:
			p.Fail(line)
			if c.NextAction != "" {
				p.Muted("  → " + c.NextAction)
			}
		}
	}
}

func renderBundle(cmd *cobra.Command, r HealthReport) {
	fmt.Fprintln(cmd.OutOrStdout(), "=== MOM DOCTOR BUNDLE ===")
	for _, c := range r.Checks {
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\n", c.Status, c.Name, c.Detail)
		if c.NextAction != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  next: %s\n", c.NextAction)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "=== END BUNDLE ===")
}
