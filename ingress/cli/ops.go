package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/momhq/mom/ops/daemon"
	"github.com/momhq/mom/services/projection"
	"github.com/momhq/mom/shared/project"
	"github.com/momhq/mom/shared/ux"
	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory status summary",
	RunE:  runStatus,
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check .mom/ health and local setup issues",
	RunE:  runDoctor,
}

// runStatus implements `mom status`. It reads the append-only Ledger
// (the source of truth) directly — no SQLite — and reports the project
// binding, Ledger stats, and the vault fold watermark.
func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	p := ux.NewPrinter(cmd.OutOrStdout())
	p.Bold("MOM")
	p.KeyValue("cwd", cwd, 12)
	if id, found := project.IdForCwd(); found {
		p.KeyValue("project", id, 12)
	} else {
		p.KeyValue("project", "(unbound — run /mom-project to bind this directory)", 12)
	}

	// Ledger stats: total events, head offset, time span, counts by type.
	ledgerDir, err := librarian.LedgerDir()
	if err != nil {
		return err
	}
	p.KeyValue("ledger", ledgerDir, 12)
	sum, err := computeLedgerSummary(ledgerDir)
	if err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	}
	if sum.total == 0 {
		p.KeyValue("events", "0 (no transcripts captured yet — run `mom watch`)", 12)
	} else {
		p.KeyValue("events", fmt.Sprintf("total %d, head offset %d", sum.total, sum.head), 12)
		p.KeyValue("span", fmt.Sprintf("%s → %s", sum.earliest.Format(time.RFC3339), sum.latest.Format(time.RFC3339)), 12)
		p.KeyValue("by type", formatTypeCounts(sum.byType), 12)
	}

	// Vault watermark for the bound project (if any).
	if root, ok := projectRoot(cwd); ok {
		if st, found, lerr := projection.LoadFoldState(root); lerr == nil && found {
			p.KeyValue("vault", fmt.Sprintf("folded to offset %d, %d files, %s",
				st.LastOffset, st.FilesWritten, st.FoldedAt.Format(time.RFC3339)), 12)
		} else {
			p.KeyValue("vault", "(not folded yet — run `mom vault fold`)", 12)
		}
	}

	p.KeyValue("watcher", cliWatcherState(), 12)
	return nil
}

// ledgerSummary is the aggregate view of the Ledger shown by `mom status`.
type ledgerSummary struct {
	total            int
	head             uint64
	earliest, latest time.Time
	byType           map[string]int
}

// computeLedgerSummary scans the whole Ledger once. A missing/empty Ledger
// yields a zero-valued summary, not an error.
func computeLedgerSummary(dir string) (ledgerSummary, error) {
	sum := ledgerSummary{byType: map[string]int{}}
	l, err := ledger.Open(dir)
	if err != nil {
		return sum, err
	}
	defer func() { _ = l.Close() }()

	it := l.Iterate(0)
	defer func() { _ = it.Close() }()
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		sum.total++
		sum.head = rec.Offset
		sum.byType[string(rec.Event.Type)]++
		if sum.earliest.IsZero() || rec.AppendedAt.Before(sum.earliest) {
			sum.earliest = rec.AppendedAt
		}
		if rec.AppendedAt.After(sum.latest) {
			sum.latest = rec.AppendedAt
		}
	}
	return sum, it.Err()
}

// formatTypeCounts renders the by-type histogram deterministically
// (descending count, then alphabetical).
func formatTypeCounts(byType map[string]int) string {
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(byType))
	for k, v := range byType {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	out := ""
	for i, p := range pairs {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%s %d", p.k, p.v)
	}
	return out
}

// projectRoot resolves the bound project's root directory from cwd. The
// fold watermark lives at <root>/.mom/vault/.fold-state.json.
func projectRoot(cwd string) (string, bool) {
	_, sourceFile, found, err := project.ResolveProject(cwd)
	if err != nil || !found {
		return "", false
	}
	return filepath.Dir(sourceFile), true
}

func cliWatcherState() string {
	health, err := daemon.StatusGlobal()
	if err != nil || len(health.Services) == 0 {
		return "unknown"
	}
	if health.Services[0].DaemonRunning {
		return "active"
	}
	return "inactive"
}
