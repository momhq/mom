package projection

import (
	"path/filepath"
	"sort"
	"strings"
)

const (
	relatedHeading = "## Related"
	// maxRelatedSiblings caps the body link list so generic, high-frequency
	// tags (the over-linking that sinks landmark selection on large corpora)
	// can't turn the section into noise.
	maxRelatedSiblings = 7
)

// relDoc is a parsed vault file used for graph computation.
type relDoc struct {
	path  string
	fm    Frontmatter
	body  string
	title string
}

// linkRelated wires the finalized vault into a navigable graph as a
// deterministic post-pass, run before the INDEX is built. It separates the two
// concerns the raw `sources` offsets conflate:
//
//   - PROVENANCE (machine): each L1/L2 file's `children` frontmatter is set from
//     offset containment — an episode whose offsets fall inside a topic's
//     `sources` is that topic's child; a topic whose offsets fall inside the
//     overview's `sources` is the overview's child. This is the technical
//     lineage, derived from `sources` rather than echoing it.
//   - NAVIGATION (agent): a "## Related" body section links sibling documents
//     that share tags, plus the parent overview, with markdown links the agent
//     can follow directly — so a topic is no longer an island reachable only
//     top-down through INDEX.
//
// It mutates files in place. Episodes participate as children but get no
// Related section of their own (they are raw captures, not navigation hubs).
func linkRelated(files map[string]string) {
	docs := make([]*relDoc, 0, len(files))
	for p, c := range files {
		if p == indexFileName {
			continue
		}
		fm, body := ParseFrontmatter(c)
		docs = append(docs, &relDoc{
			path:  p,
			fm:    fm,
			body:  stripRelatedSection(body),
			title: routerHint(p, c),
		})
	}

	for _, d := range docs {
		// Vertical lineage: children are exactly one level below whose offsets
		// are covered by this file's sources. children/ stays machine-facing
		// (frontmatter), mirroring how sources is provenance, not navigation.
		var children []string
		for _, o := range docs {
			if o.path == d.path || o.fm.Level != d.fm.Level-1 {
				continue
			}
			if offsetsIntersect(o.fm.Sources, d.fm.Sources) {
				children = append(children, o.path)
			}
		}
		sort.Strings(children)
		d.fm.Children = children

		// Episodes are leaves: no Related section.
		if d.fm.Level == 0 {
			files[d.path] = PrependFrontmatter(d.fm, ensureTrailingNewline(d.body))
			continue
		}

		links := relatedLinks(d, docs)
		body := strings.TrimRight(d.body, "\n")
		if len(links) > 0 {
			body += "\n\n" + relatedHeading + "\n" + strings.Join(links, "\n") + "\n"
		} else {
			body = ensureTrailingNewline(body)
		}
		files[d.path] = PrependFrontmatter(d.fm, body)
	}
}

// relatedLinks builds the body link list for one document: sibling docs of the
// same kind sharing the most tags (capped), then the parent overview if any.
func relatedLinks(d *relDoc, docs []*relDoc) []string {
	tagset := make(map[string]bool, len(d.fm.Tags))
	for _, t := range d.fm.Tags {
		tagset[t] = true
	}

	type scored struct {
		path, title string
		shared      int
	}
	var sibs []scored
	for _, o := range docs {
		if o.path == d.path || o.fm.Kind != d.fm.Kind {
			continue
		}
		shared := 0
		for _, t := range o.fm.Tags {
			if tagset[t] {
				shared++
			}
		}
		if shared > 0 {
			sibs = append(sibs, scored{o.path, o.title, shared})
		}
	}
	sort.Slice(sibs, func(i, j int) bool {
		if sibs[i].shared != sibs[j].shared {
			return sibs[i].shared > sibs[j].shared
		}
		return sibs[i].path < sibs[j].path
	})

	links := make([]string, 0, maxRelatedSiblings+1)
	for _, s := range sibs {
		if len(links) >= maxRelatedSiblings {
			break
		}
		links = append(links, "- "+markdownLink(d.path, s.path, s.title))
	}

	// Parent overview: the lowest-pathed higher-level file whose sources cover
	// this one's offsets.
	var parents []scored
	for _, o := range docs {
		if o.fm.Level > d.fm.Level && offsetsIntersect(d.fm.Sources, o.fm.Sources) {
			parents = append(parents, scored{o.path, o.title, 0})
		}
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].path < parents[j].path })
	if len(parents) > 0 {
		links = append(links, "- "+markdownLink(d.path, parents[0].path, parents[0].title)+" _(overview)_")
	}
	return links
}

// offsetsIntersect reports whether the two offset lists share any value. An
// episode "belongs to" a topic when they share at least one ledger offset.
func offsetsIntersect(a, b []uint64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[uint64]struct{}, len(b))
	for _, o := range b {
		set[o] = struct{}{}
	}
	for _, o := range a {
		if _, ok := set[o]; ok {
			return true
		}
	}
	return false
}

// markdownLink renders a link from one vault file to another, with the target's
// path made relative to the source file's directory so it resolves on disk
// (e.g. a topic linking the overview yields `../summaries/overview.md`).
func markdownLink(fromPath, toPath, title string) string {
	rel, err := filepath.Rel(filepath.Dir(fromPath), toPath)
	if err != nil {
		rel = toPath
	}
	return "[" + title + "](" + rel + ")"
}

// stripRelatedSection removes a previously injected "## Related" section so
// re-folds refresh it rather than stacking duplicates. The section is always
// appended last, so everything from its heading to EOF is the prior block.
func stripRelatedSection(body string) string {
	idx := strings.Index(body, relatedHeading)
	if idx < 0 {
		return body
	}
	return ensureTrailingNewline(strings.TrimRight(body[:idx], "\n"))
}

func ensureTrailingNewline(s string) string {
	if s == "" {
		return s
	}
	return strings.TrimRight(s, "\n") + "\n"
}
