package projection

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// buildIndex emits the OKF root index (ICM Layer 2 — Routing): a
// routing-only map of the three ICM layers. It never enumerates individual
// concepts — that is each layer's own per-folder INDEX.md's job. It is the
// one file the agent reads first.
func buildIndex(files map[string]string, in FoldInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# MOM Vault — %s\n\n", in.ProjectID)
	b.WriteString("Projected memory for this project, organized as an ICM (Interpretable Context ")
	b.WriteString("Methodology) structure in OKF (Open Knowledge Format). **Read the matching file ")
	b.WriteString("before acting; open links directly — don't reconstruct paths.** Files are ")
	b.WriteString("regenerated on every fold — never edit them by hand.\n\n")

	routed := false

	if c, ok := files[identityFile]; ok {
		desc := conceptDescription(c)
		b.WriteString("## Layer A — identity\n")
		b.WriteString("- " + indexLink(identityFile) + " — " + firstNonEmpty(desc, "what this project is") + "\n\n")
		routed = true
	}

	if folderHasConcepts(files, referenceDir) || folderHasConcepts(files, conventionsDir) {
		b.WriteString("## Layer B — reference & conventions\n\n")
		if folderHasConcepts(files, referenceDir) {
			b.WriteString("- " + indexLink(referenceDir+"/"+indexFileName) + " — decisions, conventions, durable facts by subject\n")
		}
		if folderHasConcepts(files, conventionsDir) {
			b.WriteString("- " + indexLink(conventionsDir+"/"+indexFileName) + " — process/workflow rules for a kind of work\n")
		}
		b.WriteString("\n")
		routed = true
	}

	if routed {
		// Vault has real synthesized memory: episodes are just backing
		// provenance, noted but not enumerated.
		if folderHasConcepts(files, episodesDir) {
			b.WriteString("## Layer C — episodes (provenance)\n\n")
			b.WriteString("Raw session capture, not enumerated here — provenance for the concepts above.\n\n")
		}
	} else {
		// Young vault below the synthesis threshold: route to raw episodes so the
		// router never goes blind while real memory sits in episodes/.
		rows := episodeIndexRows(files)
		if len(rows) == 0 {
			b.WriteString("_(no files yet — run `mom vault fold` after capturing some sessions)_\n")
		} else {
			b.WriteString("## Recent capture\n\n| Read this | What it covers |\n|---|---|\n")
			for _, r := range rows {
				b.WriteString("| " + indexLink(r.path) + " | " + r.hint + " |\n")
			}
		}
	}

	b.WriteString("\n---\n")
	engine := in.Engine
	if engine == "" {
		engine = "llm"
	}
	fmt.Fprintf(&b, "_Watermark: folded through Ledger offset **%d** at %s (%s engine)._\n",
		in.ToOffset, time.Now().UTC().Format(time.RFC3339), engine)
	return b.String()
}

// folderHasConcepts reports whether dir holds at least one concept (non-index) file.
func folderHasConcepts(files map[string]string, dir string) bool {
	for p := range files {
		if strings.HasPrefix(p, dir+"/") && !strings.HasSuffix(p, "/"+indexFileName) {
			return true
		}
	}
	return false
}

// conceptName returns the display name for a concept: OKF name, else H1 title,
// else humanized filename.
func conceptName(path, content string) string {
	fm, _ := ParseFrontmatter(content)
	if fm.Name != "" {
		return fm.Name
	}
	if t := firstHeadingTitle(content); t != "" {
		return t
	}
	base := path[strings.LastIndex(path, "/")+1:]
	return strings.ReplaceAll(strings.TrimSuffix(base, ".md"), "-", " ")
}

// conceptDescription returns the OKF description, else a first-sentence fallback.
func conceptDescription(content string) string {
	fm, body := ParseFrontmatter(content)
	if fm.Description != "" {
		return fm.Description
	}
	return firstSentence(body)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// indexLink renders a vault-relative path as a clickable markdown link whose
// text is the path itself. INDEX.md sits at the vault root, so a relative
// target like `reference/x.md` resolves directly — the agent opens the link
// instead of rebuilding the path from scratch.
func indexLink(path string) string {
	return "[`" + path + "`](" + path + ")"
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
		return "session memory (subjects not synthesized yet)"
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

// buildEntryRouter emits the tiny always-loaded pointer (ICM Layer 1/2 hook)
// written into the harness entry files (CLAUDE.md, AGENTS.md, …): a
// situational "route by what just happened" table, since OKF is not yet a
// standard the agent knows out of the box.
func buildEntryRouter(in FoldInput) string {
	var b strings.Builder
	b.WriteString("## MOM Vault (projected memory)\n\n")
	b.WriteString("This project's memory is an **ICM** structure in **OKF** format under `.mom/vault/`. ")
	b.WriteString("Each folder has its own `INDEX.md`; each concept file carries `type` / `name` / ")
	b.WriteString("`description` frontmatter — scan those to decide what to open, don't read everything.\n\n")
	b.WriteString("Route by what you're about to do:\n\n")
	b.WriteString("| If you're about to… | Go to | Then stop at |\n|---|---|---|\n")
	b.WriteString("| state what this project IS | `.mom/vault/identity.md` | identity.md |\n")
	b.WriteString("| recall a decision, convention, or fact | `.mom/vault/reference/INDEX.md` | the matching concept file |\n")
	b.WriteString("| follow a process or workflow rule | `.mom/vault/conventions/INDEX.md` | the matching convention file |\n")
	b.WriteString("| find raw provenance for a claim | `.mom/vault/INDEX.md` → Layer C | the episode file |\n\n")
	b.WriteString("Never invent past decisions — if no vault file matches, say so. The vault is regenerated ")
	b.WriteString("from the Ledger on every fold. **Do not edit vault files by hand** — changes are lost on the next fold.\n")
	fmt.Fprintf(&b, "\n_Folded through Ledger offset **%d** (project `%s`)._\n", in.ToOffset, in.ProjectID)
	return b.String()
}

// memoryType is the FoldEvent.Type value for recorded memories.
const memoryType = "capture.memory.recorded"

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
