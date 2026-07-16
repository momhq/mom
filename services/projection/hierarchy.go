package projection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	// minBisectEvents is the smallest event window bisection retries. A window
	// this small that still fails is not failing because of its size — stop the
	// fold there and let the next fold retry it.
	minBisectEvents = 10
	// maxKnownSubjectTags caps the tag vocabulary embedded in each L0 prompt.
	maxKnownSubjectTags = 60
)

// HierarchySynth wraps an LLMSynth to produce the ICM vault:
// L0 episodes (one per chunk) → L1 reference/contract concepts (one per
// subject, grouped by L0 tags) → L2 identity.md (from the concept layer).
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
//
// There is no non-LLM fallback anywhere in this path. When an L0 window keeps
// failing (after the synthesizer's retry and bisection down to
// minBisectEvents), the fold STOPS consuming events: the returned
// FoldedThrough is the end of the last consecutively successful window, the
// CLI persists that as the watermark, and the next fold re-reads the exact
// same event stream from there — so no event is ever skipped or handed to a
// dumber engine.
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
	checkpoint := in.Checkpoint
	if checkpoint == nil {
		checkpoint = func(map[string]string, map[string]string, uint64) {}
	}
	parallel := in.Parallel
	if parallel < 1 {
		parallel = 1
	}
	// Synthesis calls run concurrently; serialize the callbacks the CLI hands
	// us (spinner updates, stderr warnings) so they cannot interleave.
	var cbMu sync.Mutex
	baseProgress, baseWarn := progress, warn
	progress = func(s string) { cbMu.Lock(); baseProgress(s); cbMu.Unlock() }
	warn = func(s string) { cbMu.Lock(); baseWarn(s); cbMu.Unlock() }

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

	// foldedThrough is the offset the vault is consistent through: the end of
	// the last consecutive successfully folded chunk (the resume watermark).
	foldedThrough := in.ToOffset
	// systemicAbort is set when the harness itself is failing (usage limit,
	// auth) — every further call is doomed, so L1/L2 are skipped this fold.
	systemicAbort := false

	if in.ResumeSynthesis {
		// Resume path: L0 is already complete on disk (loaded into in.Existing).
		// Skip the L0 loop entirely; l0Files will be populated from acc below.
		// The watermark does not change — it was already persisted correctly.
		progress("resuming interrupted synthesis — skipping L0 (already complete)")
	} else if len(in.Events) > 0 {
		var stopped bool
		foldedThrough, systemicAbort, stopped = hs.runL0Pool(ctx, in, chunkSize, parallel, acc, chunkMap, progress, warn, checkpoint)
		if stopped {
			if systemicAbort {
				warn(fmt.Sprintf("harness failing systemically; aborting this fold at offset %d — run `mom vault fold` when the harness recovers (e.g. usage limit reset)", foldedThrough))
			} else {
				warn(fmt.Sprintf("some windows failed; watermark stopped at offset %d — run `mom vault fold` again to retry them (completed windows are cached)", foldedThrough))
			}
		}
	}

	// Collect L0 episode files.
	l0Files := map[string]string{}
	for p, c := range acc {
		if strings.HasPrefix(p, episodesDir+"/") {
			l0Files[p] = c
		}
	}

	// ── L1 pass: one reference/contract concept per SUBJECT ─────────────────
	// Subject-oriented, not batch-oriented. The L0 episodes already carry the
	// subjects in their `tags`; we group episodes by tag and synthesize exactly
	// ONE concept file per subject from only that subject's episodes. This
	// makes dedup structural (one file per subject — no near-duplicates), bounds
	// every call to a single small output (fast, never times out), and scales
	// with the number of subjects rather than the episode count.
	//
	// The pass is INCREMENTAL: a subject's concept file carries a
	// content-addressed id computed from its episodes' source offsets. When the
	// existing file's id already matches, the subject is untouched this fold and
	// the call is skipped — an incremental fold only pays for subjects whose
	// episode set actually changed.
	l1Changed := false
	if !systemicAbort && len(l0Files) >= hs.l1Threshold {
		subjects := collectSubjects(l0Files, in.ProjectID)
		progress(fmt.Sprintf("L1 synthesis — %d subjects from %d episodes", len(subjects), len(l0Files)))

		l1ctx, l1cancel := context.WithCancel(ctx)
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, parallel)

		for i, subj := range subjects {
			subEps := make(map[string]string, len(subj.episodePaths))
			var subOffsets []uint64
			for _, p := range subj.episodePaths {
				subEps[p] = l0Files[p]
				fm, _ := ParseFrontmatter(l0Files[p])
				subOffsets = append(subOffsets, fm.Sources...)
			}

			// The concept may live in reference/ or contracts/ — the model
			// classifies the subject. Check both for the skip and for
			// update-in-place pinning.
			refPath := referenceDir + "/" + subj.slug + ".md"
			conPath := contractsDir + "/" + subj.slug + ".md"
			mu.Lock()
			existingPath := ""
			for _, p := range []string{refPath, conPath} {
				if _, ok := acc[p]; ok {
					existingPath = p
					break
				}
			}
			expectID := chunkID(in.ProjectID, sortedUniqueOffsets(subOffsets))
			if existingPath != "" {
				if fm, _ := ParseFrontmatter(acc[existingPath]); fm.ID == expectID {
					mu.Unlock()
					progress(fmt.Sprintf("L1 subject %d/%d — %s (unchanged)", i+1, len(subjects), existingPath))
					continue
				}
			}
			existingContent := acc[existingPath]
			mu.Unlock()

			// Acquire a slot before spawning: parallel=1 stays strictly
			// sequential, and a systemic cancel stops further submissions.
			sem <- struct{}{}
			if l1ctx.Err() != nil {
				<-sem
				break
			}
			wg.Add(1)
			go func(i int, subj subject, subEps map[string]string, subOffsets []uint64, refPath, conPath, existingPath, existingContent string) {
				defer wg.Done()
				defer func() { <-sem }()
				progress(fmt.Sprintf("L1 subject %d/%d — %s (%d episodes)", i+1, len(subjects), subj.slug, len(subj.episodePaths)))
				l1Res, err := hs.inner.Fold(l1ctx, buildL1SubjectInput(in, subj, subEps, existingPath, existingContent))
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if IsSystemicError(err) && l1ctx.Err() == nil {
						warn(fmt.Sprintf("harness failing systemically (%v); aborting L1 — the next fold retries the remaining subjects", err))
					} else if l1ctx.Err() == nil {
						// Safe to skip: the id mismatch persists, so the next
						// fold retries this subject.
						warn(fmt.Sprintf("L1 subject %q failed (%v); skipping — next fold retries it", subj.slug, err))
					}
					if IsSystemicError(err) {
						systemicAbort = true
						l1cancel()
					}
					return
				}
				changed := map[string]string{}
				wrote := ""
				for p, c := range l1Res.Files {
					// Stamp provenance MOM owns: the union of the subject's
					// episode offsets, and the episodes as children. The model
					// omits sources.
					if strings.HasPrefix(p, referenceDir+"/") || strings.HasPrefix(p, contractsDir+"/") {
						c = stampProvenance(c, in.ProjectID, subOffsets, subj.episodePaths)
						wrote = p
						l1Changed = true
					}
					acc[p] = c
					changed[p] = c
				}
				// If the model reclassified the subject into the other folder,
				// drop the stale twin so one subject never has two homes.
				if wrote != "" {
					for _, p := range []string{refPath, conPath} {
						if p != wrote {
							delete(acc, p)
						}
					}
				}
				checkpoint(changed, chunkMap, foldedThrough)
			}(i, subj, subEps, subOffsets, refPath, conPath, existingPath, existingContent)
		}
		wg.Wait()
		l1cancel()
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

	// ── L2 pass: identity.md synthesized from the concept layer ─────────────
	// The concept layer is deduped and bounded by subject count (far smaller
	// than the episode corpus), so a single identity call is tractable. Skipped
	// when no concept changed this fold — identity depends only on that layer.
	if !systemicAbort && len(l1Files) >= hs.l2Threshold {
		_, hasIdentity := acc[identityFile]
		if !l1Changed && hasIdentity {
			progress("L2 identity — unchanged")
		} else {
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
				checkpoint(l2Res.Files, chunkMap, foldedThrough)
			}
		}
	}

	// Remove internal hint keys injected during synthesis — never write to disk.
	for k := range acc {
		if strings.HasPrefix(k, "_") {
			delete(acc, k)
		}
	}

	// pendingSynthesis is true when the L0 pass completed (or produced files)
	// but L1/L2 was aborted due to a systemic harness failure. This signals the
	// CLI to persist the flag so the next fold can resume L1/L2 without
	// re-synthesizing L0.
	pendingSynthesis := systemicAbort && len(l0Files) >= hs.l1Threshold

	// The INDEX and CLAUDE.md watermark must state what is actually folded,
	// which on a partial fold is behind the read head.
	idxIn := in
	idxIn.ToOffset = foldedThrough

	linkRelated(acc)
	buildPerFolderIndexes(acc)
	index := buildIndex(acc, idxIn)
	block := buildClaudeBlock(idxIn)
	return FoldResult{Files: acc, Index: index, ClaudeBlock: block, Chunks: chunkMap, FoldedThrough: foldedThrough, PendingSynthesis: pendingSynthesis}, nil
}

// runL0Pool synthesizes all L0 chunks with up to parallel concurrent calls.
// Chunk boundaries are deterministic (same watermark → same event stream →
// same chunks), so out-of-order completions are safe to persist: the
// watermark only advances along the CONSECUTIVE prefix of successful chunks,
// while later completed chunks land in the chunk cache and are skipped when
// the next fold re-reads the window. A systemic harness failure cancels the
// pool; an isolated window failure lets the other windows finish.
func (hs *HierarchySynth) runL0Pool(ctx context.Context, in FoldInput, chunkSize, parallel int, acc, chunkMap map[string]string, progress, warn func(string), checkpoint func(map[string]string, map[string]string, uint64)) (foldedThrough uint64, systemic, stopped bool) {
	events := in.Events
	total := (len(events) + chunkSize - 1) / chunkSize

	type result struct {
		endOff uint64
		ok     bool
		done   bool
	}
	results := make([]result, total)
	frontierIdx := 0
	frontier := in.FromOffset
	knownTags := knownSubjectTags(acc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallel)

	// advanceFrontier is called with mu held. The watermark moves through the
	// consecutive prefix of fully successful chunks, then may step INTO a
	// partially-bisected frontier chunk (its left halves succeeded).
	advanceFrontier := func() {
		for frontierIdx < total && results[frontierIdx].done && results[frontierIdx].ok {
			frontier = results[frontierIdx].endOff
			frontierIdx++
		}
		if frontierIdx < total && results[frontierIdx].done && !results[frontierIdx].ok && results[frontierIdx].endOff > frontier {
			frontier = results[frontierIdx].endOff
		}
	}

	for i := 0; i < len(events); i += chunkSize {
		idx := i / chunkSize
		end := i + chunkSize
		if end > len(events) {
			end = len(events)
		}
		chunk := events[i:end]

		offsets := make([]uint64, len(chunk))
		for j, e := range chunk {
			offsets[j] = e.Offset
		}
		cid := chunkID(in.ProjectID, offsets)

		// Skip synthesis if this chunk is already in the vault.
		if existingPath, seen := in.ExistingChunks[cid]; seen {
			mu.Lock()
			cachedOK := existingPath == ""
			if !cachedOK {
				_, cachedOK = acc[existingPath]
			}
			if cachedOK {
				chunkMap[cid] = existingPath
				results[idx] = result{endOff: chunk[len(chunk)-1].Offset, ok: true, done: true}
				advanceFrontier()
				mu.Unlock()
				progress(fmt.Sprintf("L0 episode %d/%d (cached)", idx+1, total))
				continue
			}
			mu.Unlock()
		}

		// Acquire a slot BEFORE spawning: parallel=1 stays strictly ordered,
		// and a systemic cancel stops further submissions.
		sem <- struct{}{}
		if ctx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func(idx int, chunk []FoldEvent) {
			defer wg.Done()
			defer func() { <-sem }()
			label := fmt.Sprintf("L0 episode %d/%d", idx+1, total)
			progress(fmt.Sprintf("%s — synthesizing %d events", label, len(chunk)))

			files := map[string]string{}
			cmap := map[string]string{}
			endOff, err := hs.foldL0Chunk(ctx, in, chunk, files, cmap, label, progress, warn, knownTags)

			mu.Lock()
			defer mu.Unlock()
			for p, c := range files {
				acc[p] = c
			}
			for k, v := range cmap {
				chunkMap[k] = v
			}
			results[idx] = result{endOff: endOff, ok: err == nil, done: true}
			advanceFrontier()
			if err != nil {
				stopped = true
				if IsSystemicError(err) {
					systemic = true
					cancel()
				} else if ctx.Err() == nil {
					warn(fmt.Sprintf("%s failed (%v); continuing other windows — the next fold retries it", label, err))
				}
			}
			if len(files) > 0 {
				checkpoint(files, chunkMap, frontier)
			}
		}(idx, chunk)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !stopped {
		frontier = in.ToOffset
	}
	return frontier, systemic, stopped
}

// foldL0Chunk synthesizes one episode for a window of events, bisecting on
// size-related failure (timeout, truncated/malformed output) so one bad
// window cannot stall the whole fold. A systemic harness failure (usage
// limit, auth) is returned immediately without bisecting — halving the window
// cannot fix the harness. Successful output is written into files and cmap
// (caller-local maps; the pool merges them under its lock). It returns the
// offset folded through — the end of the window's consecutive successful
// prefix, 0 when nothing folded — and the failure, nil on full success.
func (hs *HierarchySynth) foldL0Chunk(ctx context.Context, in FoldInput, chunk []FoldEvent, files, cmap map[string]string, label string, progress, warn func(string), knownTags []string) (uint64, error) {
	offsets := make([]uint64, len(chunk))
	for j, e := range chunk {
		offsets[j] = e.Offset
	}
	cid := chunkID(in.ProjectID, offsets)

	chunkIn := FoldInput{
		ProjectID:   in.ProjectID,
		ProjectRoot: in.ProjectRoot,
		FromOffset:  chunk[0].Offset,
		ToOffset:    chunk[len(chunk)-1].Offset,
		Events:      chunk,
	}
	res, err := hs.inner.Fold(ctx, buildL0Input(chunkIn, cid, knownTags))
	if err == nil {
		for p, c := range res.Files {
			// MOM owns provenance — the model omits sources (a big offset array
			// bloats output and truncates the JSON). Stamp the chunk's offsets.
			if strings.HasPrefix(p, episodesDir+"/") {
				c = stampProvenance(c, in.ProjectID, offsets, nil)
			}
			files[p] = c
		}
		episodePath := fmt.Sprintf("%s/%s.md", episodesDir, cid)
		if _, ok := files[episodePath]; ok {
			cmap[cid] = episodePath
		} else {
			cmap[cid] = ""
		}
		return chunk[len(chunk)-1].Offset, nil
	}

	if IsSystemicError(err) {
		return 0, err
	}
	if len(chunk) < 2*minBisectEvents {
		warn(fmt.Sprintf("%s failed (%v); window of %d events is too small to bisect", label, err, len(chunk)))
		return 0, err
	}

	mid := len(chunk) / 2
	warn(fmt.Sprintf("%s failed (%v); bisecting into %d+%d events", label, err, mid, len(chunk)-mid))
	progress(label + " — first half")
	leftEnd, lerr := hs.foldL0Chunk(ctx, in, chunk[:mid], files, cmap, label+"·a", progress, warn, knownTags)
	if lerr != nil {
		return leftEnd, lerr
	}
	progress(label + " — second half")
	rightEnd, rerr := hs.foldL0Chunk(ctx, in, chunk[mid:], files, cmap, label+"·b", progress, warn, knownTags)
	if rightEnd == 0 {
		rightEnd = leftEnd
	}
	return rightEnd, rerr
}

// knownSubjectTags returns the tag vocabulary already used by episodes in the
// vault, most frequent first, so new episodes converge on existing subject
// slugs instead of inventing near-duplicates.
func knownSubjectTags(acc map[string]string) []string {
	count := map[string]int{}
	for p, c := range acc {
		if !strings.HasPrefix(p, episodesDir+"/") {
			continue
		}
		fm, _ := ParseFrontmatter(c)
		for _, t := range fm.Tags {
			if slug := tagSlug(t); slug != "" {
				count[slug]++
			}
		}
	}
	tags := make([]string, 0, len(count))
	for t := range count {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool {
		if count[tags[i]] != count[tags[j]] {
			return count[tags[i]] > count[tags[j]]
		}
		return tags[i] < tags[j]
	})
	if len(tags) > maxKnownSubjectTags {
		tags = tags[:maxKnownSubjectTags]
	}
	return tags
}

// buildL0Input returns a FoldInput tailored for episode synthesis. It injects
// a special L0 prompt marker so the LLM knows to write an episode file.
func buildL0Input(in FoldInput, cid string, knownTags []string) FoldInput {
	out := in
	// Inject a synthetic "context" file so the LLM knows its output path.
	hint := fmt.Sprintf(
		"WORK ITEM (L0 capture): Write a SINGLE episode file at path episodes/%s.md.\n"+
			"Frontmatter: type:episode, name (<=8 words), description (<=1 sentence), level:0, tags:[2-4 kebab-case subject slugs], time_range_start/end. OMIT sources — do NOT write a sources field (MOM fills provenance).\n"+
			"Body: a FLAT bullet list — AT MOST 10 short bullets of durable decisions/corrections/preferences and what was built. "+
			"HARD LIMITS: no headings, no code blocks, no sub-bullets, no multi-line bullets; keep the whole file under 180 words. "+
			"This must stay short enough to fit in one response — terseness is mandatory, not optional.",
		cid)
	if len(knownTags) > 0 {
		hint += "\nKNOWN SUBJECT TAGS (reuse one of these when it fits instead of coining a synonym): " + strings.Join(knownTags, ", ")
	}
	out.Existing = map[string]string{"_l0_hint": hint}
	return out
}

// stampProvenance rewrites a synthesized file's machine-owned frontmatter:
// sources (deduped/sorted), children, and the content-addressed id. The model
// is told to OMIT these — a large sources offset array bloats the JSON output
// until it truncates, and provenance is MOM's to compute, not the model's.
func stampProvenance(content, projectID string, sources []uint64, children []string) string {
	fm, body := ParseFrontmatter(content)
	fm.Sources = sortedUniqueOffsets(sources)
	if len(children) > 0 {
		ch := append([]string(nil), children...)
		sort.Strings(ch)
		fm.Children = ch
	}
	fm.ID = chunkID(projectID, fm.Sources)
	return PrependFrontmatter(fm, body)
}

func sortedUniqueOffsets(in []uint64) []uint64 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[uint64]struct{}, len(in))
	out := make([]uint64, 0, len(in))
	for _, o := range in {
		if _, ok := seen[o]; ok {
			continue
		}
		seen[o] = struct{}{}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// subject is one concept to synthesize: a tag slug shared by a set of L0
// episodes.
type subject struct {
	slug         string
	name         string
	episodePaths []string
}

// collectSubjects groups L0 episodes by the tags their frontmatter carries. Each
// tag that recurs across at least l1SubjectMinEpisodes episodes becomes one
// concept. The project-name tag and one-off tags are dropped as noise; the
// result is capped at maxSubjects by frequency, then ordered by slug for a
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

// buildL1SubjectInput returns a FoldInput that asks for a SINGLE concept file
// about one subject, synthesized from only that subject's episodes. The model
// classifies the subject as reference (durable facts/decisions/conventions) or
// contract (recurring process/workflow rules) — unless a concept file already
// exists, in which case it must update that exact path in place.
func buildL1SubjectInput(in FoldInput, subj subject, episodes map[string]string, existingPath, existingContent string) FoldInput {
	var offsets []uint64
	for _, c := range episodes {
		fm, _ := ParseFrontmatter(c)
		offsets = append(offsets, fm.Sources...)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })

	var target string
	if existingPath != "" {
		target = fmt.Sprintf(
			"A concept file for this subject already exists at %s — UPDATE it IN PLACE at that EXACT path, keeping its type. Do NOT create a file at any other path.",
			existingPath)
	} else {
		target = fmt.Sprintf(
			"Classify the subject, then pick the path:\n"+
				"- contracts/%s.md with type:contract — if the subject is a recurring PROCESS or set of workflow rules (how a kind of work is done: releases, reviews, testing, branching).\n"+
				"- reference/%s.md with type:reference — otherwise (durable decisions, conventions, architecture, facts).",
			subj.slug, subj.slug)
	}

	hint := map[string]string{}
	hint["_l1_hint"] = fmt.Sprintf(
		"WORK ITEM (L1 subject synthesis): Write EXACTLY ONE file about the subject \"%s\".\n%s\n"+
			"Synthesize ONLY what the episode files below say about this subject: durable decisions, conventions, current state, gotchas. Ignore details unrelated to \"%s\".\n"+
			"Frontmatter: type (as above), name:\"%s\", description:<one line>, level:1, tags:[%s]. OMIT sources — do NOT write a sources field (MOM fills provenance).\n"+
			"Body: a FLAT bullet list, AT MOST 12 short bullets, no headings, no code blocks, no sub-bullets, under 200 words. Terse. Do NOT write any other file.",
		subj.name, target, subj.name, subj.name, subj.slug)
	if existingPath != "" && existingContent != "" {
		hint[existingPath] = existingContent
	}
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

// buildL2Input returns a FoldInput for L2 synthesis (identity from L1 files).
func buildL2Input(in FoldInput, l1Files, l2Existing map[string]string) FoldInput {
	existing := map[string]string{}
	for p, c := range l2Existing {
		existing[p] = c
	}
	for p, c := range l1Files {
		existing[p] = c
	}

	var allOffsets []uint64
	for _, content := range l1Files {
		fm, _ := ParseFrontmatter(content)
		allOffsets = append(allOffsets, fm.Sources...)
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
		"Frontmatter: type:identity, name, description, level:2, tags. OMIT sources — do NOT write a sources field (MOM fills provenance). List children: the reference/ paths it draws on.\n" +
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
