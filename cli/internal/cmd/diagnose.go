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
		return nil
	}

	p.Diamond("diagnose")
	p.Blank()

	w := 22
	p.KeyValue("Sessions analyzed", fmt.Sprintf("%d", report.SessionsAnalyzed), w)
	p.KeyValue("Total interactions", fmt.Sprintf("%d", report.TotalInteractions), w)
	p.Blank()

	// Metrics with pass/fail indicators.
	metricLine := func(name string, value float64, target string, good bool) {
		val := fmt.Sprintf("%.2f  %s", value, p.MutedText("(target: "+target+")"))
		if good {
			p.Checkf("%-20s %s", name, val)
		} else {
			p.Failf("%-20s %s", name, val)
		}
	}

	metricLine("Memory-first ratio", report.MemoryFirstRatio, "> 0.5", report.MemoryFirstRatio >= 0.5)
	metricLine("Recall efficiency", report.RecallEfficiency, "> 0.3", report.RecallEfficiency >= 0.3)
	metricLine("Context rediscovery", report.ContextRediscovery, "< 0.2", report.ContextRediscovery <= 0.2)
	metricLine("Write-back rate", report.WriteBackRate, "> 0.1", report.WriteBackRate >= 0.1)

	compliance := int(report.ProtocolCompliance * float64(report.SessionsAnalyzed))
	compliancePct := int(report.ProtocolCompliance * 100)
	complianceVal := fmt.Sprintf("%d/%d sessions (%d%%)", compliance, report.SessionsAnalyzed, compliancePct)
	if report.ProtocolCompliance >= 0.8 {
		p.Checkf("%-20s %s", "Protocol compliance", complianceVal)
	} else {
		p.Failf("%-20s %s", "Protocol compliance", complianceVal)
	}

	// Warnings.
	if report.ContextRediscovery > 0.2 {
		p.Blank()
		p.Warnf("Context rediscovery is high — memory coverage gap detected")
	}
	if report.MemoryFirstRatio < 0.5 && report.SessionsAnalyzed > 0 {
		p.Warnf("Memory-first ratio below target — agent may be bypassing memory")
	}

	p.Blank()
	return nil
}
