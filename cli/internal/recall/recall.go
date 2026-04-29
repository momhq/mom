// Package recall implements progressive scope and quality escalation for mom_recall.
// See ADR 0006 for the full design rationale.
package recall

import (
	"sort"

	"github.com/momhq/mom/cli/internal/adapters/storage"
)

// recallEscalationThreshold is the minimum number of results required before
// the engine stops escalating to a wider scope or lower quality tier.
// Named for easy promotion to config when the need arises.
const recallEscalationThreshold = 3

// Searcher searches a single scope with a given query type and quality filter.
// Implementations include ScopeSearcher (local SQLite) and future vault searchers.
type Searcher interface {
	Search(query string, queryType storage.QueryType, includeDrafts bool, limit int) ([]storage.SearchResult, error)
}

// Options controls an Engine.Search call.
type Options struct {
	// Query is the free-text search string.
	Query string
	// MaxResults caps the final result count (default 5).
	MaxResults int
	// Tags filters results by tags (AND logic).
	Tags []string
	// SessionID restricts results to a specific session.
	SessionID string
}

// Engine runs two-pass progressive escalation over an ordered Searcher chain.
//
// Pass 1 — curated only: for each scope, try AND query then OR query.
// Stop as soon as results >= recallEscalationThreshold.
//
// Pass 2 — draft fallback: repeat the same chain including drafts,
// only when Pass 1 did not meet the threshold.
//
// Results from all queried scopes are merged and re-ranked by score.
// The highest score for a given document ID wins across all searches.
type Engine struct {
	chain []Searcher
}

// NewEngine creates an Engine with the given ordered Searcher chain.
// Chain order defines escalation priority: first entry is searched first.
func NewEngine(chain []Searcher) *Engine {
	return &Engine{chain: chain}
}

// Search runs the two-pass escalation and returns top-N results by score.
func (e *Engine) Search(opts Options) ([]storage.SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = 5
	}

	seen := make(map[string]storage.SearchResult)

	// Pass 1: curated memories only.
	e.runPass(opts.Query, false, maxResults, seen)

	// Pass 2: include drafts if Pass 1 did not reach the threshold.
	if len(seen) < recallEscalationThreshold {
		e.runPass(opts.Query, true, maxResults, seen)
	}

	results := make([]storage.SearchResult, 0, len(seen))
	for _, r := range seen {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

// runPass iterates the chain with AND→OR relaxation per scope.
// Stops as soon as the seen map reaches recallEscalationThreshold.
func (e *Engine) runPass(query string, includeDrafts bool, limit int, seen map[string]storage.SearchResult) {
	for _, s := range e.chain {
		if len(seen) >= recallEscalationThreshold {
			return
		}

		// AND first — stricter, higher precision.
		if results, err := s.Search(query, storage.QueryAND, includeDrafts, limit); err == nil {
			mergeResults(seen, results)
		}
		if len(seen) >= recallEscalationThreshold {
			return
		}

		// OR fallback — broader, captures partial matches.
		if results, err := s.Search(query, storage.QueryOR, includeDrafts, limit); err == nil {
			mergeResults(seen, results)
		}
	}
}

// mergeResults adds results to seen, keeping the highest score per document ID.
func mergeResults(seen map[string]storage.SearchResult, results []storage.SearchResult) {
	for _, r := range results {
		if existing, ok := seen[r.ID]; !ok || r.Score > existing.Score {
			seen[r.ID] = r
		}
	}
}
