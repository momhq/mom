package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short:  "Generate session-level observability data from transcript",
	Hidden: true,
	Long: `Reads hook JSON from stdin, parses the transcript file, and writes
session-level metrics to .mom/logs/session-<id>.json.

Used as a Claude Code hook command. Not typically called directly.`,
	RunE:          runLog,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runLog(cmd *cobra.Command, _ []string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}

	var input struct {
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
		Cwd            string `json:"cwd"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	if input.TranscriptPath == "" {
		return nil
	}

	cwd := input.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return nil
	}

	sessionLog, err := logbook.ParseTranscript(input.TranscriptPath, input.SessionID)
	if err != nil {
		return nil // never fail the hook
	}

	logsDir := filepath.Join(sc.Path, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil // best-effort
	}

	outPath := filepath.Join(logsDir, fmt.Sprintf("session-%s.json", input.SessionID))
	outData, _ := json.MarshalIndent(sessionLog, "", "  ")
	outData = append(outData, '\n')
	_ = os.WriteFile(outPath, outData, 0644)

	return nil
}
