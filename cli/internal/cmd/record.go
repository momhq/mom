package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/momhq/mom/cli/internal/recorder"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record raw conversation data from hook stdin",
	Long: `Reads hook JSON from stdin, extracts transcript_path, and appends
new turns to .mom/raw/ as JSONL. Idempotent — safe to call multiple times.

Used as a Claude Code hook command. Not typically called directly.`,
	RunE:          runRecord,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runRecord(cmd *cobra.Command, _ []string) error {
	// Read stdin.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		logRecordError(err)
		return nil // never fail the hook
	}

	var input recorder.HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		logRecordError(err)
		return nil
	}

	// Find nearest .mom/
	cwd := input.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		logRecordError(fmt.Errorf("no .mom/ found from %q", cwd))
		return nil
	}

	if err := recorder.Record(sc.Path, input); err != nil {
		logRecordError(err)
	}
	return nil // always exit 0
}

// logRecordError appends an error to .mom/logs/record.log, best-effort.
// It uses os.Getwd() to find .mom/ if available; otherwise discards.
func logRecordError(err error) {
	cwd, werr := os.Getwd()
	if werr != nil {
		return
	}
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return
	}
	logsDir := filepath.Join(sc.Path, "logs")
	_ = os.MkdirAll(logsDir, 0755)
	logFile := filepath.Join(logsDir, "record.log")
	f, ferr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if ferr != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s record cmd error: %v\n", ts, err)
}
