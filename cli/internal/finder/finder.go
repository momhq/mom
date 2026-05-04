// Package finder is the v0.30 search service over Vault. Renamed from
// "Recall" to avoid colliding with the user-facing mom_recall / mom
// recall verbs.
//
// Finder reads through Librarian — it never imports the Vault package
// directly. The architectural rule is locked by an import-graph test
// in finder_test.go.
//
// Finder combines:
//   - FTS5 ranking with column weights from ADR 0007 (0/2/10 over
//     id/summary/content_text).
//   - AND→OR query relaxation from ADR 0008. Single-token queries are
//     unchanged; multi-token queries first try AND (precision) and
//     widen to OR (recall) only when the precise pass returned too few
//     results.
//   - Curated/draft tier escalation from ADR 0006 (quality dimension).
//     The progressive scope-chain dimension of ADR 0006 is gone with
//     the scope chain — there is one vault.
//
// Finder does NOT re-run capture-time filters (ADR 0014) — those live
// in Drafter and apply at the write boundary.
package finder

import (
	"errors"
	"fmt"
	"strings"

	"github.com/momhq/mom/cli/internal/librarian"
)

// ErrEmptyQuery is returned by Recall when the input query is empty
// after trimming. The lesson from the previous attempt: empty query
// must reject loudly with "query is required" rather than silently
// returning "no memories matched" — buggy callers were getting
// misdiagnosed as cold-cache.
var ErrEmptyQuery = errors.New("finder: query is required")

// Options narrows a Recall call. Query is required; everything else is
// optional. Limit defaults to 20.
type Options struct {
	Query         string
	Tags          []string // memory must have ALL these tags
	SessionID     string
	IncludeDrafts bool // when false (default), Finder applies tier escalation
	Limit         int
}

// Result is one ranked memory hit. Score is the BM25 score from FTS5
// (lower = more relevant in SQLite's bm25 convention); Tier identifies
// which escalation pass matched ("curated" / "draft" / "draft-or").
type Result struct {
	librarian.Memory
	Score float64
	Tier  string
}

// Finder is the search service. Construct with New.
type Finder struct {
	lib *librarian.Librarian

	// thresholdLow is the result count below which Finder escalates to
	// the next pass (more drafts, then OR-relaxation). Tuned for
	// personal-vault scale; configurable via WithThresholdLow.
	thresholdLow int
}

// New returns a Finder backed by the given Librarian.
func New(lib *librarian.Librarian) *Finder {
	return &Finder{lib: lib, thresholdLow: 5}
}

// WithThresholdLow configures the minimum result count that prevents
// further escalation. Defaults to 5.
func (f *Finder) WithThresholdLow(n int) *Finder {
	if n > 0 {
		f.thresholdLow = n
	}
	return f
}

// Recall executes the search with relaxation + tier escalation. Returns
// ranked results (best first) up to opts.Limit (default 20).
//
// Pipeline (each pass stops if it yields >= thresholdLow OR Limit):
//  1. curated + AND  — most precise.
//  2. curated + OR   — multi-token queries only; widen recall while
//                      keeping the curated tier.
//  3. drafts + AND   — drop the curated gate.
//  4. drafts + OR    — multi-token queries only; widest pass.
//
// IncludeDrafts=true skips the curated-only passes.
func (f *Finder) Recall(opts Options) ([]Result, error) {
	if strings.TrimSpace(opts.Query) == "" {
		return nil, ErrEmptyQuery
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	multiToken := tokenCount(opts.Query) > 1
	andQ := normaliseFTSQuery(opts.Query)
	orQ := buildORQuery(opts.Query)

	type pass struct {
		ftsQ  string
		state string // "" = any
		tier  string
	}
	passes := []pass{
		{andQ, "curated", "curated"},
	}
	if multiToken {
		passes = append(passes, pass{orQ, "curated", "curated-or"})
	}
	if !opts.IncludeDrafts {
		passes = append(passes, pass{andQ, "", "draft"})
		if multiToken {
			passes = append(passes, pass{orQ, "", "draft-or"})
		}
	} else {
		// IncludeDrafts skips the curated-only passes entirely.
		passes = []pass{{andQ, "", "draft"}}
		if multiToken {
			passes = append(passes, pass{orQ, "", "draft-or"})
		}
	}

	for _, p := range passes {
		hits, err := f.lib.SearchMemories(librarian.SearchFilter{
			FTSQuery:       p.ftsQ,
			Tags:           opts.Tags,
			SessionID:      opts.SessionID,
			PromotionState: p.state,
			Limit:          limit,
		})
		if err != nil {
			return nil, fmt.Errorf("Recall %s pass: %w", p.tier, err)
		}
		if len(hits) >= f.thresholdLow || len(hits) >= limit {
			return resultsFrom(hits, p.tier), nil
		}
		// Keep going — this pass yielded too few. Last pass returns
		// whatever it has even if below threshold.
	}

	// All passes exhausted; return the last (widest) result, even if
	// short of the threshold. (We keep its tier label.)
	last := passes[len(passes)-1]
	hits, err := f.lib.SearchMemories(librarian.SearchFilter{
		FTSQuery:       last.ftsQ,
		Tags:           opts.Tags,
		SessionID:      opts.SessionID,
		PromotionState: last.state,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	return resultsFrom(hits, last.tier), nil
}

func resultsFrom(hits []librarian.SearchedMemory, tier string) []Result {
	out := make([]Result, 0, len(hits))
	for _, h := range hits {
		out = append(out, Result{Memory: h.Memory, Score: h.Score, Tier: tier})
	}
	return out
}

// tokens splits the query on whitespace, trimming each. Empty tokens
// are removed.
func tokens(query string) []string {
	parts := strings.Fields(query)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// tokenCount is the number of FTS5-tokenisable terms in the query.
// Used to decide whether OR-relaxation is meaningful.
func tokenCount(query string) int {
	return len(tokens(query))
}

// normaliseFTSQuery returns the verbatim query for FTS5 MATCH. FTS5
// defaults to AND between bare terms, so a trimmed query is already
// the precise pass.
func normaliseFTSQuery(query string) string {
	return strings.TrimSpace(query)
}

// buildORQuery rewrites the query as an OR of its tokens. Each token
// is quoted to preserve FTS5 phrase semantics for any embedded
// punctuation or operators the input might contain.
func buildORQuery(query string) string {
	tt := tokens(query)
	if len(tt) == 0 {
		return ""
	}
	if len(tt) == 1 {
		return tt[0]
	}
	parts := make([]string, len(tt))
	for i, t := range tt {
		parts[i] = `"` + escapeFTS(t) + `"`
	}
	return strings.Join(parts, " OR ")
}

// escapeFTS doubles any embedded double-quote so the returned string
// is safe inside an FTS5 phrase.
func escapeFTS(s string) string {
	return strings.ReplaceAll(s, `"`, `""`)
}
