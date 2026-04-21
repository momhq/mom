package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	leort "github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
	"github.com/vmarinogg/leo-core/cli/internal/config"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
	"github.com/vmarinogg/leo-core/cli/internal/scope"
)

func init() {
	doctorCmd.Flags().Bool("verbose", false, "Show memory breakdowns by confidence, promotion state, and classification")
	doctorCmd.Flags().Bool("telemetry-preview", false, "Show telemetry status and a sample event")
	doctorCmd.Flags().Bool("landmarks", false, "List top landmark memories at current scope")
	doctorCmd.Flags().Bool("bundle", false, "Print a redacted diagnostic bundle to stdout")

	// Update doctor command metadata.
	doctorCmd.Long = `Check .mom/ health and diagnose issues.

No network calls; this command reads only local files.

Use flags to access additional diagnostic sections:
  --verbose           Memory breakdowns by confidence, promotion state, classification
  --telemetry-preview Telemetry status and a sample event from today's file
  --landmarks         Top landmark memories at current scope
  --bundle            Redacted diagnostic bundle (stdout only, safe to share)`
}

// runDoctor is the main entry point for `leo doctor` and all its flag variants.
func runDoctor(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	telemetryPreview, _ := cmd.Flags().GetBool("telemetry-preview")
	landmarksMode, _ := cmd.Flags().GetBool("landmarks")
	bundle, _ := cmd.Flags().GetBool("bundle")

	if bundle {
		return runDoctorBundle(cmd)
	}
	if telemetryPreview {
		return runDoctorTelemetryPreview(cmd)
	}
	if landmarksMode {
		return runDoctorLandmarks(cmd)
	}
	return runDoctorBase(cmd, verbose)
}

// ─── base doctor ──────────────────────────────────────────────────────────────

func runDoctorBase(cmd *cobra.Command, verbose bool) error {
	leoDir, err := findMomDir()
	if err != nil {
		cmd.Printf("✗ .mom/ directory: not found — run 'mom init' first\n")
		return err
	}

	// Detect legacy layout (.mom/kb/ present = pre-v0.8.0 install).
	if _, statErr := os.Stat(filepath.Join(leoDir, "kb")); statErr == nil {
		cmd.Printf("⚠ Legacy layout detected (.mom/kb/ present)\n  Run 'mom upgrade' to migrate to the v0.8.0 flat layout.\n")
		return nil
	}

	failed := false

	// Check 1: .mom/ exists and is writable.
	if err := checkDirWritable(leoDir); err != nil {
		cmd.Printf("✗ .mom/ directory: %v\n", err)
		failed = true
	} else {
		cmd.Printf("✔ .mom/ directory: exists and writable\n")
	}

	// Check 2: config.yaml is valid.
	cfg, cfgErr := config.Load(leoDir)
	if cfgErr != nil {
		cmd.Printf("✗ config.yaml: %v\n", cfgErr)
		failed = true
	} else {
		cmd.Printf("✔ config.yaml: valid (runtimes: %s)\n", strings.Join(cfg.EnabledRuntimes(), ", "))
	}

	// Check 3: memory and core dirs exist.
	docsDir := filepath.Join(leoDir, "memory")
	if _, statErr := os.Stat(docsDir); statErr != nil {
		cmd.Printf("✗ memory/: %v\n", statErr)
		failed = true
	} else {
		cmd.Printf("✔ memory/: exists\n")
	}

	constraintsDir := filepath.Join(leoDir, "constraints")
	if _, statErr := os.Stat(constraintsDir); statErr != nil {
		cmd.Printf("⚠ constraints/: not found\n")
	} else {
		cmd.Printf("✔ constraints/: exists\n")
	}

	skillsDir := filepath.Join(leoDir, "skills")
	if _, statErr := os.Stat(skillsDir); statErr != nil {
		cmd.Printf("⚠ skills/: not found\n")
	} else {
		cmd.Printf("✔ skills/: exists\n")
	}

	// Check 4: All docs pass schema validation.
	diskDocIDs := make(map[string]bool)
	totalErrors := 0

	docErrors, docIDs := validateAllDocs(cmd, docsDir, "doc")
	totalErrors += docErrors
	for id := range docIDs {
		diskDocIDs[id] = true
	}

	constraintErrors, constraintIDs := validateAllDocs(cmd, constraintsDir, "constraint")
	totalErrors += constraintErrors
	for id := range constraintIDs {
		diskDocIDs[id] = true
	}

	skillErrors, skillIDs := validateAllDocs(cmd, skillsDir, "skill")
	totalErrors += skillErrors
	for id := range skillIDs {
		diskDocIDs[id] = true
	}

	if totalErrors > 0 {
		failed = true
	}

	// Check 5: Index consistency.
	if orphanFail := checkIndexConsistency(cmd, leoDir, diskDocIDs); orphanFail {
		failed = true
	}

	// Check 6: Communication mode.
	if cfg != nil {
		commMode := cfg.Communication.Mode
		if commMode == "" {
			commMode = "concise"
		}
		cmd.Printf("✔ communication mode: %s\n", commMode)
	}

	// Check 7: Version.
	cmd.Printf("✔ mom version: %s (%s)\n", Version, Commit)

	// Check 8: Telemetry status.
	if cfg != nil {
		if cfg.Telemetry.TelemetryEnabled() {
			cmd.Printf("✔ telemetry: enabled (local-only)\n")
		} else {
			cmd.Printf("⚠ telemetry: disabled\n")
		}
	}

	// Check 9: Active scopes + memory counts.
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		printScopesSection(cmd, cwd)
		if verbose {
			printVerboseMemoryBreakdown(cmd, cwd)
		}
	}

	// Check 10: Last session timestamp + recent errors from telemetry.
	if cfg != nil {
		telDir := filepath.Join(leoDir, "telemetry")
		printLastSession(cmd, telDir)
		printRecentErrors(cmd, telDir, 5)
	}

	// Check 11: Adapter capabilities.
	if cfg != nil {
		printAdapterCapabilities(cmd, cwd, cfg)
	}

	if failed {
		return fmt.Errorf("one or more doctor checks failed")
	}

	return nil
}

// ─── --verbose additions ──────────────────────────────────────────────────────

// printVerboseMemoryBreakdown reads all memory docs in scope and prints
// breakdowns by confidence, promotion_state, and classification.
func printVerboseMemoryBreakdown(cmd *cobra.Command, cwd string) {
	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		return
	}

	cmd.Printf("\nMemory breakdown (verbose):\n")

	for _, s := range scopes {
		memDir := filepath.Join(s.Path, "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}

		confidence := map[string]int{}
		promotion := map[string]int{}
		classification := map[string]int{}
		landmarks := 0

		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			doc, err := kb.LoadDoc(filepath.Join(memDir, e.Name()))
			if err != nil {
				continue
			}
			confidence[doc.Confidence]++
			promotion[doc.PromotionState]++
			classification[doc.Classification]++
			if doc.Landmark {
				landmarks++
			}
		}

		cmd.Printf("  Scope: %s (%s)\n", s.Label, shortenPath(s.Path))
		cmd.Printf("    Confidence:       EXTRACTED=%d  INFERRED=%d  AMBIGUOUS=%d\n",
			confidence["EXTRACTED"], confidence["INFERRED"], confidence["AMBIGUOUS"])
		cmd.Printf("    Promotion state:  draft=%d  curated=%d  validated=%d  deprecated=%d\n",
			promotion["draft"], promotion["curated"], promotion["validated"], promotion["deprecated"])
		cmd.Printf("    Classification:   PUBLIC=%d  INTERNAL=%d  CONFIDENTIAL=%d\n",
			classification["PUBLIC"], classification["INTERNAL"], classification["CONFIDENTIAL"])
		cmd.Printf("    Landmarks:        %d\n", landmarks)
	}

	// Capture pipeline latency from telemetry.
	leoDir, err := findMomDir()
	if err == nil {
		telDir := filepath.Join(leoDir, "telemetry")
		printCapturePipelineLatency(cmd, telDir)
		printExtractorModelUsage(cmd, telDir)
	}
}

// printCapturePipelineLatency computes p50/p95 of CaptureEvent latency from
// the last 7 days of telemetry. Latency is inferred from CaptureEvent.
func printCapturePipelineLatency(cmd *cobra.Command, telDir string) {
	events := readTelemetryWindow(telDir, 7)
	var latencies []int64

	for _, raw := range events {
		if raw["kind"] != "CaptureEvent" {
			continue
		}
		// CaptureEvent doesn't have explicit latency_ms; skip if not present.
		if v, ok := raw["latency_ms"]; ok {
			switch n := v.(type) {
			case float64:
				latencies = append(latencies, int64(n))
			}
		}
	}

	if len(latencies) == 0 {
		cmd.Printf("\n  Capture pipeline latency: no data\n")
		return
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	cmd.Printf("\n  Capture pipeline latency (last 7d): p50=%dms  p95=%dms\n", p50, p95)
}

// printExtractorModelUsage prints the top 5 extractor models used in last 7 days.
func printExtractorModelUsage(cmd *cobra.Command, telDir string) {
	events := readTelemetryWindow(telDir, 7)
	counts := map[string]int{}

	for _, raw := range events {
		if raw["kind"] != "CaptureEvent" {
			continue
		}
		if m, ok := raw["extractor_model"].(string); ok && m != "" {
			counts[m]++
		}
	}

	if len(counts) == 0 {
		cmd.Printf("  Extractor model usage (last 7d): no data\n")
		return
	}

	type pair struct {
		model string
		count int
	}
	var pairs []pair
	for m, c := range counts {
		pairs = append(pairs, pair{m, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })

	cmd.Printf("  Extractor model usage (last 7d):\n")
	for i, p := range pairs {
		if i >= 5 {
			break
		}
		cmd.Printf("    %-40s %d captures\n", p.model, p.count)
	}
}

// ─── --telemetry-preview ──────────────────────────────────────────────────────

func runDoctorTelemetryPreview(cmd *cobra.Command) error {
	leoDir, leoDirErr := findMomDir()

	// Config for telemetry enabled status.
	var telEnabled bool
	if leoDirErr == nil {
		cfg, cfgErr := config.Load(leoDir)
		if cfgErr == nil {
			telEnabled = cfg.Telemetry.TelemetryEnabled()
		} else {
			telEnabled = true // default
		}
	}

	cmd.Printf("Telemetry mode: LOCAL-ONLY (no network calls)\n")
	if !telEnabled {
		cmd.Printf("Status: disabled\n")
		cmd.Printf("\nTo enable: set telemetry.enabled: true in .mom/config.yaml\n")
		return nil
	}
	cmd.Printf("Status: enabled\n")

	if leoDirErr != nil {
		cmd.Printf("\n(no .mom/ directory found)\n")
		return nil
	}

	telDir := filepath.Join(leoDir, "telemetry")
	today := time.Now().UTC().Format("2006-01-02")
	todayFile := filepath.Join(telDir, today+".jsonl")

	// Count today's events by kind.
	todayEvents, todayRaw := readJSONLFile(todayFile)
	totalToday := len(todayEvents)
	kindCounts := map[string]int{}
	for _, ev := range todayEvents {
		if k, ok := ev["kind"].(string); ok {
			kindCounts[k]++
		}
	}

	cmd.Printf("Events written today: %d\n", totalToday)
	if totalToday > 0 {
		// Print counts in a stable order.
		for _, kind := range []string{"SessionEvent", "CaptureEvent", "MemoryMutation", "ConsumptionEvent", "RuntimeHealth"} {
			if c := kindCounts[kind]; c > 0 {
				cmd.Printf("  %s: %d\n", kind, c)
			}
		}
		// Any unexpected kinds.
		for k, c := range kindCounts {
			switch k {
			case "SessionEvent", "CaptureEvent", "MemoryMutation", "ConsumptionEvent", "RuntimeHealth":
				// already printed
			default:
				cmd.Printf("  %s: %d\n", k, c)
			}
		}
	}

	// Sample event: most recent (last line).
	if len(todayRaw) > 0 {
		lastLine := todayRaw[len(todayRaw)-1]
		// Pretty-print with indent.
		var pretty map[string]any
		if err := json.Unmarshal([]byte(lastLine), &pretty); err == nil {
			out, _ := json.MarshalIndent(pretty, "", "  ")
			cmd.Printf("\nSample event (most recent):\n%s\n", string(out))
		} else {
			cmd.Printf("\nSample event (most recent):\n%s\n", lastLine)
		}
	} else {
		cmd.Printf("\n(no events yet today)\n")
	}

	// File info.
	if info, err := os.Stat(todayFile); err == nil {
		size := info.Size()
		var sizeStr string
		if size < 1024 {
			sizeStr = fmt.Sprintf("%d B", size)
		} else {
			sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
		}
		rel := ".mom/telemetry/" + today + ".jsonl"
		cmd.Printf("\nFull file: %s (%s)\n", rel, sizeStr)
	} else {
		cmd.Printf("\nFull file: .mom/telemetry/%s.jsonl (not yet created)\n", today)
	}

	return nil
}

// ─── --landmarks ──────────────────────────────────────────────────────────────

const landmarkComputationThreshold = 100

func runDoctorLandmarks(cmd *cobra.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		cmd.Printf("No .mom/ directory found. Run 'mom init' first.\n")
		return nil
	}

	// Use nearest scope.
	s := scopes[0]
	memDir := filepath.Join(s.Path, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		cmd.Printf("No landmark memories found (memory/ unreadable).\n")
		return nil
	}

	// Count total memories first for threshold check.
	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}

	if len(jsonFiles) < landmarkComputationThreshold {
		cmd.Printf("No landmarks computed yet. Run 'mom reindex --landmarks' to compute.\n")
		cmd.Printf("(Graph below computation threshold: %d/%d memories)\n", len(jsonFiles), landmarkComputationThreshold)
		return nil
	}

	// Load all docs, filter landmarks.
	type landmarkEntry struct {
		doc *kb.Doc
	}
	var landmarks []landmarkEntry

	for _, name := range jsonFiles {
		doc, err := kb.LoadDoc(filepath.Join(memDir, name))
		if err != nil {
			continue
		}
		if doc.Landmark {
			landmarks = append(landmarks, landmarkEntry{doc: doc})
		}
	}

	if len(landmarks) == 0 {
		cmd.Printf("No landmarks found. Run 'mom reindex --landmarks' to compute.\n")
		return nil
	}

	// Sort by centrality_score desc.
	sort.Slice(landmarks, func(i, j int) bool {
		si, sj := 0.0, 0.0
		if landmarks[i].doc.CentralityScore != nil {
			si = *landmarks[i].doc.CentralityScore
		}
		if landmarks[j].doc.CentralityScore != nil {
			sj = *landmarks[j].doc.CentralityScore
		}
		return si > sj
	})

	cmd.Printf("Top landmarks at scope: %s (%s)\n\n", s.Label, shortenPath(s.Path))
	cmd.Printf("  %-30s  %-8s  %-12s  Tags\n", "Memory ID", "Centrality", "Last Updated")
	cmd.Printf("  %s\n", strings.Repeat("-", 80))

	shown := 0
	for _, lm := range landmarks {
		if shown >= 10 {
			break
		}
		doc := lm.doc
		centrality := 0.0
		if doc.CentralityScore != nil {
			centrality = *doc.CentralityScore
		}
		updated := doc.Updated.Format("2006-01-02")
		tagCount := len(doc.Tags)
		tagStr := strings.Join(doc.Tags, ", ")
		if len(tagStr) > 40 {
			tagStr = tagStr[:37] + "..."
		}
		summary := doc.Summary
		if summary == "" {
			summary = doc.ID
		}
		cmd.Printf("  %-30s  %.4f      %-12s  [%d] %s\n",
			truncate(doc.ID, 30), centrality, updated, tagCount, tagStr)
		if summary != doc.ID {
			cmd.Printf("    %s\n", truncate(summary, 76))
		}
		shown++
	}

	return nil
}

// ─── --bundle ────────────────────────────────────────────────────────────────

func runDoctorBundle(cmd *cobra.Command) error {
	cmd.Printf("=== MOM DIAGNOSTIC BUNDLE ===\n")
	cmd.Printf("Generated: (deterministic — no timestamp)\n")
	cmd.Printf("Note: All network calls: NONE. Local files only.\n\n")

	// Version info.
	cmd.Printf("--- Version ---\n")
	cmd.Printf("Mom:  %s (%s)\n", Version, Commit)
	cmd.Printf("Go:   %s\n", runtime.Version())
	cmd.Printf("OS:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	cmd.Printf("\n")

	leoDir, leoDirErr := findMomDir()
	if leoDirErr != nil {
		cmd.Printf("--- Error ---\n")
		cmd.Printf(".mom/ directory not found. Run 'mom init' first.\n")
		return nil
	}

	cfg, cfgErr := config.Load(leoDir)

	// Adapter status.
	cmd.Printf("--- Adapter Status ---\n")
	if cfgErr != nil {
		cmd.Printf("(config unavailable: %v)\n", cfgErr)
	} else {
		cwd, _ := os.Getwd()
		printBundleAdapterStatus(cmd, cwd, cfg)
	}
	cmd.Printf("\n")

	// Scope list (paths only).
	cmd.Printf("--- Scopes ---\n")
	cwd, _ := os.Getwd()
	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		cmd.Printf("(none)\n")
	} else {
		for _, s := range scopes {
			cmd.Printf("  %s  %s\n", s.Label, s.Path)
		}
	}
	cmd.Printf("\n")

	// Memory counts per type — no titles, no bodies.
	cmd.Printf("--- Memory Counts ---\n")
	typeCounts := map[string]int{}
	totalMem := 0
	for _, s := range scopes {
		memDir := filepath.Join(s.Path, "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			doc, err := kb.LoadDoc(filepath.Join(memDir, e.Name()))
			if err != nil {
				continue
			}
			typeCounts[doc.Type]++
			totalMem++
		}
	}
	cmd.Printf("Total: %d\n", totalMem)
	// Stable sort by type name.
	var types []string
	for t := range typeCounts {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		cmd.Printf("  %-15s %d\n", t, typeCounts[t])
	}
	cmd.Printf("\n")

	// Recent errors from RuntimeHealth — content stripped.
	cmd.Printf("--- Recent Errors ---\n")
	telDir := filepath.Join(leoDir, "telemetry")
	bundleErrors := readRecentErrors(telDir, 10)
	if len(bundleErrors) == 0 {
		cmd.Printf("(none)\n")
	} else {
		for _, e := range bundleErrors {
			errType := "(nil)"
			if e.ErrorType != nil {
				errType = *e.ErrorType
			}
			cmd.Printf("  ts=%s  runtime=%s  error_type=%s\n", e.TS, e.Runtime, errType)
		}
	}
	cmd.Printf("\n")

	// Telemetry summary — counts per kind, no event bodies.
	cmd.Printf("--- Telemetry Summary ---\n")
	if cfgErr == nil && !cfg.Telemetry.TelemetryEnabled() {
		cmd.Printf("Status: disabled\n")
	} else {
		cmd.Printf("Status: enabled (local-only)\n")
		events, _ := readJSONLFile(filepath.Join(telDir, time.Now().UTC().Format("2006-01-02")+".jsonl"))
		kindCounts := map[string]int{}
		for _, ev := range events {
			if k, ok := ev["kind"].(string); ok {
				kindCounts[k]++
			}
		}
		cmd.Printf("Events today: %d\n", len(events))
		var kinds []string
		for k := range kindCounts {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			cmd.Printf("  %s: %d\n", k, kindCounts[k])
		}
	}
	cmd.Printf("\n")

	cmd.Printf("=== END BUNDLE ===\n")
	return nil
}

func printBundleAdapterStatus(cmd *cobra.Command, cwd string, cfg *config.Config) {
	enabled := cfg.EnabledRuntimes()
	if len(enabled) == 0 {
		cmd.Printf("(no adapters enabled)\n")
		return
	}
	registry := leort.NewRegistry(cwd)
	for _, name := range enabled {
		adapter, ok := registry.Get(name)
		if !ok {
			cmd.Printf("  %s: unknown adapter\n", name)
			continue
		}
		cap := adapter.Capabilities()
		cmd.Printf("  %s v%s\n", cap.Name, cap.Version)
		if len(cap.Supports) > 0 {
			cmd.Printf("    supported:    %s\n", strings.Join(cap.Supports, ", "))
		}
		if len(cap.Experimental) > 0 {
			cmd.Printf("    experimental: %s\n", strings.Join(cap.Experimental, ", "))
		}
	}
}

// ─── telemetry helpers ────────────────────────────────────────────────────────

// runtimeHealthEvent is a minimal struct for reading RuntimeHealth events.
type runtimeHealthEvent struct {
	Kind          string  `json:"kind"`
	Runtime       string  `json:"runtime"`
	TS            string  `json:"ts"`
	WrapUpSuccess bool    `json:"wrap_up_success"`
	ErrorType     *string `json:"error_type"`
	LatencyMS     int64   `json:"latency_ms"`
}

// readJSONLFile reads a JSONL file, returning parsed events and raw lines.
// Gracefully handles missing or empty files.
func readJSONLFile(path string) ([]map[string]any, []string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var events []map[string]any
	var rawLines []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		rawLines = append(rawLines, line)
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			events = append(events, ev)
		}
	}

	return events, rawLines
}

// readTelemetryWindow reads the last N days of JSONL files from telDir.
func readTelemetryWindow(telDir string, days int) []map[string]any {
	now := time.Now().UTC()
	var all []map[string]any

	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		path := filepath.Join(telDir, date+".jsonl")
		events, _ := readJSONLFile(path)
		all = append(all, events...)
	}

	return all
}

// printLastSession finds and prints the timestamp of the most recent SessionEvent.
func printLastSession(cmd *cobra.Command, telDir string) {
	events := readTelemetryWindow(telDir, 7)
	var lastTS string

	for _, ev := range events {
		if ev["kind"] != "SessionEvent" {
			continue
		}
		ts := ""
		if s, ok := ev["started_at"].(string); ok {
			ts = s
		}
		if ts > lastTS {
			lastTS = ts
		}
	}

	if lastTS == "" {
		cmd.Printf("⚠ last session: no session events found\n")
	} else {
		cmd.Printf("✔ last session: %s\n", lastTS)
	}
}

// printRecentErrors reads the last N RuntimeHealth events with errors.
func printRecentErrors(cmd *cobra.Command, telDir string, limit int) {
	errors := readRecentErrors(telDir, limit)
	if len(errors) == 0 {
		return
	}

	cmd.Printf("⚠ recent runtime errors (%d):\n", len(errors))
	for _, e := range errors {
		errType := "(unknown)"
		if e.ErrorType != nil {
			errType = *e.ErrorType
		}
		cmd.Printf("  ts=%s  runtime=%s  error_type=%s\n", e.TS, e.Runtime, errType)
	}
}

// readRecentErrors returns at most limit RuntimeHealth events where ErrorType != nil.
func readRecentErrors(telDir string, limit int) []runtimeHealthEvent {
	events := readTelemetryWindow(telDir, 7)
	var errors []runtimeHealthEvent

	for _, raw := range events {
		if raw["kind"] != "RuntimeHealth" {
			continue
		}
		data, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var ev runtimeHealthEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		if ev.ErrorType == nil && ev.WrapUpSuccess {
			continue
		}
		if ev.ErrorType != nil || !ev.WrapUpSuccess {
			errors = append(errors, ev)
		}
	}

	// Return only the most recent ones.
	if len(errors) > limit {
		errors = errors[len(errors)-limit:]
	}
	return errors
}

// ─── shared helpers ───────────────────────────────────────────────────────────

// shortenPath replaces the home directory prefix with ~.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
}
