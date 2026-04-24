package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/config"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/spf13/cobra"
)

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Delete old raw JSONL recordings based on retention policy",
	Long: `Scans .mom/raw/ and deletes .jsonl files whose modification time
exceeds the configured retention period (default: 30 days).

Never deletes the current day's files. Configurable via config.yaml:

  raw_memories:
    retention_days: 30
    auto_clean: false`,
	RunE:          runSweep,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runSweep(cmd *cobra.Command, _ []string) error {
	cwd, _ := os.Getwd()
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return fmt.Errorf("no .mom/ found from %q", cwd)
	}

	cfg, err := config.Load(sc.Path)
	if err != nil {
		def := config.Default()
		cfg = &def
	}

	result := sweep(sc.Path, cfg.RawMemories)
	if result.Errors > 0 {
		fmt.Fprintf(os.Stderr, "sweep: deleted %d files, freed %.1f MB (%d errors)\n",
			result.Deleted, float64(result.BytesFreed)/(1024*1024), result.Errors)
	} else if result.Deleted > 0 {
		fmt.Fprintf(os.Stderr, "sweep: deleted %d files, freed %.1f MB\n",
			result.Deleted, float64(result.BytesFreed)/(1024*1024))
	} else {
		fmt.Fprintf(os.Stderr, "sweep: nothing to clean\n")
	}
	return nil
}

// SweepResult holds the outcome of a sweep operation.
type SweepResult struct {
	Deleted    int
	BytesFreed int64
	Errors     int
}

// sweep deletes old .jsonl files from .mom/raw/ based on retention config.
// Safe: never deletes files modified today, only touches *.jsonl files.
func sweep(momDir string, cfg config.RawMemoriesConfig) SweepResult {
	var result SweepResult

	retentionDays := cfg.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	rawDir := filepath.Join(momDir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return result
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	today := time.Now().Truncate(24 * time.Hour)

	var deleted []string
	for _, e := range entries {
		isCursor := strings.HasPrefix(e.Name(), ".cursor-")
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".jsonl") && !isCursor) {
			continue
		}

		info, err := e.Info()
		if err != nil {
			result.Errors++
			continue
		}

		modTime := info.ModTime()

		// Never delete files modified today.
		if !modTime.Before(today) {
			continue
		}

		// Delete if older than retention period.
		if modTime.Before(cutoff) {
			path := filepath.Join(rawDir, e.Name())
			size := info.Size()
			if err := os.Remove(path); err != nil {
				result.Errors++
				continue
			}
			result.Deleted++
			result.BytesFreed += size
			deleted = append(deleted, fmt.Sprintf("%s (%.1f MB)", e.Name(), float64(size)/(1024*1024)))
		}
	}

	// Log deletions.
	if len(deleted) > 0 {
		logSweep(momDir, deleted, result.BytesFreed)
	}

	return result
}

// logSweep appends deletion records to .mom/logs/sweep.log.
func logSweep(momDir string, deleted []string, totalBytes int64) {
	logsDir := filepath.Join(momDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)
	logFile := filepath.Join(logsDir, "sweep.log")

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s sweep: deleted %d files, freed %.1f MB\n", ts, len(deleted), float64(totalBytes)/(1024*1024))
	for _, d := range deleted {
		fmt.Fprintf(f, "  - %s\n", d)
	}
}
