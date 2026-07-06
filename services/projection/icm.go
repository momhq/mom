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
//	contracts/     — L3 Stage Contract: process, conventions, workflow rules
//	episodes/      — raw L0 capture: provenance, hidden from routing
//
// There is deliberately no history/dev-log layer: ICM has none, and chronology
// is provenance (episodes + the Ledger), not a synthesized concept.
const (
	identityFile = "identity.md"
	referenceDir = "reference"
	contractsDir = "contracts"
	episodesDir  = "episodes"

	typeIdentity  = "identity"
	typeReference = "reference"
	typeContract  = "contract"
	typeEpisode   = "episode"
	typeIndex     = "index"
)

// icmFolder describes one routable ICM folder for the root router.
type icmFolder struct {
	dir     string
	layer   string // ICM layer label
	whenFor string // "read these when…"
}

// icmFolders is the routing order shown in the root INDEX.
var icmFolders = []icmFolder{
	{referenceDir, "Reference", "you need a decision, convention, or durable fact about a subject"},
	{contractsDir, "Stage Contract", "you need the process/rules for a kind of work (workflow, release, review)"},
}

// buildPerFolderIndexes generates an OKF INDEX.md inside each routable folder
// (reference/, contracts/), listing every concept with its name and description
// so the agent can pick a file without opening any. It mutates files in place,
// adding "<dir>/INDEX.md" entries. Episodes are provenance — no index.
func buildPerFolderIndexes(files map[string]string) {
	for _, dir := range []string{referenceDir, contractsDir} {
		type row struct{ path, name, desc string }
		var rows []row
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
			rows = append(rows, row{p, name, desc})
		}
		if len(rows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].path < rows[j].path })

		var b strings.Builder
		b.WriteString(RenderFrontmatter(Frontmatter{Type: typeIndex, Version: 1}))
		fmt.Fprintf(&b, "# %s — index\n\n", strings.Title(dir)) //nolint:staticcheck
		b.WriteString("| Concept | What it covers |\n|---|---|\n")
		for _, r := range rows {
			// Link is relative to this folder, so just the base filename.
			base := strings.TrimPrefix(r.path, dir+"/")
			name := r.name
			if name == "" {
				name = base
			}
			b.WriteString("| [`" + base + "`](" + base + ") | " + name)
			if r.desc != "" && r.desc != name {
				b.WriteString(" — " + r.desc)
			}
			b.WriteString(" |\n")
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
