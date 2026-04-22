package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/momhq/mom/cli/internal/drafter"
	"github.com/momhq/mom/cli/internal/memory"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/spf13/cobra"
)

var draftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Extract draft memories from raw recordings",
	Long: `Reads raw JSONL from .mom/raw/ and extracts structured draft memories
into .mom/memory/ using RAKE keyword extraction and BM25 ranking.

Can be invoked as a Claude Code hook (reads hook JSON from stdin) or
standalone (processes all raw entries since the last draft run).`,
	RunE:          runDraft,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// draftHookInput matches the hook payload shape (same as recorder).
type draftHookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

func runDraft(cmd *cobra.Command, _ []string) error {
	// Attempt to read hook JSON from stdin (non-blocking).
	var hookCwd string
	stdinData, err := io.ReadAll(os.Stdin)
	if err == nil && len(stdinData) > 0 {
		var input draftHookInput
		if jsonErr := json.Unmarshal(stdinData, &input); jsonErr == nil {
			hookCwd = input.Cwd
		}
	}

	// Resolve .mom/ directory.
	cwd := hookCwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		logDraftError(fmt.Errorf("no .mom/ found from %q", cwd))
		return nil
	}

	momDir := sc.Path
	rawDir := filepath.Join(momDir, "raw")
	memDir := filepath.Join(momDir, "memory")

	if err := os.MkdirAll(memDir, 0755); err != nil {
		logDraftError(fmt.Errorf("creating memory dir: %w", err))
		return nil
	}

	// Determine since: last draft run timestamp from marker file, or 24h ago.
	since := lastDraftTime(momDir)

	// Vocab function: collect tags from existing memory docs.
	vocabFn := func() []string {
		return collectVocab(memDir)
	}

	d := drafter.New(rawDir, memDir, vocabFn)
	drafts, err := d.Process(since)
	if err != nil {
		logDraftError(fmt.Errorf("processing raw: %w", err))
		return nil
	}

	if len(drafts) == 0 {
		return nil
	}

	// Write each draft as a memory doc.
	written := 0
	for _, dr := range drafts {
		if writeErr := writeDraftDoc(memDir, dr); writeErr != nil {
			logDraftError(fmt.Errorf("writing draft %s: %w", dr.ID, writeErr))
			continue
		}
		written++
	}

	// Update last-draft marker.
	updateDraftMarker(momDir)

	fmt.Fprintf(os.Stderr, "draft: wrote %d/%d drafts to %s\n", written, len(drafts), memDir)
	return nil
}

// writeDraftDoc writes a Draft as a memory.Doc JSON file.
func writeDraftDoc(memDir string, dr drafter.Draft) error {
	now := time.Now().UTC()
	doc := &memory.Doc{
		ID:             dr.ID,
		Scope:          "project",
		Tags:           dr.Tags,
		Created:        now,
		CreatedBy:      "mom-draft",
		SessionID:      dr.SourceSession,
		PromotionState: "draft",
		Classification: "INTERNAL",
		Provenance: &memory.Provenance{
			Runtime:       "mom-draft",
			TriggerEvent:  "draft",
			RawExhaustRef: dr.SourceFile,
		},
		Content: map[string]any{
			"text":         dr.Content,
			"source_lines": dr.SourceLines,
		},
	}

	path := filepath.Join(memDir, dr.ID+".json")
	return memory.SaveDoc(path, doc)
}

// lastDraftTime reads the last-draft timestamp marker, defaulting to 24h ago.
func lastDraftTime(momDir string) time.Time {
	markerFile := filepath.Join(momDir, "raw", ".draft-cursor")
	data, err := os.ReadFile(markerFile)
	if err != nil {
		return time.Now().Add(-24 * time.Hour)
	}
	t, err := time.Parse(time.RFC3339, string(data))
	if err != nil {
		return time.Now().Add(-24 * time.Hour)
	}
	return t
}

// updateDraftMarker writes the current time to the draft cursor marker.
func updateDraftMarker(momDir string) {
	markerFile := filepath.Join(momDir, "raw", ".draft-cursor")
	_ = os.WriteFile(markerFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644)
}

// collectVocab gathers tags from existing memory docs for BM25 seeding.
func collectVocab(memDir string) []string {
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var vocab []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		for _, tag := range doc.Tags {
			if !seen[tag] {
				seen[tag] = true
				vocab = append(vocab, tag)
			}
		}
	}
	return vocab
}

// logDraftError appends an error to .mom/logs/draft.log, best-effort.
func logDraftError(err error) {
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
	logFile := filepath.Join(logsDir, "draft.log")
	f, ferr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if ferr != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s draft cmd error: %v\n", ts, err)
}
