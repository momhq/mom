package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
	"github.com/vmarinogg/leo-core/cli/internal/cartographer"
	"github.com/vmarinogg/leo-core/cli/internal/gardener"
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
	bootstrapCmd.Flags().Bool("no-graph", false, "Skip opening the memory graph in the browser after bootstrap")
}

func runBootstrap(cmd *cobra.Command, _ []string) error {
	scanPath, _ := cmd.Flags().GetString("path")
	refresh, _ := cmd.Flags().GetBool("refresh")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	commitDepth, _ := cmd.Flags().GetInt("commit-depth")
	maxFileSizeMB, _ := cmd.Flags().GetInt64("max-file-size")
	scopeLabel, _ := cmd.Flags().GetString("scope")
	noGraph, _ := cmd.Flags().GetBool("no-graph")

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

	// For user/org scopes, discover child repos and scan each into its own .leo/.
	if targetScope.Label == "user" || targetScope.Label == "org" {
		return runMultiRepoBootstrap(cmd, scanPath, targetScope, cfg, dryRun)
	}

	cfg.ScopeDir = targetScope.Path

	isTTY := isTerminalWriter(cmd.OutOrStdout())

	// Wire up spinner and progress callback when running interactively.
	var spinner *bootstrapSpinner
	if isTTY {
		spinner = newBootstrapSpinner(os.Stderr)
		spinner.start("Scanning...")
		cfg.OnProgress = func(processed, total int) {
			spinner.update(fmt.Sprintf("Scanning... (%d / %d files)", processed, total))
		}
	}

	cart := cartographer.New(cfg)

	if !isTTY {
		cmd.Printf("Scanning %s\n", scanPath)
	}
	if dryRun {
		cmd.Println("  (dry-run: no memories will be written)")
	}

	result, err := cart.Scan(cmd.Context(), scanPath)

	if spinner != nil {
		spinner.stop()
	}

	if !isTTY {
		// Already printed above; nothing extra needed.
	} else {
		cmd.Printf("Scanning %s\n", scanPath)
	}

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

		// Regenerate index.json so memories are immediately visible to recall.
		adapter := storage.NewJSONAdapter(targetScope.Path)
		if err := adapter.Reindex(); err != nil {
			cmd.Printf("  ⚠ index rebuild error: %v\n", err)
		}
	}

	cmd.Println()
	if dryRun {
		cmd.Printf("Total: %d memories would be seeded in %.1fs.\n",
			len(result.Drafts), result.Duration().Seconds())
	} else {
		cmd.Printf("Total: %d memories seeded in %.1fs.\n",
			written, result.Duration().Seconds())
	}

	// After a real write, attempt landmark computation when corpus is large enough.
	if !dryRun {
		memDir := filepath.Join(targetScope.Path, "memory")
		cacheDir := filepath.Join(targetScope.Path, "cache")
		totalDocs := countMemoryDocs(memDir)

		// Always write tag graph for incremental future updates.
		_ = gardener.WriteTagGraph(memDir, cacheDir)

		if totalDocs >= gardener.MinDocsForLandmarks {
			n, err := gardener.ComputeLandmarks(memDir, 2.0)
			if err == nil {
				_ = n
				landmarkCount := countLandmarks(memDir)
				cmd.Printf("✓ %d landmarks identified\n", landmarkCount)
			}
		}

		// Build and open the memory graph unless suppressed.
		if !noGraph {
			data, graphErr := gardener.BuildGraphData(memDir, 50)
			if graphErr == nil && data.Stats.TotalDocs > 0 {
				outPath := filepath.Join(os.TempDir(), "leo-memory-graph.html")
				if writeErr := gardener.WriteGraphHTML(data, outPath); writeErr == nil {
					cmd.Printf("Graph written to %s\n", outPath)
					if openErr := openBrowser(outPath); openErr != nil {
						cmd.Printf("  Open the file in your browser to view the graph.\n")
					}
				}
			}
		}
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

// runMultiRepoBootstrap handles bootstrap for user/org scopes by scanning
// each child repo that has its own .leo/ independently, outputting per-repo
// progress grouped by repo name.
func runMultiRepoBootstrap(cmd *cobra.Command, scanPath string, targetScope scope.Scope, cfg cartographer.Config, dryRun bool) error {
	// Discover the parent dir: it's the directory containing the .leo/ (go one level up from .leo/).
	parentDir := filepath.Dir(targetScope.Path)

	// Find child repos: immediate children with .git/ (may or may not have .leo/).
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return fmt.Errorf("reading parent dir: %w", err)
	}

	type repoEntry struct {
		root   string
		leoDir string
	}
	var repos []repoEntry

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(parentDir, e.Name())
		gitPath := filepath.Join(child, ".git")
		leoPath := filepath.Join(child, ".leo")

		gitInfo, gitErr := os.Stat(gitPath)
		if gitErr != nil || !gitInfo.IsDir() {
			continue // not a git repo
		}

		leoInfo, leoErr := os.Stat(leoPath)
		if leoErr != nil || !leoInfo.IsDir() {
			cmd.Printf("  ⚠ %s — no .leo/ found, skipping (run 'leo init' in this repo first)\n", child)
			continue
		}

		repos = append(repos, repoEntry{root: child, leoDir: leoPath})
	}

	if len(repos) == 0 {
		cmd.Println("No initialized child repos found. Run 'leo init' in each repo first.")
		return nil
	}

	cmd.Printf("Multi-repo bootstrap: %d repos under %s\n", len(repos), parentDir)
	if dryRun {
		cmd.Println("  (dry-run: no memories will be written)")
	}

	totalProposed := 0
	totalWritten := 0

	isTTY := isTerminalWriter(cmd.OutOrStdout())

	for _, repo := range repos {
		repoName := filepath.Base(repo.root)
		cmd.Printf("\n  [%s]\n", repoName)

		repoCfg := cfg
		repoCfg.ScopeDir = repo.leoDir

		var repoSpinner *bootstrapSpinner
		if isTTY {
			repoSpinner = newBootstrapSpinner(os.Stderr)
			repoSpinner.start(fmt.Sprintf("Scanning %s...", repoName))
			repoCfg.OnProgress = func(processed, total int) {
				repoSpinner.update(fmt.Sprintf("Scanning %s... (%d / %d files)", repoName, processed, total))
			}
		}

		cart := cartographer.New(repoCfg)

		result, err := cart.Scan(cmd.Context(), repo.root)

		if repoSpinner != nil {
			repoSpinner.stop()
		}

		if err != nil {
			cmd.Printf("    ⚠ scan error: %v\n", err)
			continue
		}

		printBootstrapProgress(cmd, result)
		totalProposed += len(result.Drafts)

		if !dryRun && len(result.Drafts) > 0 {
			w, writeErr := writeDrafts(result.Drafts, repo.leoDir)
			if writeErr != nil {
				cmd.Printf("    ⚠ write error: %v\n", writeErr)
			}
			totalWritten += w
			cmd.Printf("    %d memories seeded in %.1fs.\n", w, result.Duration().Seconds())
		} else if dryRun {
			cmd.Printf("    %d memories would be seeded.\n", len(result.Drafts))
		}
	}

	cmd.Println()
	if dryRun {
		cmd.Printf("Total: %d memories would be seeded across %d repos.\n", totalProposed, len(repos))
	} else {
		cmd.Printf("Total: %d memories seeded across %d repos.\n", totalWritten, len(repos))
	}

	return nil
}

// printBootstrapProgress prints the per-extractor breakdown and cache summary.
func printBootstrapProgress(cmd *cobra.Command, result *cartographer.Result) {
	order := []struct {
		key   string
		label string
	}{
		{"markdown", "Markdown"},
		{"dependencies", "Dependencies"},
		{"commits", "Commits"},
		{"todo-fixme", "TODO/FIXME"},
		{"ast", "AST"},
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

		// For AST, print per-language breakdown if we have data.
		if item.key == "ast" && len(result.ByLanguage) > 0 {
			// Sort language names for deterministic output.
			langs := make([]string, 0, len(result.ByLanguage))
			for lang := range result.ByLanguage {
				langs = append(langs, lang)
			}
			sort.Strings(langs)
			for _, lang := range langs {
				count := result.ByLanguage[lang]
				displayLang := canonicalLanguageLabel(lang)
				cmd.Printf("    %-14s %d memories\n", displayLang, count)
			}
		}
	}

	// Cache summary line.
	total := result.CacheHits + result.CacheMisses
	if total > 0 {
		cmd.Printf("  ✓ %d cached · processing %d new\n", result.CacheHits, result.CacheMisses)
	}
}

// canonicalLanguageLabel returns a display-friendly label for an AST language tag.
func canonicalLanguageLabel(lang string) string {
	switch lang {
	case "go":
		return "Go"
	case "python":
		return "Python"
	case "javascript":
		return "JavaScript"
	case "typescript":
		return "TypeScript"
	case "tsx":
		return "TSX"
	case "rust":
		return "Rust"
	case "java":
		return "Java"
	case "ruby":
		return "Ruby"
	case "c":
		return "C"
	case "cpp":
		return "C++"
	case "csharp":
		return "C#"
	default:
		return lang
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

// countMemoryDocs returns the total number of .json files in memDir.
func countMemoryDocs(memDir string) int {
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			n++
		}
	}
	return n
}

// runBootstrapInline runs a bootstrap scan from within the init flow.
// scanDir is the directory to scan; leoDir is the .leo/ to write into.
func runBootstrapInline(cmd *cobra.Command, scanDir, leoDir string) error {
	cfg := cartographer.DefaultConfig()
	cfg.ScopeDir = leoDir

	isTTY := isTerminalWriter(cmd.OutOrStdout())

	var spinner *bootstrapSpinner
	if isTTY {
		spinner = newBootstrapSpinner(os.Stderr)
		spinner.start("Scanning...")
		cfg.OnProgress = func(processed, total int) {
			spinner.update(fmt.Sprintf("Scanning... (%d / %d files)", processed, total))
		}
	}

	cart := cartographer.New(cfg)

	if !isTTY {
		cmd.Printf("Scanning %s for initial memories...\n", scanDir)
	}

	result, err := cart.Scan(cmd.Context(), scanDir)

	if spinner != nil {
		spinner.stop()
	}

	if isTTY {
		cmd.Printf("Scanning %s for initial memories...\n", scanDir)
	}

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

// bootstrapSpinner is a simple TTY spinner that writes to an io.Writer (stderr).
// It uses a goroutine + ticker to update the spinner frame periodically.
type bootstrapSpinner struct {
	w       io.Writer
	frames  []string
	current atomic.Int32 // current frame index
	text    atomic.Value // current label string
	done    chan struct{}
}

// newBootstrapSpinner creates a spinner that writes to w.
func newBootstrapSpinner(w io.Writer) *bootstrapSpinner {
	s := &bootstrapSpinner{
		w:      w,
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		done:   make(chan struct{}),
	}
	s.text.Store("")
	return s
}

// start begins displaying the spinner with an initial label.
func (s *bootstrapSpinner) start(label string) {
	s.text.Store(label)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				idx := int(s.current.Add(1)) % len(s.frames)
				frame := s.frames[idx]
				txt := s.text.Load().(string)
				// \r returns to column 0; clear to end of line with spaces then re-print.
				fmt.Fprintf(s.w, "\r%s %s        \r%s %s", frame, txt, frame, txt)
			}
		}
	}()
}

// update sets a new spinner label; safe to call from any goroutine.
func (s *bootstrapSpinner) update(label string) {
	s.text.Store(label)
}

// stop halts the spinner and erases the spinner line.
func (s *bootstrapSpinner) stop() {
	close(s.done)
	// Erase the spinner line.
	fmt.Fprintf(s.w, "\r\033[K")
}
