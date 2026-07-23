package projection

import (
	"fmt"
	"sort"
	"strings"
)

// ICM (Interpretable Context Methodology) vault layout. The vault is projected
// from the Ledger into the five ICM layers, written in OKF (Open Knowledge
// Format): per-folder INDEX.md routers and concept files carrying type / name /
// description metadata an agent can scan before opening anything.
//
//	INDEX.md       — L2 Routing: root OKF index (identity blurb + folder routing)
//	identity.md    — L1 Identity: what the project is (one concept)
//	reference/     — L4 Reference: canonical, deduped decisions/subjects
//	conventions/   — L3 Stage Contract: process, conventions, workflow rules
//	episodes/      — raw L0 capture: provenance, hidden from routing
//
// There is deliberately no history/dev-log layer: ICM has none, and chronology
// is provenance (episodes + the Ledger), not a synthesized concept.
const (
	identityFile   = "identity.md"
	referenceDir   = "reference"
	conventionsDir = "conventions"
	episodesDir    = "episodes"

	typeIdentity   = "identity"
	typeReference  = "reference"
	typeConvention = "convention"
	typeEpisode    = "episode"
	typeIndex      = "index"
)

// buildPerFolderIndexes generates an OKF INDEX.md inside each routable folder
// (reference/, conventions/), listing every concept with its name and description
// so the agent can pick a file without opening any. It mutates files in place,
// adding "<dir>/INDEX.md" entries. Episodes are provenance — no index.
func buildPerFolderIndexes(files map[string]string) {
	for _, dir := range []string{referenceDir, conventionsDir} {
		type row struct{ path, name, desc, through, author string }
		var rows, docRows []row
		for p, c := range files {
			if !strings.HasPrefix(p, dir+"/") || strings.HasSuffix(p, "/"+indexFileName) {
				continue
			}
			fm, body := ParseFrontmatter(c)
			name := fm.Name
			if name == "" {
				name = firstHeadingTitle(c)
			}
			desc := fm.Description
			if desc == "" {
				desc = firstSentence(body)
			}
			// "Through" is the concept's fact-freshness: the date of the last
			// captured event that contributed to it — NOT when it was folded.
			through := ""
			if !fm.TimeRangeEnd.IsZero() {
				through = fm.TimeRangeEnd.UTC().Format("2006-01-02")
			}
			r := row{p, name, desc, through, sourceAuthor(body)}
			if fm.Subtype == "document" {
				docRows = append(docRows, r)
			} else {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 && len(docRows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].path < rows[j].path })
		sort.Slice(docRows, func(i, j int) bool { return docRows[i].path < docRows[j].path })

		var b strings.Builder
		b.WriteString(RenderFrontmatter(Frontmatter{Type: typeIndex, Version: 1}))
		fmt.Fprintf(&b, "# %s — index\n\n", strings.Title(dir)) //nolint:staticcheck
		b.WriteString("_\"Through\" is the date of the last captured fact in the concept — its freshness, not its fold time._\n\n")

		writeRow := func(b *strings.Builder, r row) {
			base := strings.TrimPrefix(r.path, dir+"/")
			name := r.name
			if name == "" {
				name = base
			}
			b.WriteString("| [`" + base + "`](" + base + ") | " + name)
			if r.desc != "" && r.desc != name {
				b.WriteString(" — " + r.desc)
			}
			b.WriteString(" | " + r.through + " |\n")
		}

		if len(rows) > 0 {
			b.WriteString("| Concept | What it covers | Through |\n|---|---|---|\n")
			for _, r := range rows {
				writeRow(&b, r)
			}
		}
		if len(docRows) > 0 {
			if len(rows) > 0 {
				b.WriteString("\n")
			}
			b.WriteString("## 📖 Documents\n\n")
			b.WriteString("| Book | Author | Through |\n|---|---|---|\n")
			for _, r := range docRows {
				base := strings.TrimPrefix(r.path, dir+"/")
				name := r.name
				if name == "" {
					name = base
				}
				b.WriteString("| [`" + base + "`](" + base + ") | " + name + " | " + r.author + " | " + r.through + " |\n")
			}
		}
		files[dir+"/"+indexFileName] = b.String()
	}
}

// firstHeadingTitle extracts the first markdown H1 from content (after any
// frontmatter), stripped of legacy "Topic:"/"Summary:" prefixes.
func firstHeadingTitle(content string) string {
	_, body := ParseFrontmatter(content)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			return ""
		}
		t := strings.TrimSpace(strings.TrimPrefix(line, "# "))
		for _, p := range []string{"Topic:", "Summary:", "Timeline:"} {
			t = strings.TrimSpace(strings.TrimPrefix(t, p))
		}
		return t
	}
	return ""
}

// sourceAuthor extracts the author from a document concept's trailing
// "Source: <title> by <author>" line (see buildDocumentConceptHint), empty
// when absent (the model omits "by <author>" when the text never states one).
func sourceAuthor(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Source:") {
			continue
		}
		if i := strings.LastIndex(line, " by "); i >= 0 {
			return strings.TrimSpace(line[i+len(" by "):])
		}
	}
	return ""
}

// firstSentence returns a short one-line description from a markdown body: the
// first non-heading, non-bullet prose line, truncated to ~140 chars.
func firstSentence(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "|") {
			continue
		}
		if i := strings.Index(line, ". "); i > 20 && i < 140 {
			return line[:i+1]
		}
		if len(line) > 140 {
			return strings.TrimSpace(line[:140]) + "…"
		}
		return line
	}
	return ""
}
