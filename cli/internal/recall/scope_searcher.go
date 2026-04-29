package recall

import (
	"github.com/momhq/mom/cli/internal/adapters/storage"
)

// ScopeSearcher implements Searcher for a single local .mom/ scope
// backed by a shared IndexedAdapter.
type ScopeSearcher struct {
	adapter   *storage.IndexedAdapter
	scopePath string
}

// NewScopeSearcher creates a ScopeSearcher for the given scope path.
// The adapter is shared across scopes; scopePath limits results to this scope.
func NewScopeSearcher(adapter *storage.IndexedAdapter, scopePath string) *ScopeSearcher {
	return &ScopeSearcher{adapter: adapter, scopePath: scopePath}
}

// Search queries this scope with the given query type and quality filter.
func (s *ScopeSearcher) Search(query string, queryType storage.QueryType, includeDrafts bool, limit int) ([]storage.SearchResult, error) {
	return s.adapter.Search(storage.SearchOptions{
		Query:         query,
		QueryType:     queryType,
		ScopePaths:    []string{s.scopePath},
		ExcludeDrafts: !includeDrafts,
		Limit:         limit,
	})
}
