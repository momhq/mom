package projection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// l1BatchEpisodes is how many L0 episodes one L1 synthesis call processes.
// Bounded so neither the prompt nor the expected JSON output grows with the
// whole corpus (which made a single all-episodes call time out or truncate).
const l1BatchEpisodes = 20

// HierarchySynth wraps an LLMSynth to produce the ICM vault:
// L0 episodes (one per chunk) → L1 reference/ + contracts/ concepts
// (synthesized from L0 in batches) → L2 identity.md (from the reference layer).
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

	// ── L1 pass: reference/ + contracts/ synthesized from L0 episodes ────────
	// Batched: each call sees a BATCH of episodes plus the reference/contracts
	// built so far, and updates concepts in place. A single call over all
	// episodes does not scale — the prompt and the expected JSON output grow
	// with the corpus until the model times out or truncates and the synth
	// silently falls back to the deterministic engine.
	if len(l0Files) >= hs.l1Threshold {
		epPaths := make([]string, 0, len(l0Files))
		for p := range l0Files {
			epPaths = append(epPaths, p)
		}
		sort.Strings(epPaths)

		batch := l1BatchEpisodes
		nBatches := (len(epPaths) + batch - 1) / batch
		for bi := 0; bi < len(epPaths); bi += batch {
			end := bi + batch
			if end > len(epPaths) {
				end = len(epPaths)
			}
			progress(fmt.Sprintf("L1 synthesis — reference/contracts (batch %d/%d, %d episodes)", bi/batch+1, nBatches, end-bi))

			batchFiles := map[string]string{}
			for _, p := range epPaths[bi:end] {
				batchFiles[p] = l0Files[p]
			}
			// Feed back the reference/contracts accumulated so far so the model
			// UPDATES an existing concept rather than spawning a duplicate.
			l1Existing := map[string]string{}
			for p, c := range acc {
				if strings.HasSuffix(p, "/"+indexFileName) {
					continue
				}
				if strings.HasPrefix(p, referenceDir+"/") || strings.HasPrefix(p, contractsDir+"/") {
					l1Existing[p] = c
				}
			}
			l1Res, err := hs.inner.Fold(ctx, buildL1Input(in, batchFiles, l1Existing))
			if err != nil {
				warn(fmt.Sprintf("L1 batch %d/%d failed (%v); skipping", bi/batch+1, nBatches, err))
				continue
			}
			for p, c := range l1Res.Files {
				acc[p] = c
			}
		}
	}

	// Collect L1 concept files (reference + contracts), excluding folder indexes.
	l1Files := map[string]string{}
	for p, c := range acc {
		if strings.HasSuffix(p, "/"+indexFileName) {
			continue
		}
		if strings.HasPrefix(p, referenceDir+"/") || strings.HasPrefix(p, contractsDir+"/") {
			l1Files[p] = c
		}
	}

	// ── L2 pass: identity.md synthesized from the reference/contract layer ───
	// The reference layer is deduped and bounded by subject count (far smaller
	// than the episode corpus), so a single identity call is tractable.
	if len(l1Files) >= hs.l2Threshold {
		progress(fmt.Sprintf("L2 synthesis — identity from %d reference/contract files", len(l1Files)))
		l2Existing := map[string]string{}
		if c, ok := acc[identityFile]; ok {
			l2Existing[identityFile] = c
		}
		l2Res, err := hs.inner.Fold(ctx, buildL2Input(in, l1Files, l2Existing))
		if err != nil {
			warn(fmt.Sprintf("L2 synthesis failed (%v); keeping existing identity", err))
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
	buildPerFolderIndexes(acc)
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
		"WORK ITEM (L0 capture): Write a SINGLE episode file at path episodes/%s.md.\n"+
			"Frontmatter: type:episode, name (<=8 words), description (<=1 sentence), level:0, sources:[<offsets from events>], tags:[<subject slugs>], time_range_start/end.\n"+
			"Body: a FLAT bullet list — AT MOST 10 short bullets of durable decisions/corrections/preferences and what was built. "+
			"HARD LIMITS: no headings, no code blocks, no sub-bullets, no multi-line bullets; keep the whole file under 180 words. "+
			"This must stay short enough to fit in one response — terseness is mandatory, not optional.",
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
	hint["_l1_hint"] = "WORK ITEM (L1 synthesis): From the L0 episode files in the existing set, synthesize CANONICAL concept files.\n" +
		"- reference/<subject>.md — one file per durable SUBJECT (a decision, convention, architecture area, or fact). type:reference.\n" +
		"- contracts/<subject>.md — one file per recurring PROCESS or rule (workflow, release flow, review, naming). type:contract.\n" +
		"MINIMALISM: one subject per file. If reference/<subject>.md already exists, UPDATE it — never create a near-duplicate (no -v2/-view/-cleanup variants); merge instead.\n" +
		"Frontmatter: type, name (short title), description (one line), level:1, sources:[combined offsets], tags, time_range_start/end. List children: the episodes/ paths that contributed.\n" +
		"Body: synthesized decisions/patterns/conventions. No inventing."
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
	hint["_l2_hint"] = "WORK ITEM (L2 synthesis): From the L1 reference/contract files in the existing set, write a SINGLE file identity.md (type:identity).\n" +
		"It states what THIS project IS right now: purpose, what it does, current architecture/direction, active concerns. A LIVING orientation, NOT a chronological recap — no dates, no history log. UPDATE the existing identity.md in place.\n" +
		"Frontmatter: type:identity, name, description, level:2, sources:[combined offsets], tags. List children: the reference/ paths it draws on.\n" +
		"Body: synthesized from the existing files. No inventing."
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
