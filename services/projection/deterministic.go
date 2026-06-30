package projection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	snippetMaxChars = 400
	maxTopicFiles   = 12
)

// DeterministicSynth is the no-LLM fallback synthesizer. It groups events
// into a small fixed structure (timeline/, topics/) and emits templated
// markdown. Fast and free; always succeeds.
type DeterministicSynth struct{}

// NewDeterministicSynth builds the templated synthesizer.
func NewDeterministicSynth() *DeterministicSynth { return &DeterministicSynth{} }

// Fold implements Synthesizer. It merges the new events with the existing
// vault by regenerating from the combined view it can derive; because the
// deterministic synth has no memory of prior raw events, it folds the new
// window and preserves any existing files it does not overwrite.
func (s *DeterministicSynth) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	files := map[string]string{}
	// Preserve existing files we don't regenerate this pass.
	for p, c := range in.Existing {
		files[p] = c
	}

	timeline := buildTimeline(in.ProjectID, in.Events)
	for p, c := range timeline {
		files[p] = c
	}
	topics := buildTopics(in.ProjectID, in.Events)
	for p, c := range topics {
		files[p] = c
	}

	// INDEX is regenerated each pass from the full file set.
	linkRelated(files)
	index := buildIndex(files, in)
	// INDEX.md is carried separately; don't leave a stray copy in Files.
	delete(files, indexFileName)

	block := buildClaudeBlock(in)

	return FoldResult{Files: files, Index: index, ClaudeBlock: block}, nil
}

// deterministicFrontmatter builds a Frontmatter for a deterministic vault
// file from the events that contributed to it.
func deterministicFrontmatter(projectID, kind string, events []FoldEvent) Frontmatter {
	offsets := make([]uint64, 0, len(events))
	tagSet := map[string]struct{}{}
	var earliest, latest time.Time
	for _, e := range events {
		offsets = append(offsets, e.Offset)
		for _, t := range e.Tags {
			tagSet[t] = struct{}{}
		}
		if earliest.IsZero() || e.CreatedAt.Before(earliest) {
			earliest = e.CreatedAt
		}
		if latest.IsZero() || e.CreatedAt.After(latest) {
			latest = e.CreatedAt
		}
	}
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	return Frontmatter{
		ID:             chunkID(projectID, offsets),
		Level:          1,
		Kind:           kind,
		Sources:        offsets,
		Tags:           tags,
		TimeRangeStart: earliest,
		TimeRangeEnd:   latest,
		FoldedAt:       time.Now().UTC(),
		Version:        1,
	}
}

// buildTimeline produces timeline/<YYYY-MM>.md, one bullet per
// memory.recorded and per detected cluster of turns, newest first.
func buildTimeline(projectID string, events []FoldEvent) map[string]string {
	type bullet struct {
		when time.Time
		line string
	}
	type monthBucket struct {
		bullets []bullet
		events  []FoldEvent
	}
	byMonth := map[string]*monthBucket{}

	// Cluster consecutive turns within the same session into one bullet.
	var turnRun []FoldEvent
	flush := func() {
		if len(turnRun) == 0 {
			return
		}
		first := turnRun[0]
		month := first.CreatedAt.UTC().Format("2006-01")
		summary := clusterSummary(turnRun)
		line := fmt.Sprintf("- **%s** · session `%s` · %d turns — %s",
			first.CreatedAt.UTC().Format("2006-01-02 15:04"),
			shortSession(first.SessionID), len(turnRun), summary)
		if byMonth[month] == nil {
			byMonth[month] = &monthBucket{}
		}
		byMonth[month].bullets = append(byMonth[month].bullets, bullet{when: first.CreatedAt, line: line})
		byMonth[month].events = append(byMonth[month].events, turnRun...)
		turnRun = nil
	}

	for _, e := range events {
		if e.Type == string(memoryType) {
			month := e.CreatedAt.UTC().Format("2006-01")
			text := e.Summary
			if strings.TrimSpace(text) == "" {
				text = e.Text
			}
			line := fmt.Sprintf("- **%s** · 🧠 memory · session `%s` — %s",
				e.CreatedAt.UTC().Format("2006-01-02 15:04"),
				shortSession(e.SessionID), truncate(text, snippetMaxChars))
			if len(e.Tags) > 0 {
				line += "  _(" + strings.Join(e.Tags, ", ") + ")_"
			}
			if byMonth[month] == nil {
				byMonth[month] = &monthBucket{}
			}
			byMonth[month].bullets = append(byMonth[month].bullets, bullet{when: e.CreatedAt, line: line})
			byMonth[month].events = append(byMonth[month].events, e)
			continue
		}
		// turn
		if len(turnRun) > 0 && turnRun[len(turnRun)-1].SessionID != e.SessionID {
			flush()
		}
		turnRun = append(turnRun, e)
	}
	flush()

	out := map[string]string{}
	for month, bucket := range byMonth {
		sort.SliceStable(bucket.bullets, func(i, j int) bool { return bucket.bullets[i].when.After(bucket.bullets[j].when) })
		var body strings.Builder
		fmt.Fprintf(&body, "# Timeline — %s\n\n", month)
		body.WriteString("_Episodic log of what happened, newest first._\n\n")
		for _, bl := range bucket.bullets {
			body.WriteString(bl.line)
			body.WriteString("\n")
		}
		fm := deterministicFrontmatter(projectID, "timeline", bucket.events)
		out[fmt.Sprintf("timeline/%s.md", month)] = PrependFrontmatter(fm, body.String())
	}
	return out
}

// buildTopics groups memory content by tag into topics/<tag>.md, capped
// at the most-frequent maxTopicFiles tags.
func buildTopics(projectID string, events []FoldEvent) map[string]string {
	type snippet struct {
		when  time.Time
		text  string
		sess  string
		event FoldEvent
	}
	byTag := map[string][]snippet{}
	count := map[string]int{}

	for _, e := range events {
		if e.Type != string(memoryType) {
			continue
		}
		text := e.Text
		if strings.TrimSpace(text) == "" {
			text = e.Summary
		}
		tags := e.Tags
		if len(tags) == 0 {
			tags = []string{"untagged"}
		}
		for _, t := range tags {
			slug := tagSlug(t)
			if slug == "" {
				continue
			}
			byTag[slug] = append(byTag[slug], snippet{when: e.CreatedAt, text: text, sess: e.SessionID, event: e})
			count[slug]++
		}
	}

	// Rank tags by frequency and cap.
	slugs := make([]string, 0, len(byTag))
	for s := range byTag {
		slugs = append(slugs, s)
	}
	sort.Slice(slugs, func(i, j int) bool {
		if count[slugs[i]] != count[slugs[j]] {
			return count[slugs[i]] > count[slugs[j]]
		}
		return slugs[i] < slugs[j]
	})
	if len(slugs) > maxTopicFiles {
		slugs = slugs[:maxTopicFiles]
	}

	out := map[string]string{}
	for _, slug := range slugs {
		snips := byTag[slug]
		sort.SliceStable(snips, func(i, j int) bool { return snips[i].when.After(snips[j].when) })

		bucketEvents := make([]FoldEvent, len(snips))
		for i, sn := range snips {
			bucketEvents[i] = sn.event
		}

		var body strings.Builder
		fmt.Fprintf(&body, "# Topic: %s\n\n", slug)
		body.WriteString("_Memories tagged with this topic, newest first. These point at immutable captures — do not rewrite them._\n\n")
		for _, sn := range snips {
			fmt.Fprintf(&body, "- **%s** (session `%s`): %s\n",
				sn.when.UTC().Format("2006-01-02"), shortSession(sn.sess), truncate(sn.text, snippetMaxChars))
		}
		fm := deterministicFrontmatter(projectID, "topic", bucketEvents)
		fm.Tags = []string{slug}
		out[fmt.Sprintf("topics/%s.md", slug)] = PrependFrontmatter(fm, body.String())
	}
	return out
}

// buildIndex emits the router INDEX.md mapping "read X when Y" to the
// files that exist, plus the watermark line.
func buildIndex(files map[string]string, in FoldInput) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		if p == indexFileName {
			continue
		}
		if !shouldIndexPath(p) {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var b strings.Builder
	fmt.Fprintf(&b, "# MOM Vault Index — %s\n\n", in.ProjectID)
	b.WriteString("Router for this project's projected memory. **Read the matching file before acting.** ")
	b.WriteString("These files synthesize and POINT AT immutable captures — never rewrite memories here. ")
	b.WriteString("Each row links straight to the file — open the link, don't reconstruct the path.\n\n")
	b.WriteString("| Read this | What it covers |\n")
	b.WriteString("|---|---|\n")
	for _, p := range paths {
		b.WriteString("| " + indexLink(p) + " | " + routerHint(p, files[p]) + " |\n")
	}
	if len(paths) == 0 {
		// No synthesized L1/L2 files yet. Route to L0 episodes directly so the
		// router never goes blind while a young vault is below the L1 threshold
		// — otherwise it reports "no files yet" while real memory sits in
		// episodes/, and the agent concludes the project has no history.
		rows := episodeIndexRows(files)
		if len(rows) == 0 {
			b.WriteString("| _(no files yet)_ | run `mom vault fold` after capturing some sessions |\n")
		}
		for _, r := range rows {
			b.WriteString("| " + indexLink(r.path) + " | " + r.hint + " |\n")
		}
	}
	b.WriteString("\n---\n")
	engine := in.Engine
	if engine == "" {
		engine = "deterministic"
	}
	fmt.Fprintf(&b, "_Watermark: folded through Ledger offset **%d** at %s (%s engine)._\n",
		in.ToOffset, time.Now().UTC().Format(time.RFC3339), engine)
	return b.String()
}

// indexLink renders a vault-relative path as a clickable markdown link whose
// text is the path itself. INDEX.md sits at the vault root, so a relative
// target like `topics/x.md` resolves directly — the agent opens the link
// instead of rebuilding the path from scratch.
func indexLink(path string) string {
	return "[`" + path + "`](" + path + ")"
}

// routerHint describes what a vault file covers, drawn from the file's own
// synthesized title when available so the router carries real content (e.g.
// "Harness MCP removal → vault-first context") rather than echoing the slug.
// content may be empty; the description then falls back to the path.
func routerHint(path, content string) string {
	switch {
	case strings.HasPrefix(path, "timeline/"):
		return "Chronological history — " + strings.TrimSuffix(strings.TrimPrefix(path, "timeline/"), ".md")
	case strings.HasPrefix(path, "topics/"):
		if t := docTitle(content); t != "" {
			return t
		}
		return strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(path, "topics/"), ".md"), "-", " ")
	case strings.HasPrefix(path, "summaries/"):
		if t := docTitle(content); t != "" {
			return t
		}
		return "High-level project overview"
	default:
		if t := docTitle(content); t != "" {
			return t
		}
		return "Relevant context"
	}
}

// docTitle extracts the first markdown H1 from a vault file body, stripped of
// the "Topic:"/"Summary:"/"Timeline:" prefix the synthesizers prepend. Returns
// "" when the body has no leading H1 (e.g. deterministic stubs handled by the
// caller's slug fallback).
func docTitle(content string) string {
	if content == "" {
		return ""
	}
	_, body := ParseFrontmatter(content)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			return "" // body opened without an H1 title
		}
		title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
		for _, prefix := range []string{"Topic:", "Summary:", "Timeline:"} {
			title = strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
		return title
	}
	return ""
}

func shouldIndexPath(path string) bool {
	return !strings.HasPrefix(path, "episodes/")
}

// indexRow is one router entry: the vault-relative path and its "when to read"
// hint.
type indexRow struct {
	path string
	hint string
}

// episodeIndexRows returns router rows for the L0 episode files, newest first.
// Used only as the router fallback when no L1/L2 files exist yet, so the agent
// is pointed at the raw episodes instead of being told there is nothing.
func episodeIndexRows(files map[string]string) []indexRow {
	type ep struct {
		path       string
		start, end time.Time
	}
	var eps []ep
	for p, content := range files {
		if !strings.HasPrefix(p, "episodes/") {
			continue
		}
		fm, _ := ParseFrontmatter(content)
		eps = append(eps, ep{path: p, start: fm.TimeRangeStart, end: fm.TimeRangeEnd})
	}
	// Newest first by end time; ties (incl. missing timestamps) broken by path
	// so the order is stable across folds.
	sort.Slice(eps, func(i, j int) bool {
		if eps[i].end.Equal(eps[j].end) {
			return eps[i].path < eps[j].path
		}
		return eps[i].end.After(eps[j].end)
	})
	rows := make([]indexRow, 0, len(eps))
	for _, e := range eps {
		rows = append(rows, indexRow{path: e.path, hint: episodeHint(e.start, e.end)})
	}
	return rows
}

// episodeHint describes an episode by its captured date range so the router
// reads like the rest of the table ("read this when …").
func episodeHint(start, end time.Time) string {
	s, e := dateOrEmpty(start), dateOrEmpty(end)
	switch {
	case s == "" && e == "":
		return "session memory (topics not synthesized yet)"
	case s == "":
		return "session memory through " + e
	case e == "" || s == e:
		return "session memory from " + s
	default:
		return "session memory from " + s + " to " + e
	}
}

func dateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// buildClaudeBlock emits the tiny always-loaded pointer.
func buildClaudeBlock(in FoldInput) string {
	var b strings.Builder
	b.WriteString("## MOM Vault (projected memory)\n\n")
	b.WriteString("**Read the vault files below for this project's memory.** They are pre-synthesized markdown — open them with your file tools; there is no search command to run.\n\n")
	b.WriteString("1. Read `.mom/vault/INDEX.md` first — it routes you to the right file.\n")
	b.WriteString("2. Topics: `.mom/vault/topics/<slug>.md` — decisions, patterns, preferences by subject.\n")
	b.WriteString("3. Timeline: `.mom/vault/timeline/<YYYY-MM>.md` — chronological history.\n")
	b.WriteString("4. Overview: `.mom/vault/summaries/overview.md` — high-level project summary.\n\n")
	b.WriteString("The vault synthesizes and POINTS AT immutable captured memories. **Do not rewrite vault files** — direct edits are lost on the next fold.\n")
	fmt.Fprintf(&b, "\n_Folded through Ledger offset **%d** (project `%s`)._\n", in.ToOffset, in.ProjectID)
	return b.String()
}

// memoryType is the FoldEvent.Type value for recorded memories.
const memoryType = "capture.memory.recorded"

func clusterSummary(turns []FoldEvent) string {
	// Prefer the first substantive user turn, else first turn text.
	for _, t := range turns {
		if strings.EqualFold(t.Role, "user") && strings.TrimSpace(t.Text) != "" {
			return truncate(t.Text, snippetMaxChars)
		}
	}
	if len(turns) > 0 {
		return truncate(turns[0].Text, snippetMaxChars)
	}
	return "(no text)"
}

func shortSession(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func tagSlug(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	var b strings.Builder
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/' || r == ':':
			b.WriteRune('-')
		case r == '-':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
