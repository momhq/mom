// Package projection implements MOM's v0.50 vault-experiment lane: a
// strictly-additive downstream consumer of the Ledger (ADR 0021) that
// projects a single project's captured events into a navigable markdown
// "vault" (an LLM-wiki) under the project's `.mom/vault/`, plus a small
// always-loaded pointer block in the project's `CLAUDE.md`.
//
// This lane never touches the central SQLite vault or the existing
// capture→Ledger→SQLite pipeline. It only reads the Ledger and writes
// project-local markdown. The agent then RETRIEVES memory by navigating
// those files instead of running a DB search.
//
// The pipeline is: Reader (Ledger → []FoldEvent) → Synthesizer (events
// → markdown FoldResult) → Writer (FoldResult → files + CLAUDE.md block).
// A JSON watermark at `.mom/vault/.fold-state.json` lets `fold` resume
// incrementally while `rebuild` starts from offset 0.
package projection

import (
	"context"
	"fmt"
	"time"
)

// defaultChunkSize is the number of events folded per synthesizer call in
// FoldAll. Kept below maxPromptEvents (80) so the in-prompt window never
// triggers and every chunk is folded whole.
const defaultChunkSize = 60

// FoldAll folds ALL of in.Events by threading vault state forward across
// ordered chunks of chunkSize, so older history is no longer dropped by a
// single windowed call. The accumulating files map is seeded from
// in.Existing; each chunk's synthesis sees the files produced so far as its
// Existing set. The returned Index is the last chunk's Index, and the
// ClaudeBlock is always the deterministic block for the overall window.
//
// A chunk that surfaces a hard error is warned about and skipped. FoldAll is
// the flat driver for plain Synthesizers (tests, custom engines); the CLI's
// hierarchical path is FoldHierarchical, which stops the watermark short on
// failure instead.
func FoldAll(ctx context.Context, synth Synthesizer, in FoldInput, chunkSize int) (FoldResult, error) {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	// Seed the accumulator from the existing vault.
	acc := map[string]string{}
	for p, c := range in.Existing {
		acc[p] = c
	}

	warn := func(string) {}
	if ls, ok := synth.(*LLMSynth); ok && ls.Warn != nil {
		warn = ls.Warn
	}
	progress := in.Progress
	if progress == nil {
		progress = func(string) {}
	}

	lastIndex := ""
	events := in.Events
	if len(events) == 0 {
		// Nothing new — run a single pass so INDEX/files refresh against the
		// existing vault (matches single-Fold behavior on an empty window).
		single := in
		single.Existing = acc
		res, err := synth.Fold(ctx, single)
		if err != nil {
			return FoldResult{}, err
		}
		for p, c := range res.Files {
			acc[p] = c
		}
		lastIndex = res.Index
		return FoldResult{Files: acc, Index: lastIndex, ClaudeBlock: buildClaudeBlock(in), Chunks: in.ExistingChunks, FoldedThrough: in.ToOffset}, nil
	}

	// chunkMap accumulates chunkID → vault path across all chunks this fold.
	chunkMap := map[string]string{}

	total := (len(events) + chunkSize - 1) / chunkSize
	for i := 0; i < len(events); i += chunkSize {
		end := i + chunkSize
		if end > len(events) {
			end = len(events)
		}
		chunk := events[i:end]
		chunkNum := (i / chunkSize) + 1

		// Compute content-addressed ID for this chunk.
		offsets := make([]uint64, len(chunk))
		for j, e := range chunk {
			offsets[j] = e.Offset
		}
		cid := chunkID(in.ProjectID, offsets)

		// Skip synthesis if this chunk was already synthesized. When
		// existingPath is non-empty, verify the file is still in the vault.
		// When empty, the chunk produced no standalone file (e.g. deterministic
		// synth folds into timeline/topics), but the events are already folded.
		if existingPath, seen := in.ExistingChunks[cid]; seen {
			if existingPath == "" {
				progress(fmt.Sprintf("chunk %d/%d (cached)", chunkNum, total))
				chunkMap[cid] = ""
				continue
			}
			if _, exists := acc[existingPath]; exists {
				progress(fmt.Sprintf("chunk %d/%d (cached)", chunkNum, total))
				chunkMap[cid] = existingPath
				continue
			}
		}

		progress(fmt.Sprintf("chunk %d/%d — synthesizing %d events", chunkNum, total, len(chunk)))

		// Snapshot the accumulator as this chunk's Existing.
		existing := make(map[string]string, len(acc))
		for p, c := range acc {
			existing[p] = c
		}

		chunkIn := FoldInput{
			ProjectID:   in.ProjectID,
			ProjectRoot: in.ProjectRoot,
			FromOffset:  chunk[0].Offset,
			ToOffset:    chunk[len(chunk)-1].Offset,
			Existing:    existing,
			Events:      chunk,
		}
		res, err := synth.Fold(ctx, chunkIn)
		if err != nil {
			warn(fmt.Sprintf("chunk %d/%d failed (%v); skipping and continuing", chunkNum, total, err))
			continue
		}
		for p, c := range res.Files {
			acc[p] = c
		}
		if res.Index != "" {
			lastIndex = res.Index
		}
		// Record chunk → path mapping. Use the episode path if present,
		// otherwise record the chunkID with an empty path so it's tracked.
		episodePath := fmt.Sprintf("episodes/%s.md", cid)
		if _, ok := acc[episodePath]; ok {
			chunkMap[cid] = episodePath
		} else {
			chunkMap[cid] = ""
		}
	}

	return FoldResult{Files: acc, Index: lastIndex, ClaudeBlock: buildClaudeBlock(in), Chunks: chunkMap, FoldedThrough: in.ToOffset}, nil
}

// FoldEvent is a normalized, projection-facing view of a single Ledger
// Record. The Reader builds these from herald turn/memory events; the
// Synthesizer consumes only these (never raw Records).
type FoldEvent struct {
	Offset    uint64    `json:"offset"`
	Type      string    `json:"type"`
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	Role      string    `json:"role,omitempty"`
	Text      string    `json:"text,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

// FoldInput is the full context handed to a Synthesizer: the project
// identity, the offset window being folded, the existing vault files
// (so synthesis can incrementally update rather than regenerate), and
// the new events.
type FoldInput struct {
	ProjectID   string
	ProjectRoot string
	FromOffset  uint64
	ToOffset    uint64
	// Existing maps a vault-relative path (e.g. "topics/voice.md") to its
	// current content. Empty on rebuild.
	Existing map[string]string
	Events   []FoldEvent
	// ExistingChunks maps chunkID → vault-relative path from the last
	// FoldState. FoldAll skips re-synthesizing chunks whose ID is present
	// here and whose file still exists in Existing. Nil on rebuild.
	ExistingChunks map[string]string
	// Engine is the human-readable engine name stamped into INDEX.md (e.g. "claude").
	// Empty → "deterministic".
	Engine string
	// Progress, if non-nil, is called with a short human-readable label whenever
	// a meaningful step starts. Used to drive a spinner in the CLI.
	Progress func(string)
	// Checkpoint, if non-nil, is called after every durable synthesis step
	// (a successful L0 chunk, L1 subject, or the L2 identity) with the files
	// produced by that step, the full chunk map so far, and the consecutive
	// watermark reached. The CLI persists these immediately so an interrupted
	// fold (Ctrl-C, crash, power loss) loses at most one call's work.
	Checkpoint func(changed map[string]string, chunks map[string]string, foldedThrough uint64)
	// Parallel is the number of concurrent synthesis calls for the L0 and L1
	// passes. <=1 means sequential.
	Parallel int
	// ResumeSynthesis, when true, signals that L0 is already complete in Existing
	// and only the L1/L2 passes need to run. Set by the CLI when fold-state has
	// PendingSynthesis=true. FoldHierarchical skips the L0 loop entirely and
	// seeds its episode set from the on-disk Existing files.
	ResumeSynthesis bool
}

// FoldResult is what a Synthesizer returns: the set of vault files to
// write (path relative to .mom/vault → content), the INDEX.md router
// content, and the markdown that goes inside the CLAUDE.md managed block.
type FoldResult struct {
	// Files maps a vault-relative path → content. Does NOT include
	// INDEX.md (carried separately as Index).
	Files       map[string]string
	Index       string
	ClaudeBlock string
	// Chunks maps chunkID → vault-relative path for every chunk synthesized
	// (or reused from ExistingChunks) this fold. Written into FoldState so
	// the next incremental fold can skip unchanged chunks.
	Chunks map[string]string
	// FoldedThrough is the Ledger offset the vault is consistent through —
	// the watermark the caller must persist. Equal to the input's ToOffset on
	// a full fold; behind it when synthesis stopped early on a failing window
	// (the next fold resumes exactly there).
	FoldedThrough uint64
	// PendingSynthesis is true when L0 succeeded but L1 or L2 was aborted due
	// to a systemic harness failure (usage limit, auth). The vault has all
	// episode files but missing or incomplete concept/identity files. The CLI
	// persists this in fold-state so the next fold can resume the L1/L2 passes
	// without re-synthesizing L0.
	PendingSynthesis bool
}

// Synthesizer turns a window of FoldEvents into a FoldResult. Synthesis is
// LLM-only — the vault structure depends on reasoning, so there is no
// templated fallback engine. A synthesizer that cannot produce a result
// returns an error and the fold driver stops the watermark short.
type Synthesizer interface {
	Fold(ctx context.Context, in FoldInput) (FoldResult, error)
}
