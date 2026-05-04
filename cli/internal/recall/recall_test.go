package recall_test

import (
	"path/filepath"
	"testing"

	"github.com/momhq/mom/cli/internal/recall"
	"github.com/momhq/mom/cli/internal/store"
	"github.com/momhq/mom/cli/internal/vault"
)

// newEngine opens a fresh Vault in a temp dir and returns an Engine
// backed by it, plus the MemoryStore + GraphStore for test setup.
func newEngine(t *testing.T) (*recall.Engine, *store.MemoryStore, *store.GraphStore, *vault.Vault) {
	t.Helper()
	dir := t.TempDir()
	v, err := vault.Open(filepath.Join(dir, "mom.db"))
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return recall.NewEngine(v), store.NewMemoryStore(v), store.NewGraphStore(v), v
}

// T1 (tracer bullet): an inserted memory whose content matches the
// query keyword is returned by Engine.Search.
func TestEngine_Search_FindsByContentKeyword(t *testing.T) {
	e, ms, _, _ := newEngine(t)

	mem, err := ms.Insert(store.Memory{
		Type:                   "semantic",
		Summary:                "deploy procedure",
		Content:                map[string]any{"text": "the quick brown fox jumps over the lazy dog"},
		SessionID:              "s1",
		ProvenanceActor:        "claude-code",
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
		PromotionState:         "curated",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	results, err := e.Search(recall.Options{Query: "quick"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result for 'quick', got none")
	}
	found := false
	for _, r := range results {
		if r.ID == mem.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected result for memory %s, got %v", mem.ID, results)
	}
}

// insertCurated is a fixture helper that inserts a curated memory
// with the given summary + content text, returning its ID.
func insertCurated(t *testing.T, ms *store.MemoryStore, summary, contentText string) string {
	t.Helper()
	mem, err := ms.Insert(store.Memory{
		Type:                   "semantic",
		Summary:                summary,
		Content:                map[string]any{"text": contentText},
		SessionID:              "fixture-session",
		ProvenanceActor:        "test",
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
		PromotionState:         "curated",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	return mem.ID
}

// T2: when the AND form of a multi-token query yields no results,
// the engine falls back to the OR form. ADR 0008 contract.
func TestEngine_Search_FallsBackToOR_WhenANDMisses(t *testing.T) {
	e, ms, _, _ := newEngine(t)

	id := insertCurated(t, ms, "partial match memory", "alpha beta")
	// Query has "alpha" (matches) and "gamma" (does not). AND requires
	// both → 0 results. OR fallback should still find this memory.
	results, err := e.Search(recall.Options{Query: "alpha gamma"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected OR fallback to find memory matching 'alpha', got 0 results")
	}
	found := false
	for _, r := range results {
		if r.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected memory %s in results, got %v", id, results)
	}
}

// insertDraft inserts a memory in 'draft' promotion_state.
func insertDraft(t *testing.T, ms *store.MemoryStore, summary, contentText string) string {
	t.Helper()
	mem, err := ms.Insert(store.Memory{
		Summary:                summary,
		Content:                map[string]any{"text": contentText},
		SessionID:              "fixture-session",
		ProvenanceActor:        "test",
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
		// PromotionState defaults to "draft" via Insert.
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	return mem.ID
}

// T3: when the curated tier returns enough results to meet the
// escalation threshold, the draft tier is not searched. The contract
// is "prefer curated; only fall through to drafts if needed". ADR
// 0006 quality dimension.
func TestEngine_Search_PrefersCuratedTier_WhenThresholdMet(t *testing.T) {
	e, ms, _, _ := newEngine(t)

	// 3 curated memories matching "test" — meets the threshold of 3.
	for i := 0; i < 3; i++ {
		insertCurated(t, ms, "curated", "test memory body")
	}
	// 1 draft also matching "test". Should NOT appear in results.
	draftID := insertDraft(t, ms, "draft", "test memory body")

	results, err := e.Search(recall.Options{Query: "test", MaxResults: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected curated results, got 0")
	}
	for _, r := range results {
		if r.ID == draftID {
			t.Errorf("draft memory %s should not appear when curated tier met threshold", draftID)
		}
		if r.PromotionState != "curated" {
			t.Errorf("expected only curated results, got %q for %s", r.PromotionState, r.ID)
		}
	}
}

// T4: tag filter narrows results to memories tagged with ALL the
// requested tags (AND logic). Implemented as a SQL join through
// memory_tags + tags, not a denormalised scan. ADR 0010.
func TestEngine_Search_TagFilter(t *testing.T) {
	e, ms, gs, _ := newEngine(t)

	idAlpha := insertCurated(t, ms, "alpha tagged", "test body alpha")
	idBeta := insertCurated(t, ms, "beta tagged", "test body beta")

	tagAlpha, _ := gs.UpsertTag("alpha")
	tagBeta, _ := gs.UpsertTag("beta")
	if err := gs.LinkTag(idAlpha, tagAlpha); err != nil {
		t.Fatal(err)
	}
	if err := gs.LinkTag(idBeta, tagBeta); err != nil {
		t.Fatal(err)
	}

	results, err := e.Search(recall.Options{Query: "test", Tags: []string{"alpha"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for tag=alpha, got %d: %v", len(results), results)
	}
	if results[0].ID != idAlpha {
		t.Errorf("expected memory %s (alpha-tagged), got %s", idAlpha, results[0].ID)
	}
}

// T5: bm25 column weights from ADR 0007 give content_text 5× the
// weight of summary. A memory whose content matches a query token
// ranks higher than a memory whose only match is in the summary.
func TestEngine_Search_ContentOutranksSummary(t *testing.T) {
	e, ms, _, _ := newEngine(t)

	summaryOnlyID := insertCurated(t, ms, "alpha is the topic here", "unrelated body text")
	contentMatchID := insertCurated(t, ms, "different summary", "the body talks about alpha here")

	results, err := e.Search(recall.Options{Query: "alpha"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected both memories returned, got %d: %v", len(results), results)
	}

	// Lower bm25 score (more negative) = better in SQLite.
	if results[0].ID != contentMatchID {
		t.Errorf("expected content-match (%s) to rank first, got order: %s, %s",
			contentMatchID, results[0].ID, results[1].ID)
	}
	_ = summaryOnlyID
}
