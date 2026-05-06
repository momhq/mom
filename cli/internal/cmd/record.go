package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/momhq/mom/cli/internal/librarian"
	"github.com/spf13/cobra"
)

var (
	recordSession string
	recordSummary string
	recordTags    []string
	recordActor   string
)

var recordCmd = &cobra.Command{
	Use:    "record",
	Short:  "Save an explicit memory from CLI input (CLI mirror of the mom_record MCP tool)",
	Hidden: true,
	Long: `Reads memory text from stdin and persists it to the central vault
($HOME/.mom/mom.db) as an explicit-write memory — bypassing Drafter's
content filters per ADR 0014. Tags are normalised before insert; if
any tag normalises to empty the request is dropped without persisting.

This command is the CLI mirror of the mom_record MCP tool. It is the
human-driven path for recording a memory from a shell pipeline:

  echo "decided to use Postgres for the canary deploy" | \
    mom record --session "$SID" --tags decision,deploy

Hook-friendly behaviour: legacy hook configs that pipe JSON to this
command silently exit 0 — the JSON shape is detected and discarded
rather than persisted as memory text.`,
	RunE:          runRecord,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	recordCmd.Flags().StringVar(&recordSession, "session", "", "Session ID this memory belongs to (required)")
	recordCmd.Flags().StringVar(&recordSummary, "summary", "", "One-line summary")
	recordCmd.Flags().StringSliceVar(&recordTags, "tags", nil, "Tag names (comma-separated; normalised before insert)")
	recordCmd.Flags().StringVar(&recordActor, "actor", "cli", "Calling agent / human label (defaults to 'cli')")
}

func runRecord(cmd *cobra.Command, _ []string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Hooks pipe whatever they have; never fail.
		return nil
	}
	text := strings.TrimSpace(string(data))

	// Hook-friendly bail-outs: missing session, empty input, or JSON
	// (legacy hook payload) all exit 0 without writing.
	if recordSession == "" {
		fmt.Fprintln(os.Stderr, "mom record: --session is required (skipping)")
		return nil
	}
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "{") {
		fmt.Fprintln(os.Stderr, "mom record: input looks like JSON (legacy hook payload?) — skipping")
		return nil
	}

	tags, err := normaliseRecordTags(recordTags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mom record: %v\n", err)
		return nil
	}

	lib, closeFn, err := openCentralLibrarian()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mom record: %v\n", err)
		return nil
	}
	defer func() { _ = closeFn() }()

	contentBytes, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mom record: marshal content: %v\n", err)
		return nil
	}

	id, err := lib.InsertMemoryWithTags(librarian.InsertMemory{
		Content:                string(contentBytes),
		Summary:                recordSummary,
		SessionID:              recordSession,
		ProvenanceActor:        recordActor,
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
	}, tags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mom record: insert: %v\n", err)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "recorded: id=%s session=%s tags=%v\n", id, recordSession, tags)
	return nil
}

// normaliseRecordTags mirrors mcp.normaliseTagsOrReject: every input
// tag is normalised; if any normalises to empty the whole list is
// rejected so we never persist a partial-tag memory.
func normaliseRecordTags(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	for i, t := range raw {
		n := librarian.NormalizeTagName(t)
		if n == "" {
			return nil, fmt.Errorf("tag %d (%q) normalises to empty; reject the request rather than persist a partial memory", i, t)
		}
		out = append(out, n)
	}
	return out, nil
}
