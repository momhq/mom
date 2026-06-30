package projection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// HierarchySynth wraps an LLMSynth to produce a three-level vault:
// L0 episodes (one per chunk) → L1 topics/timeline (synthesized from L0
// when enough episodes exist) → L2 summaries (when enough L1 files exist).
type HierarchySynth struct {
	inner       Synthesizer
	l1Threshold int // min episodes before triggering L1 synthesis (default 5)
	l2Threshold int // min L1 files before triggering L2 synthesis (default 10)
}

// NewHierarchySynth builds a hierarchy synthesizer wrapping inner.
func NewHierarchySynth(inner Synthesizer, l1Threshold, l2Threshold int) *HierarchySynth {
	if l1Threshold <= 0 {
		l1Threshold = 5
	}
	if l2Threshold <= 0 {
		l2Threshold = 10
	}
	return &HierarchySynth{inner: inner, l1Threshold: l1Threshold, l2Threshold: l2Threshold}
}

// Fold implements Synthesizer. For HierarchySynth, the caller should use
// FoldHierarchical instead; this method is provided so HierarchySynth
// satisfies the interface and can be used in type assertions.
func (hs *HierarchySynth) Fold(ctx context.Context, in FoldInput) (FoldResult, error) {
	return FoldHierarchical(ctx, hs, in, defaultChunkSize)
}

// FoldHierarchical runs the three-pass L0→L1→L2 fold. It is the entry
// point called by the CLI when the synthesizer is a HierarchySynth.
func FoldHierarchical(ctx context.Context, hs *HierarchySynth, in FoldInput, chunkSize int) (FoldResult, error) {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	warn := func(string) {}
	if ls, ok := hs.inner.(*LLMSynth); ok && ls.Warn != nil {
		warn = ls.Warn
	}
	progress := in.Progress
	if progress == nil {
		progress = func(string) {}
	}

	// ── L0 pass: one episode file per chunk ──────────────────────────────────
	acc := map[string]string{}
	for p, c := range in.Existing {
		acc[p] = c
	}

	chunkMap := map[string]string{}
	// Carry over already-synthesized chunks from prior folds.
	for id, path := range in.ExistingChunks {
		chunkMap[id] = path
	}

	events := in.Events
	total := (len(events) + chunkSize - 1) / chunkSize
	if total == 0 {
		total = 1
	}

	for i := 0; i < len(events); i += chunkSize {
		end := i + chunkSize
		if end > len(events) {
			end = len(events)
		}
		chunk := events[i:end]
		chunkNum := (i / chunkSize) + 1

		offsets := make([]uint64, len(chunk))
		for j, e := range chunk {
			offsets[j] = e.Offset
		}
		cid := chunkID(in.ProjectID, offsets)

		// Skip synthesis if this chunk is already in the vault.
		if existingPath, seen := in.ExistingChunks[cid]; seen {
			if existingPath == "" {
				progress(fmt.Sprintf("L0 episode %d/%d (cached)", chunkNum, total))
				chunkMap[cid] = ""
				continue
			}
			if _, exists := acc[existingPath]; exists {
				progress(fmt.Sprintf("L0 episode %d/%d (cached)", chunkNum, total))
				chunkMap[cid] = existingPath
				continue
			}
		}

		progress(fmt.Sprintf("L0 episode %d/%d — synthesizing %d events", chunkNum, total, len(chunk)))

		// Only pass episodes/ dir as existing context for L0.
		episodeExisting := map[string]string{}
		for p, c := range acc {
			if strings.HasPrefix(p, "episodes/") {
				episodeExisting[p] = c
			}
		}

		chunkIn := FoldInput{
			ProjectID:   in.ProjectID,
			ProjectRoot: in.ProjectRoot,
			FromOffset:  chunk[0].Offset,
			ToOffset:    chunk[len(chunk)-1].Offset,
			Existing:    episodeExisting,
			Events:      chunk,
		}
		// Build episode prompt specifically for L0.
		res, err := hs.inner.Fold(ctx, buildL0Input(chunkIn, cid))
		if err != nil {
			warn(fmt.Sprintf("L0 chunk %d/%d failed (%v); skipping", (i/chunkSize)+1, total, err))
			continue
		}
		for p, c := range res.Files {
			acc[p] = c
		}
		// Record the episode path.
		episodePath := fmt.Sprintf("episodes/%s.md", cid)
		if _, ok := acc[episodePath]; ok {
			chunkMap[cid] = episodePath
		} else {
			chunkMap[cid] = ""
		}
	}

	// Collect L0 episode files.
	l0Files := map[string]string{}
	for p, c := range acc {
		if strings.HasPrefix(p, "episodes/") {
			l0Files[p] = c
		}
	}

	// ── L1 pass: topics + timeline synthesized from all L0 episodes ──────────
	if len(l0Files) >= hs.l1Threshold {
		progress(fmt.Sprintf("L1 synthesis — topics + timeline from %d episodes", len(l0Files)))
		l1Existing := map[string]string{}
		for p, c := range acc {
			if strings.HasPrefix(p, "topics/") || strings.HasPrefix(p, "timeline/") {
				l1Existing[p] = c
			}
		}
		l1In := buildL1Input(in, l0Files, l1Existing)
		l1Res, err := hs.inner.Fold(ctx, l1In)
		if err != nil {
			warn(fmt.Sprintf("L1 synthesis failed (%v); keeping existing L1 files", err))
		} else {
			for p, c := range l1Res.Files {
				acc[p] = c
			}
		}
	}

	// Collect L1 files.
	l1Files := map[string]string{}
	for p, c := range acc {
		if strings.HasPrefix(p, "topics/") || strings.HasPrefix(p, "timeline/") {
			l1Files[p] = c
		}
	}

	// ── L2 pass: summary synthesized from all L1 files ───────────────────────
	if len(l1Files) >= hs.l2Threshold {
		progress(fmt.Sprintf("L2 synthesis — overview from %d topics/timeline files", len(l1Files)))
		l2Existing := map[string]string{}
		for p, c := range acc {
			if strings.HasPrefix(p, "summaries/") {
				l2Existing[p] = c
			}
		}
		l2In := buildL2Input(in, l1Files, l2Existing)
		l2Res, err := hs.inner.Fold(ctx, l2In)
		if err != nil {
			warn(fmt.Sprintf("L2 synthesis failed (%v); keeping existing summaries", err))
		} else {
			for p, c := range l2Res.Files {
				acc[p] = c
			}
		}
	}

	// Remove internal hint keys injected during synthesis — never write to disk.
	for k := range acc {
		if strings.HasPrefix(k, "_") {
			delete(acc, k)
		}
	}

	linkRelated(acc)
	index := buildIndex(acc, in)
	block := buildClaudeBlock(in)
	return FoldResult{Files: acc, Index: index, ClaudeBlock: block, Chunks: chunkMap}, nil
}

// buildL0Input returns a FoldInput tailored for episode synthesis. It injects
// a special L0 prompt marker so the LLM knows to write an episode file.
func buildL0Input(in FoldInput, cid string) FoldInput {
	out := in
	// Inject a synthetic "context" file so the LLM knows its output path.
	hint := map[string]string{}
	hint["_l0_hint"] = fmt.Sprintf(
		"Write a SINGLE episode file at path episodes/%s.md.\n"+
			"Set frontmatter: level:0, kind:episode, sources:[<offsets from events>], tags:[<topic slugs>], time_range_start/end.\n"+
			"Body: bullet-point log of significant moments from this session chunk (decisions, corrections, preferences). No chatter.",
		cid)
	out.Existing = hint
	return out
}

// buildL1Input returns a FoldInput for L1 synthesis (topics + timeline from L0 episodes).
func buildL1Input(in FoldInput, l0Files, l1Existing map[string]string) FoldInput {
	// Merge l0 files into a synthetic "existing" set alongside the real L1 files.
	existing := map[string]string{}
	for p, c := range l1Existing {
		existing[p] = c
	}
	for p, c := range l0Files {
		existing[p] = c
	}

	// Collect all offsets and time range from the l0 files' frontmatter.
	var allOffsets []uint64
	var earliest, latest time.Time
	for _, content := range l0Files {
		fm, _ := ParseFrontmatter(content)
		allOffsets = append(allOffsets, fm.Sources...)
		if !fm.TimeRangeStart.IsZero() {
			if earliest.IsZero() || fm.TimeRangeStart.Before(earliest) {
				earliest = fm.TimeRangeStart
			}
		}
		if !fm.TimeRangeEnd.IsZero() {
			if latest.IsZero() || fm.TimeRangeEnd.After(latest) {
				latest = fm.TimeRangeEnd
			}
		}
	}
	sort.Slice(allOffsets, func(i, j int) bool { return allOffsets[i] < allOffsets[j] })

	// Build synthetic events list — one placeholder per l0 file.
	syntheticEvents := make([]FoldEvent, 0, len(l0Files))
	for p := range l0Files {
		syntheticEvents = append(syntheticEvents, FoldEvent{
			Type: "synthetic.l0",
			Text: fmt.Sprintf("Episode file: %s", p),
		})
	}

	hint := map[string]string{}
	hint["_l1_hint"] = "Synthesize topics/ and timeline/ files from the L0 episode files in the existing set.\n" +
		"Set frontmatter: level:1, kind:topic or kind:timeline, sources:[combined offsets], tags:[relevant slugs], time_range_start/end.\n" +
		"List children: paths of the episodes/ files that contributed.\n" +
		"Body: synthesized content — patterns, decisions, and chronological summaries. No inventing."
	for p, c := range existing {
		hint[p] = c
	}

	var fromOff, toOff uint64
	if len(allOffsets) > 0 {
		fromOff = allOffsets[0]
		toOff = allOffsets[len(allOffsets)-1]
	}

	return FoldInput{
		ProjectID:   in.ProjectID,
		ProjectRoot: in.ProjectRoot,
		FromOffset:  fromOff,
		ToOffset:    toOff,
		Existing:    hint,
		Events:      syntheticEvents,
	}
}

// buildL2Input returns a FoldInput for L2 synthesis (overview summary from L1 files).
func buildL2Input(in FoldInput, l1Files, l2Existing map[string]string) FoldInput {
	existing := map[string]string{}
	for p, c := range l2Existing {
		existing[p] = c
	}
	for p, c := range l1Files {
		existing[p] = c
	}

	var allOffsets []uint64
	var earliest, latest time.Time
	for _, content := range l1Files {
		fm, _ := ParseFrontmatter(content)
		allOffsets = append(allOffsets, fm.Sources...)
		if !fm.TimeRangeStart.IsZero() {
			if earliest.IsZero() || fm.TimeRangeStart.Before(earliest) {
				earliest = fm.TimeRangeStart
			}
		}
		if !fm.TimeRangeEnd.IsZero() {
			if latest.IsZero() || fm.TimeRangeEnd.After(latest) {
				latest = fm.TimeRangeEnd
			}
		}
	}
	sort.Slice(allOffsets, func(i, j int) bool { return allOffsets[i] < allOffsets[j] })

	syntheticEvents := make([]FoldEvent, 0, len(l1Files))
	for p := range l1Files {
		syntheticEvents = append(syntheticEvents, FoldEvent{
			Type: "synthetic.l1",
			Text: fmt.Sprintf("L1 file: %s", p),
		})
	}

	hint := map[string]string{}
	hint["_l2_hint"] = "Write a SINGLE high-level overview at path summaries/overview.md.\n" +
		"Set frontmatter: level:2, kind:summary, sources:[combined offsets], tags:[main themes].\n" +
		"List children: paths of the topics/ and timeline/ files that contributed.\n" +
		"Body: concise project overview — main themes, key decisions, active concerns. No inventing."
	for p, c := range existing {
		hint[p] = c
	}

	var fromOff, toOff uint64
	if len(allOffsets) > 0 {
		fromOff = allOffsets[0]
		toOff = allOffsets[len(allOffsets)-1]
	}

	return FoldInput{
		ProjectID:   in.ProjectID,
		ProjectRoot: in.ProjectRoot,
		FromOffset:  fromOff,
		ToOffset:    toOff,
		Existing:    hint,
		Events:      syntheticEvents,
	}
}
