package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/momhq/mom/cli/internal/diagnose"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
	"github.com/spf13/cobra"
)

var diagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Compute derived metrics from session logs",
	RunE:  runDiagnose,
}

func init() {
	diagnoseCmd.Flags().Int("last", 0, "Analyze only the last N sessions")
	diagnoseCmd.Flags().Bool("json", false, "Output as JSON")
}

func runDiagnose(cmd *cobra.Command, _ []string) error {
	lastN, _ := cmd.Flags().GetInt("last")
	jsonOut, _ := cmd.Flags().GetBool("json")

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return fmt.Errorf("no .mom/ directory found. Run 'mom init' first")
	}

	logsDir := filepath.Join(sc.Path, "logs")
	sessions, err := diagnose.LoadSessionLogs(logsDir, lastN)
	if err != nil {
		return fmt.Errorf("loading session logs: %w", err)
	}

	p := ux.NewPrinter(cmd.OutOrStdout())
	if len(sessions) == 0 {
		p.Muted("No session logs found. Run some sessions with Logbook active first.")
		return nil
	}

	report := diagnose.ComputeReport(sessions)

	if jsonOut {
		data, _ := json.MarshalIndent(report, "", "  ")
		cmd.Println(string(data))
	} else {
		p.Text(diagnose.FormatReport(report))
	}

	return nil
}
