package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/recorder"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/spf13/cobra"
)

var recordRaw bool

var recordCmd = &cobra.Command{
	Use:   "record",
	Short:  "Record raw conversation data from hook stdin",
	Hidden: true,
	Long: `Reads hook JSON from stdin, extracts transcript_path, and appends
new turns to .mom/raw/ as JSONL. Idempotent — safe to call multiple times.

With --raw, reads plain text from stdin instead of Claude Code hook JSON.
This mode is used by runtimes that don't provide transcript_path (e.g. Cline).

Used as a hook command. Not typically called directly.`,
	RunE:          runRecord,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	recordCmd.Flags().BoolVar(&recordRaw, "raw", false, "Read plain text from stdin instead of Claude Code hook JSON")
}

func runRecord(cmd *cobra.Command, _ []string) error {
	// Read stdin.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		logRecordError(err)
		return nil // never fail the hook
	}

	// Find nearest .mom/
	cwd, _ := os.Getwd()
	if envDir := os.Getenv("MOM_PROJECT_DIR"); envDir != "" {
		cwd = envDir
	}

	if recordRaw {
		return runRecordRaw(data, cwd)
	}

	var input recorder.HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		logRecordError(err)
		return nil
	}

	// Windsurf puts transcript_path inside tool_info, not at the root.
	if input.TranscriptPath == "" {
		var wrapper struct {
			ToolInfo struct {
				TranscriptPath string `json:"transcript_path"`
			} `json:"tool_info"`
			TrajectoryID string `json:"trajectory_id"`
		}
		if json.Unmarshal(data, &wrapper) == nil && wrapper.ToolInfo.TranscriptPath != "" {
			input.TranscriptPath = wrapper.ToolInfo.TranscriptPath
			if input.SessionID == "" {
				input.SessionID = wrapper.TrajectoryID
			}
		}
	}

	if input.Cwd != "" {
		cwd = input.Cwd
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

// runRecordRaw handles --raw mode: plain text from stdin written directly to
// .mom/raw/ as a JSONL entry. Used by runtimes that don't provide
// transcript_path (Cline, Windsurf, etc.).
func runRecordRaw(data []byte, cwd string) error {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil // nothing to record
	}

	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		logRecordError(fmt.Errorf("no .mom/ found from %q", cwd))
		return nil
	}

	if err := recorder.RecordText(sc.Path, text, ""); err != nil {
		logRecordError(err)
	}
	return nil
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
