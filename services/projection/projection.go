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
// A chunk that fails internally is already handled by the synthesizer's own
// fallback; if a chunk surfaces a hard error, FoldAll warns and keeps going.
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
		return FoldResult{Files: acc, Index: lastIndex, ClaudeBlock: buildClaudeBlock(in), Chunks: in.ExistingChunks}, nil
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

	return FoldResult{Files: acc, Index: lastIndex, ClaudeBlock: buildClaudeBlock(in), Chunks: chunkMap}, nil
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
}

// Synthesizer turns a window of FoldEvents into a FoldResult. Two
// implementations exist: DeterministicSynth (templated, free, fast) and
// ClaudeSynth (LLM, falls back to deterministic on any error).
type Synthesizer interface {
	Fold(ctx context.Context, in FoldInput) (FoldResult, error)
}
