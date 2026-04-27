package search_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/momhq/mom/cli/internal/search"
)

// writeDoc writes a minimal memory JSON doc to dir.
func writeDoc(t *testing.T, dir, id, summary string, tags []string, session string) {
	t.Helper()
	type doc struct {
		ID            string         `json:"id"`
		Tags          []string       `json:"tags"`
		Summary       string         `json:"summary"`
		SourceSession string         `json:"source_session"`
		Lifecycle     string         `json:"lifecycle"`
		Created       string         `json:"created"`
		Content       map[string]any `json:"content"`
	}
	d := doc{
		ID:            id,
		Tags:          tags,
		Summary:       summary,
		SourceSession: session,
		Lifecycle:     "draft",
		Created:       "2026-01-01T00:00:00Z",
		Content:       map[string]any{"detail": summary},
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

// TestSearch_BasicQuery verifies BM25 search returns relevant results.
func TestSearch_BasicQuery(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "doc-001", "BM25 ranking algorithm for memory search", []string{"bm25", "ranking", "search"}, "sess-1")
	writeDoc(t, dir, "doc-002", "RAKE keyword extraction pipeline", []string{"rake", "keyword", "extraction"}, "sess-1")
	writeDoc(t, dir, "doc-003", "Session recording with raw JSONL files", []string{"recording", "jsonl", "session"}, "sess-2")

	results, err := search.Search(dir, search.SearchOptions{Query: "bm25 ranking", MaxResults: 5})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result, got 0")
	}
	// The BM25-related doc should rank first.
	if results[0].ID != "doc-001" {
		t.Errorf("expected doc-001 to rank first, got %q", results[0].ID)
	}
	// Scores should be positive.
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("result %q has non-positive score %f", r.ID, r.Score)
		}
	}
	// Scores should be sorted descending.
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: [%d].Score=%f > [%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

// TestSearch_TagFilter verifies AND-logic tag filtering.
func TestSearch_TagFilter(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "doc-001", "BM25 search with ranking", []string{"bm25", "search", "ranking"}, "sess-1")
	writeDoc(t, dir, "doc-002", "RAKE extraction algorithm", []string{"rake", "extraction"}, "sess-1")
	writeDoc(t, dir, "doc-003", "BM25 and RAKE combined", []string{"bm25", "rake", "combined"}, "sess-2")

	// Filter to docs that have both "bm25" and "rake".
	results, err := search.Search(dir, search.SearchOptions{
		Query:      "",
		MaxResults: 10,
		Tags:       []string{"bm25", "rake"},
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with both bm25 and rake tags, got %d", len(results))
	}
	if results[0].ID != "doc-003" {
		t.Errorf("expected doc-003, got %q", results[0].ID)
	}
}

// TestSearch_NoResults verifies empty result when nothing matches.
func TestSearch_NoResults(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "doc-001", "Session recording pipeline", []string{"recording", "pipeline"}, "sess-1")

	results, err := search.Search(dir, search.SearchOptions{Query: "xyznonexistent", MaxResults: 5})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching query, got %d", len(results))
	}
}

// TestSearch_EmptyQuery returns all docs with equal score when no query given.
func TestSearch_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "doc-001", "First doc", []string{"first"}, "sess-1")
	writeDoc(t, dir, "doc-002", "Second doc", []string{"second"}, "sess-1")
	writeDoc(t, dir, "doc-003", "Third doc", []string{"third"}, "sess-1")

	results, err := search.Search(dir, search.SearchOptions{Query: "", MaxResults: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results for empty query, got %d", len(results))
	}
	// All should have equal score.
	for _, r := range results {
		if r.Score != 1.0 {
			t.Errorf("expected score 1.0 for empty query, got %f for %q", r.Score, r.ID)
		}
	}
}

// TestSearch_SessionFilter verifies session_id filtering.
func TestSearch_SessionFilter(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "doc-001", "Doc from session A", []string{"session-a"}, "sess-A")
	writeDoc(t, dir, "doc-002", "Doc from session B", []string{"session-b"}, "sess-B")
	writeDoc(t, dir, "doc-003", "Another doc from session A", []string{"session-a-2"}, "sess-A")

	results, err := search.Search(dir, search.SearchOptions{
		Query:     "",
		MaxResults: 10,
		SessionID: "sess-A",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for sess-A, got %d", len(results))
	}
	for _, r := range results {
		if r.SourceSession != "sess-A" {
			t.Errorf("result %q has wrong session %q", r.ID, r.SourceSession)
		}
	}
}

// TestSearch_MaxResults respects the MaxResults limit.
func TestSearch_MaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeDoc(t, dir, "doc-"+string(rune('a'+i)), "keyword ranking doc", []string{"keyword"}, "sess-1")
	}

	results, err := search.Search(dir, search.SearchOptions{Query: "keyword", MaxResults: 3})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

// TestSearch_EmptyDir returns nil for an empty directory.
func TestSearch_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	results, err := search.Search(dir, search.SearchOptions{Query: "anything", MaxResults: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty dir, got %d", len(results))
	}
}

// TestSearch_DefaultMaxResults verifies MaxResults defaults to 5.
func TestSearch_DefaultMaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeDoc(t, dir, "doc-"+string(rune('a'+i)), "memory search keyword", []string{"memory"}, "sess-1")
	}
	results, err := search.Search(dir, search.SearchOptions{Query: "memory"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("expected at most 5 results (default), got %d", len(results))
	}
}

// TestSearch_ExcludeDrafts verifies that ExcludeDrafts filters out docs with
// promotion_state == "draft" (#147).
func TestSearch_ExcludeDrafts(t *testing.T) {
	dir := t.TempDir()

	// writeDocWithState writes a minimal doc with a specific promotion_state.
	writeDocWithState := func(id, summary, state string) {
		t.Helper()
		data := []byte(`{"id":"` + id + `","tags":["arch"],"summary":"` + summary +
			`","promotion_state":"` + state + `","created":"2026-01-01T00:00:00Z","content":{"detail":"` + summary + `"}}`)
		if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0644); err != nil {
			t.Fatalf("write doc: %v", err)
		}
	}

	writeDocWithState("curated-doc", "Curated memory about architecture", "curated")
	writeDocWithState("draft-doc", "Draft memory about architecture", "draft")

	// With ExcludeDrafts=true, draft-doc should not appear (empty query returns all non-draft).
	results, err := search.Search(dir, search.SearchOptions{
		Query:         "",
		MaxResults:    10,
		ExcludeDrafts: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	for _, r := range results {
		if r.ID == "draft-doc" {
			t.Errorf("draft-doc appeared in results with ExcludeDrafts=true")
		}
	}
	if len(results) == 0 {
		t.Error("expected at least curated-doc in results")
	}

	// Without ExcludeDrafts, both should appear.
	all, err := search.Search(dir, search.SearchOptions{
		Query:      "",
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("expected at least 2 results without filter, got %d", len(all))
	}
}

// TestBM25Index_Score verifies the exported BM25Index.Score works correctly.
func TestBM25Index_Score(t *testing.T) {
	vocab := []string{
		"bm25 ranking search",
		"rake keyword extraction",
		"memory documents storage",
		"session recording pipeline",
		"tag vocabulary index",
	}
	idx := search.NewBM25Index(vocab)

	// Score a matching query.
	score := idx.Score("bm25 ranking", search.TokenizeBM25("bm25 ranking search"))
	if score <= 0 {
		t.Errorf("expected positive score for matching query, got %f", score)
	}

	// Non-matching doc should score lower than matching.
	lowScore := idx.Score("bm25 ranking", search.TokenizeBM25("session recording pipeline"))
	if lowScore >= score {
		t.Errorf("non-matching doc scored higher or equal (%f >= %f)", lowScore, score)
	}

	// Empty vocab index returns 0.
	emptyIdx := search.NewBM25Index(nil)
	if s := emptyIdx.Score("anything", []string{"word"}); s != 0 {
		t.Errorf("expected 0 from empty index, got %f", s)
	}
}
