package recall

import (
	"fmt"
	"testing"

	"github.com/momhq/mom/cli/internal/adapters/storage"
)

// mockSearcher implements Searcher for testing.
// Results are keyed by "AND-curated", "OR-curated", "AND-draft", "OR-draft".
type mockSearcher struct {
	name    string
	results map[string][]storage.SearchResult
	calls   []string // records call signatures for assertion
}

func (m *mockSearcher) Search(query string, qt storage.QueryType, includeDrafts bool, limit int) ([]storage.SearchResult, error) {
	qtStr := "OR"
	if qt == storage.QueryAND {
		qtStr = "AND"
	}
	qualStr := "curated"
	if includeDrafts {
		qualStr = "draft"
	}
	key := fmt.Sprintf("%s-%s", qtStr, qualStr)
	m.calls = append(m.calls, fmt.Sprintf("%s:%s", m.name, key))
	return m.results[key], nil
}

func makeResult(id string, score float64, promotionState string) storage.SearchResult {
	return storage.SearchResult{ID: id, Score: score, PromotionState: promotionState}
}

// TestEngine_ANDSuccess: AND query meets threshold — no OR or scope escalation.
func TestEngine_ANDSuccess(t *testing.T) {
	s1 := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {makeResult("a", 1.0, "curated"), makeResult("b", 0.9, "curated"), makeResult("c", 0.8, "curated")},
	}}
	e := NewEngine([]Searcher{s1})
	results, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Only AND-curated should have been called.
	if len(s1.calls) != 1 || s1.calls[0] != "repo:AND-curated" {
		t.Errorf("expected only AND-curated call, got %v", s1.calls)
	}
}

// TestEngine_ANDORFallback: AND returns < threshold, OR fills the gap.
func TestEngine_ANDORFallback(t *testing.T) {
	s1 := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {makeResult("a", 1.0, "curated")},
		"OR-curated":  {makeResult("a", 1.0, "curated"), makeResult("b", 0.8, "curated"), makeResult("c", 0.7, "curated")},
	}}
	e := NewEngine([]Searcher{s1})
	results, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// AND-curated then OR-curated should be called; no draft pass.
	if len(s1.calls) != 2 {
		t.Errorf("expected 2 calls, got %v", s1.calls)
	}
}

// TestEngine_ScopeEscalation: repo scope insufficient, escalates to org.
func TestEngine_ScopeEscalation(t *testing.T) {
	repo := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {makeResult("a", 1.0, "curated")},
		"OR-curated":  {makeResult("a", 1.0, "curated")},
	}}
	org := &mockSearcher{name: "org", results: map[string][]storage.SearchResult{
		"AND-curated": {makeResult("b", 0.9, "curated"), makeResult("c", 0.8, "curated")},
	}}
	e := NewEngine([]Searcher{repo, org})
	results, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results from repo+org, got %d", len(results))
	}
	// Repo: AND+OR; org: AND.
	if len(repo.calls) != 2 {
		t.Errorf("expected 2 repo calls, got %v", repo.calls)
	}
	if len(org.calls) != 1 || org.calls[0] != "org:AND-curated" {
		t.Errorf("expected 1 org AND-curated call, got %v", org.calls)
	}
}

// TestEngine_DraftFallback: curated pass empty, draft pass fills results.
func TestEngine_DraftFallback(t *testing.T) {
	s1 := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {},
		"OR-curated":  {},
		"AND-draft":   {makeResult("d1", 0.9, "draft"), makeResult("d2", 0.8, "draft"), makeResult("d3", 0.7, "draft")},
	}}
	e := NewEngine([]Searcher{s1})
	results, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 draft results, got %d", len(results))
	}
	// Draft pass must have run.
	hasDraftCall := false
	for _, c := range s1.calls {
		if c == "repo:AND-draft" {
			hasDraftCall = true
		}
	}
	if !hasDraftCall {
		t.Errorf("expected draft fallback call, got %v", s1.calls)
	}
}

// TestEngine_NoDraftFallbackWhenCuratedSufficient: curated threshold met — no draft pass.
func TestEngine_NoDraftFallbackWhenCuratedSufficient(t *testing.T) {
	s1 := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {makeResult("a", 1.0, "curated"), makeResult("b", 0.9, "curated"), makeResult("c", 0.8, "curated")},
		"AND-draft":   {makeResult("d", 0.5, "draft")},
	}}
	e := NewEngine([]Searcher{s1})
	_, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range s1.calls {
		if c == "repo:AND-draft" || c == "repo:OR-draft" {
			t.Errorf("draft pass should not run when curated threshold met, got call %q", c)
		}
	}
}

// TestEngine_MergeRerank: higher score wins when same doc appears in multiple searches.
func TestEngine_MergeRerank(t *testing.T) {
	s1 := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {makeResult("a", 0.5, "curated")},
		"OR-curated":  {makeResult("a", 1.5, "curated"), makeResult("b", 1.0, "curated"), makeResult("c", 0.8, "curated")},
	}}
	e := NewEngine([]Searcher{s1})
	results, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	// "a" should have score 1.5 (OR score wins over AND score).
	if results[0].ID != "a" || results[0].Score != 1.5 {
		t.Errorf("expected a/1.5 first, got %s/%.1f", results[0].ID, results[0].Score)
	}
}

// TestEngine_ThresholdBoundary: exactly threshold results stops escalation.
func TestEngine_ThresholdBoundary(t *testing.T) {
	repo := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {
			makeResult("a", 1.0, "curated"),
			makeResult("b", 0.9, "curated"),
			makeResult("c", 0.8, "curated"),
		},
	}}
	org := &mockSearcher{name: "org", results: map[string][]storage.SearchResult{}}
	e := NewEngine([]Searcher{repo, org})
	_, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	// Org should never be called — threshold reached after repo AND.
	if len(org.calls) > 0 {
		t.Errorf("org should not be called when threshold met, got %v", org.calls)
	}
}

// TestEngine_EmptyChain: no searchers returns empty results without error.
func TestEngine_EmptyChain(t *testing.T) {
	e := NewEngine([]Searcher{})
	results, err := e.Search(Options{Query: "test", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results from empty chain, got %d", len(results))
	}
}

// TestEngine_TopNRespected: results capped at MaxResults even when more are found.
func TestEngine_TopNRespected(t *testing.T) {
	s1 := &mockSearcher{name: "repo", results: map[string][]storage.SearchResult{
		"AND-curated": {
			makeResult("a", 5.0, "curated"),
			makeResult("b", 4.0, "curated"),
			makeResult("c", 3.0, "curated"),
			makeResult("d", 2.0, "curated"),
			makeResult("e", 1.0, "curated"),
		},
	}}
	e := NewEngine([]Searcher{s1})
	results, err := e.Search(Options{Query: "test", MaxResults: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results (MaxResults), got %d", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("expected top result to be 'a', got %q", results[0].ID)
	}
}
