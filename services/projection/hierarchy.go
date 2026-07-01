package projection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// l1SubjectMinEpisodes is the minimum number of episodes a tag must span to
	// become a reference concept — filters one-off tags out as noise.
	l1SubjectMinEpisodes = 2
	// maxSubjects caps how many reference concepts one fold synthesizes.
	maxSubjects = 60
	// maxEpisodesPerSubject bounds a single subject's synthesis prompt so a very
	// hot tag can't produce an oversized call.
	maxEpisodesPerSubject = 40
)

// HierarchySynth wraps an LLMSynth to produce the ICM vault:
// L0 episodes (one per chunk) → L1 reference/ concepts (one per subject, grouped
// by L0 tags) → L2 identity.md (from the reference layer).
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

	// ── L1 pass: one reference/ concept per SUBJECT ──────────────────────────
	// Subject-oriented, not batch-oriented. The L0 episodes already carry the
	// subjects in their `tags`; we group episodes by tag and synthesize exactly
	// ONE reference file per subject from only that subject's episodes. This
	// makes dedup structural (one file per subject — no near-duplicates), bounds
	// every call to a single small output (fast, never times out), and scales
	// with the number of subjects rather than the episode count. A single call
	// over all episodes emitting all concepts did neither.
	if len(l0Files) >= hs.l1Threshold {
		subjects := collectSubjects(l0Files, in.ProjectID)
		progress(fmt.Sprintf("L1 synthesis — %d subjects from %d episodes", len(subjects), len(l0Files)))
		for i, subj := range subjects {
			progress(fmt.Sprintf("L1 subject %d/%d — reference/%s (%d episodes)", i+1, len(subjects), subj.slug, len(subj.episodePaths)))
			subEps := make(map[string]string, len(subj.episodePaths))
			for _, p := range subj.episodePaths {
				subEps[p] = l0Files[p]
			}
			l1Res, err := hs.inner.Fold(ctx, buildL1SubjectInput(in, subj, subEps))
			if err != nil {
				warn(fmt.Sprintf("L1 subject %q failed (%v); skipping", subj.slug, err))
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

// subject is one reference concept to synthesize: a tag slug shared by a set of
// L0 episodes.
type subject struct {
	slug         string
	name         string
	episodePaths []string
}

// collectSubjects groups L0 episodes by the tags their frontmatter carries. Each
// tag that recurs across at least l1SubjectMinEpisodes episodes becomes one
// reference concept. The project-name tag and one-off tags are dropped as noise;
// the result is capped at maxSubjects by frequency, then ordered by slug for a
// deterministic fold.
func collectSubjects(l0Files map[string]string, projectID string) []subject {
	tagEps := map[string][]string{}
	for p, c := range l0Files {
		fm, _ := ParseFrontmatter(c)
		seen := map[string]bool{}
		for _, t := range fm.Tags {
			slug := tagSlug(t)
			if slug == "" || seen[slug] {
				continue
			}
			seen[slug] = true
			tagEps[slug] = append(tagEps[slug], p)
		}
	}

	projSlug := tagSlug(projectID)
	subs := make([]subject, 0, len(tagEps))
	for slug, eps := range tagEps {
		if slug == projSlug || len(eps) < l1SubjectMinEpisodes {
			continue
		}
		// Bound the episodes per subject so a very hot tag can't produce an
		// oversized prompt; keep the most recent (highest-offset) episodes.
		sort.Strings(eps)
		if len(eps) > maxEpisodesPerSubject {
			eps = eps[len(eps)-maxEpisodesPerSubject:]
		}
		subs = append(subs, subject{slug: slug, name: strings.ReplaceAll(slug, "-", " "), episodePaths: eps})
	}

	// Keep the top maxSubjects by episode count, then order by slug.
	sort.Slice(subs, func(i, j int) bool {
		if len(subs[i].episodePaths) != len(subs[j].episodePaths) {
			return len(subs[i].episodePaths) > len(subs[j].episodePaths)
		}
		return subs[i].slug < subs[j].slug
	})
	if len(subs) > maxSubjects {
		subs = subs[:maxSubjects]
	}
	sort.Slice(subs, func(i, j int) bool { return subs[i].slug < subs[j].slug })
	return subs
}

// buildL1SubjectInput returns a FoldInput that asks for a SINGLE reference file
// about one subject, synthesized from only that subject's episodes.
func buildL1SubjectInput(in FoldInput, subj subject, episodes map[string]string) FoldInput {
	var offsets []uint64
	var earliest, latest time.Time
	for _, c := range episodes {
		fm, _ := ParseFrontmatter(c)
		offsets = append(offsets, fm.Sources...)
		if !fm.TimeRangeStart.IsZero() && (earliest.IsZero() || fm.TimeRangeStart.Before(earliest)) {
			earliest = fm.TimeRangeStart
		}
		if !fm.TimeRangeEnd.IsZero() && (latest.IsZero() || fm.TimeRangeEnd.After(latest)) {
			latest = fm.TimeRangeEnd
		}
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })

	hint := map[string]string{}
	hint["_l1_hint"] = fmt.Sprintf(
		"WORK ITEM (L1 subject synthesis): Write EXACTLY ONE file at reference/%s.md about the subject \"%s\".\n"+
			"Synthesize ONLY what the episode files below say about this subject: durable decisions, conventions, current state, gotchas. Ignore details unrelated to \"%s\".\n"+
			"Frontmatter: type:reference, name:\"%s\", description:<one line>, level:1, sources (the ledger offsets of the contributing episodes), tags:[%s].\n"+
			"Body: a FLAT bullet list, AT MOST 12 short bullets, no headings, no code blocks, no sub-bullets, under 200 words. Terse. Do NOT write any other file.",
		subj.slug, subj.name, subj.name, subj.name, subj.slug)
	for p, c := range episodes {
		hint[p] = c
	}

	var fromOff, toOff uint64
	if len(offsets) > 0 {
		fromOff, toOff = offsets[0], offsets[len(offsets)-1]
	}
	return FoldInput{
		ProjectID:   in.ProjectID,
		ProjectRoot: in.ProjectRoot,
		FromOffset:  fromOff,
		ToOffset:    toOff,
		Existing:    hint,
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
