package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/cartographer"
	"github.com/vmarinogg/leo-core/cli/internal/scope"
	"github.com/vmarinogg/leo-core/cli/internal/transponder"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Scan existing code, docs, and commits to seed the memory",
	Long: `Bootstrap scans the chosen directory for code, markdown, dependency
manifests, and commit history to create initial memories.

By default it writes to the nearest .leo/ found by walking up from the
scan directory. Use --scope to override the target .leo/ location.`,
	RunE: runBootstrap,
}

func init() {
	bootstrapCmd.Flags().String("path", "", "Directory to scan (default: current directory)")
	bootstrapCmd.Flags().Bool("refresh", false, "Re-scan all files, ignoring the SHA256 cache")
	bootstrapCmd.Flags().Bool("dry-run", false, "Show what would be written without persisting")
	bootstrapCmd.Flags().Int("commit-depth", 200, "Number of recent commits to scan")
	bootstrapCmd.Flags().Int64("max-file-size", 2, "Skip files larger than this many MB")
	bootstrapCmd.Flags().String("scope", "", "Target scope label (user/org/repo/workspace/custom)")
}

func runBootstrap(cmd *cobra.Command, _ []string) error {
	scanPath, _ := cmd.Flags().GetString("path")
	refresh, _ := cmd.Flags().GetBool("refresh")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	commitDepth, _ := cmd.Flags().GetInt("commit-depth")
	maxFileSizeMB, _ := cmd.Flags().GetInt64("max-file-size")
	scopeLabel, _ := cmd.Flags().GetString("scope")

	if scanPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		scanPath = cwd
	}

	scanPath, err := filepath.Abs(scanPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Resolve target .leo/ directory.
	var targetScope scope.Scope
	var found bool

	if scopeLabel != "" {
		targetScope, found = scope.FindByLabel(scanPath, scopeLabel)
		if !found {
			return fmt.Errorf("no .leo/ with scope %q found from %s", scopeLabel, scanPath)
		}
	} else {
		targetScope, found = scope.NearestWritable(scanPath)
		if !found {
			return fmt.Errorf("no .leo/ directory found — run 'leo init' first")
		}
	}

	cfg := cartographer.DefaultConfig()
	cfg.CommitDepth = commitDepth
	cfg.MaxFileSizeMB = maxFileSizeMB
	cfg.Refresh = refresh
	cfg.DryRun = dryRun
	cfg.ScopeDir = targetScope.Path

	cart := cartographer.New(cfg)

	cmd.Printf("Scanning %s\n", scanPath)
	if dryRun {
		cmd.Println("  (dry-run: no memories will be written)")
	}

	result, err := cart.Scan(cmd.Context(), scanPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Print per-extractor results.
	printBootstrapProgress(cmd, result)

	// Write memories unless dry-run.
	written := 0
	if !dryRun && len(result.Drafts) > 0 {
		w, writeErr := writeDrafts(result.Drafts, targetScope.Path)
		if writeErr != nil {
			cmd.Printf("  ⚠ write error: %v\n", writeErr)
		}
		written = w
	}

	cmd.Println()
	if dryRun {
		cmd.Printf("Total: %d memories would be seeded in %.1fs.\n",
			len(result.Drafts), result.Duration().Seconds())
	} else {
		cmd.Printf("Total: %d memories seeded in %.1fs.\n",
			written, result.Duration().Seconds())
	}

	cmd.Println()
	cmd.Println("Suggested first questions:")
	cmd.Println("  · \"What does this project do?\"")
	cmd.Println("  · \"Which dependencies drive the core behavior?\"")
	cmd.Println("  · \"What was the last major refactor about?\"")

	// Emit telemetry.
	emitter := transponder.New(targetScope.Path, true)
	emitter.EmitCaptureEvent(transponder.CaptureEvent{
		CaptureID:        fmt.Sprintf("bootstrap-%d", time.Now().UnixMilli()),
		TS:               time.Now().UTC().Format(time.RFC3339),
		ExtractorModel:   "cartographer",
		ExtractorVersion: "v0.8.0",
		MemoriesProposed: len(result.Drafts),
		MemoriesAccepted: written,
		Tags:             []string{"bootstrap"},
		Summary:          fmt.Sprintf("bootstrap scan of %s", filepath.Base(scanPath)),
	})

	return nil
}

// printBootstrapProgress prints the per-extractor breakdown.
func printBootstrapProgress(cmd *cobra.Command, result *cartographer.Result) {
	order := []struct {
		key   string
		label string
	}{
		{"markdown", "Markdown"},
		{"dependencies", "Dependencies"},
		{"commits", "Commits"},
		{"todo-fixme", "TODO/FIXME"},
		{"ast", "AST (Go)"},
	}

	for _, item := range order {
		er, ok := result.ByExtractor[item.key]
		if !ok {
			continue
		}

		symbol := "✓"
		if er.Count > 0 && er.Ambiguous == er.Count {
			symbol = "⚠"
		}

		cmd.Printf("  %s %-16s — %3d memories  (%d EXTRACTED · %d INFERRED · %d AMBIGUOUS)\n",
			symbol,
			item.label,
			er.Count,
			er.Extracted,
			er.Inferred,
			er.Ambiguous,
		)
	}
}

// writeDrafts persists draft memories to .leo/memory/ as JSON files.
// Returns the count of successfully written memories.
func writeDrafts(drafts []cartographer.Draft, leoDir string) (int, error) {
	memDir := filepath.Join(leoDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return 0, fmt.Errorf("creating memory dir: %w", err)
	}

	written := 0
	for _, d := range drafts {
		if err := writeDraft(d, memDir); err != nil {
			// One bad draft should not stop the rest.
			continue
		}
		written++
	}
	return written, nil
}

// writeDraft writes a single draft as a JSON memory file.
func writeDraft(d cartographer.Draft, memDir string) error {
	now := time.Now().UTC()
	id := draftID(d)

	content := d.Content
	if content == nil {
		content = make(map[string]any)
	}
	content["summary"] = d.Summary

	lifecycle := "learning"
	if d.Type == "decision" || d.Type == "fact" || d.Type == "pattern" {
		lifecycle = "permanent"
	}

	doc := map[string]any{
		"id":              id,
		"type":            mapDraftType(d.Type),
		"summary":         d.Summary,
		"lifecycle":       lifecycle,
		"scope":           "project",
		"tags":            d.Tags,
		"created":         now.Format(time.RFC3339),
		"created_by":      "cartographer",
		"updated":         now.Format(time.RFC3339),
		"updated_by":      "cartographer",
		"confidence":      d.Confidence,
		"promotion_state": "draft",
		"classification":  "INTERNAL",
		"provenance": map[string]any{
			"source_file":   d.Provenance.SourceFile,
			"source_lines":  d.Provenance.SourceLines,
			"source_hash":   d.Provenance.SourceHash,
			"trigger_event": d.Provenance.TriggerEvent,
			"commit_sha":    d.Provenance.CommitSHA,
		},
		"content": content,
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := filepath.Join(memDir, id+".json")
	return os.WriteFile(path, data, 0644)
}

// mapDraftType converts cartographer draft types to KB schema types.
func mapDraftType(t string) string {
	switch t {
	case "decision":
		return "decision"
	case "fact":
		return "fact"
	case "pattern":
		return "pattern"
	case "learning":
		return "learning"
	default:
		return "fact"
	}
}

// draftID generates a short, deterministic ID for a draft memory.
func draftID(d cartographer.Draft) string {
	raw := d.Type + ":" + d.Summary + ":" + d.Provenance.SourceFile
	h := cartographer.DraftHash(raw)
	return draftTypePrefix(d.Type) + h[:12]
}

// draftTypePrefix returns a short prefix for the draft type.
func draftTypePrefix(t string) string {
	switch t {
	case "decision":
		return "dec-"
	case "fact":
		return "fact-"
	case "pattern":
		return "pat-"
	case "learning":
		return "learn-"
	default:
		return "mem-"
	}
}

// runBootstrapInline runs a bootstrap scan from within the init flow.
// scanDir is the directory to scan; leoDir is the .leo/ to write into.
func runBootstrapInline(cmd *cobra.Command, scanDir, leoDir string) error {
	cfg := cartographer.DefaultConfig()
	cfg.ScopeDir = leoDir

	cart := cartographer.New(cfg)

	cmd.Printf("Scanning %s for initial memories...\n", scanDir)

	result, err := cart.Scan(cmd.Context(), scanDir)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	printBootstrapProgress(cmd, result)

	written := 0
	if len(result.Drafts) > 0 {
		w, writeErr := writeDrafts(result.Drafts, leoDir)
		if writeErr != nil {
			cmd.Printf("  ⚠ write error: %v\n", writeErr)
		}
		written = w
	}

	cmd.Printf("  %d memories seeded in %.1fs.\n", written, result.Duration().Seconds())
	return nil
}
