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
		// Force layer/access_tier by path on every re-render: an LLM mislabel
		// that isn't a bare numeral (e.g. "layer: A" on a reference file)
		// parses as a valid layer and would otherwise survive folds forever.
		if layer, tier, ok := layerForPath(p); ok {
			fm.Layer = layer
			fm.AccessTier = tier
		}
		docs = append(docs, &relDoc{
			path:  p,
			fm:    fm,
			body:  stripRelatedSection(body),
			title: conceptName(p, c),
		})
	}

	for _, d := range docs {
		// Vertical lineage: children are exactly one level below whose offsets
		// are covered by this file's sources. children/ stays machine-facing
		// (frontmatter), mirroring how sources is provenance, not navigation.
		// The source set is built once per doc — with thousands of episodes a
		// per-pair set rebuild dominates the fold's post-pass.
		var children []string
		if d.fm.Layer == "A" {
			// identity.md carries no sources (see stampIdentityProvenance) —
			// it is the overview of the WHOLE concept layer by layer alone,
			// no offset overlap needed.
			for _, o := range docs {
				if o.path != d.path && layerRank(o.fm.Layer) == layerRank(d.fm.Layer)-1 {
					children = append(children, o.path)
				}
			}
		} else {
			dset := offsetSet(d.fm.Sources)
			for _, o := range docs {
				if o.path == d.path || layerRank(o.fm.Layer) != layerRank(d.fm.Layer)-1 {
					continue
				}
				if intersectsSet(o.fm.Sources, dset) {
					children = append(children, o.path)
				}
			}
		}
		sort.Strings(children)
		d.fm.Children = children

		// Episodes are leaves: no Related section.
		if layerRank(d.fm.Layer) == 0 {
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

// relatedLinks builds the body link list for one document: same-level concept
// docs ranked by how strongly they co-occur, then the parent overview if any.
//
// Relatedness is scored primarily by SOURCE-OFFSET OVERLAP — two concepts
// whose provenance shares captured windows were literally worked on together.
// Shared tags are the secondary signal. Tags alone used to be the only
// signal, which silently produced zero links in ICM vaults: every subject
// concept carries its own unique slug as its tag, so no two ever matched.
func relatedLinks(d *relDoc, docs []*relDoc) []string {
	tagset := make(map[string]bool, len(d.fm.Tags))
	for _, t := range d.fm.Tags {
		tagset[t] = true
	}
	dset := offsetSet(d.fm.Sources)

	type scored struct {
		path, title           string
		sharedOff, sharedTags int
	}

	var sibs []scored
	for _, o := range docs {
		// Same level only; episodes (level 0) and INDEX files never link.
		if o.path == d.path || layerRank(o.fm.Layer) != layerRank(d.fm.Layer) || layerRank(o.fm.Layer) == 0 || o.fm.Type == typeIndex {
			continue
		}
		sharedOff := 0
		for _, off := range o.fm.Sources {
			if _, ok := dset[off]; ok {
				sharedOff++
			}
		}
		sharedTags := 0
		for _, t := range o.fm.Tags {
			if tagset[t] {
				sharedTags++
			}
		}
		if sharedOff > 0 || sharedTags > 0 {
			sibs = append(sibs, scored{o.path, o.title, sharedOff, sharedTags})
		}
	}
	sort.Slice(sibs, func(i, j int) bool {
		if sibs[i].sharedOff != sibs[j].sharedOff {
			return sibs[i].sharedOff > sibs[j].sharedOff
		}
		if sibs[i].sharedTags != sibs[j].sharedTags {
			return sibs[i].sharedTags > sibs[j].sharedTags
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

	// Parent overview: a doc exactly one layer up is the overview by layer
	// alone (this is how identity.md — carrying no sources — becomes the
	// overview of every B concept); anything further above still needs
	// offset overlap to qualify.
	var parents []scored
	for _, o := range docs {
		if o.path == d.path {
			continue
		}
		if layerRank(o.fm.Layer) == layerRank(d.fm.Layer)+1 ||
			(layerRank(o.fm.Layer) > layerRank(d.fm.Layer) && intersectsSet(d.fm.Sources, offsetSet(o.fm.Sources))) {
			parents = append(parents, scored{path: o.path, title: o.title})
		}
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].path < parents[j].path })
	if len(parents) > 0 {
		links = append(links, "- "+markdownLink(d.path, parents[0].path, parents[0].title)+" _(overview)_")
	}
	return links
}

// offsetSet builds a lookup set from an offset list.
func offsetSet(offs []uint64) map[uint64]struct{} {
	set := make(map[uint64]struct{}, len(offs))
	for _, o := range offs {
		set[o] = struct{}{}
	}
	return set
}

// intersectsSet reports whether any offset in a is present in set. An episode
// "belongs to" a concept when they share at least one ledger offset.
func intersectsSet(a []uint64, set map[uint64]struct{}) bool {
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
