package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/momhq/mom/cli/internal/store"
	"github.com/momhq/mom/cli/internal/vault"
)

// newStore opens a fresh Vault in a temp dir and returns a MemoryStore
// + GraphStore wired to it. Cleanup is registered via t.Cleanup.
func newStore(t *testing.T) (*store.MemoryStore, *store.GraphStore, *vault.Vault) {
	t.Helper()
	dir := t.TempDir()
	v, err := vault.Open(filepath.Join(dir, "mom.db"))
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return store.NewMemoryStore(v), store.NewGraphStore(v), v
}

// T1 (tracer bullet): Insert a memory with substance + operational
// fields, Get it back, verify the substance round-trips.
func TestMemoryStore_InsertAndGetRoundTrip(t *testing.T) {
	ms, _, _ := newStore(t)

	in := store.Memory{
		Type:                   "semantic",
		Summary:                "test summary",
		Content:                map[string]any{"text": "test body"},
		SessionID:              "session-1",
		ProvenanceActor:        "claude-code",
		ProvenanceSourceType:   "transcript-extraction",
		ProvenanceTriggerEvent: "session-end",
	}

	inserted, err := ms.Insert(in)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if inserted.ID == "" {
		t.Fatalf("expected Insert to mint an ID")
	}

	got, err := ms.Get(inserted.ID)
	if err != nil {
		t.Fatalf("Get(%s): %v", inserted.ID, err)
	}

	if got.ID != inserted.ID {
		t.Errorf("ID: got %q, want %q", got.ID, inserted.ID)
	}
	if got.Type != "semantic" {
		t.Errorf("Type: got %q, want %q", got.Type, "semantic")
	}
	if got.Summary != in.Summary {
		t.Errorf("Summary: got %q, want %q", got.Summary, in.Summary)
	}
	if got.SessionID != in.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, in.SessionID)
	}
	if got.ProvenanceActor != in.ProvenanceActor {
		t.Errorf("ProvenanceActor: got %q, want %q", got.ProvenanceActor, in.ProvenanceActor)
	}
	if text, _ := got.Content["text"].(string); text != "test body" {
		t.Errorf("Content.text: got %v, want %q", got.Content["text"], "test body")
	}
}

// T2: When the caller provides no ID, Insert mints a UUID v4. Locks
// in ADR 0013 — IDs are opaque UUIDs, not slugs or session-derived.
func TestMemoryStore_Insert_MintsUUIDv4WhenIDEmpty(t *testing.T) {
	ms, _, _ := newStore(t)

	inserted, err := ms.Insert(store.Memory{
		Content: map[string]any{"text": "x"},
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if inserted.ID == "" {
		t.Fatalf("expected ID to be minted, got empty")
	}
	parsed, err := uuid.Parse(inserted.ID)
	if err != nil {
		t.Fatalf("expected valid UUID, got %q: %v", inserted.ID, err)
	}
	if got := parsed.Version(); got != 4 {
		t.Errorf("expected UUIDv4, got version %d", got)
	}
}

// T3+T4: When fields are zero, Insert applies sensible defaults —
// Type="untyped" (ADR 0012), PromotionState="draft", CreatedAt=now.
// All defaults persist through Get.
func TestMemoryStore_Insert_AppliesDefaults(t *testing.T) {
	ms, _, _ := newStore(t)

	before := time.Now().UTC()
	inserted, err := ms.Insert(store.Memory{
		Content: map[string]any{"text": "x"},
	})
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if inserted.Type != "untyped" {
		t.Errorf("Type default: got %q, want %q", inserted.Type, "untyped")
	}
	if inserted.PromotionState != "draft" {
		t.Errorf("PromotionState default: got %q, want %q", inserted.PromotionState, "draft")
	}
	if inserted.CreatedAt.Before(before) || inserted.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v outside expected range [%v, %v]", inserted.CreatedAt, before, after)
	}

	got, err := ms.Get(inserted.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != "untyped" {
		t.Errorf("Type after Get: got %q, want %q", got.Type, "untyped")
	}
	if got.PromotionState != "draft" {
		t.Errorf("PromotionState after Get: got %q, want %q", got.PromotionState, "draft")
	}
}

// T8: Get returns ErrNotFound when the ID does not exist. Sentinel
// error so callers can distinguish "missing" from other failures.
func TestMemoryStore_Get_ReturnsErrNotFoundForUnknownID(t *testing.T) {
	ms, _, _ := newStore(t)

	_, err := ms.Get("00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// insertedFixture is a helper that inserts a memory with known
// substance fields so operational-setter tests can verify substance
// remains untouched.
func insertedFixture(t *testing.T, ms *store.MemoryStore) store.Memory {
	t.Helper()
	in, err := ms.Insert(store.Memory{
		Type:                   "untyped",
		Summary:                "original summary",
		Content:                map[string]any{"text": "original body"},
		SessionID:              "session-1",
		ProvenanceActor:        "claude-code",
		ProvenanceSourceType:   "transcript-extraction",
		ProvenanceTriggerEvent: "session-end",
	})
	if err != nil {
		t.Fatalf("fixture Insert: %v", err)
	}
	return in
}

// assertSubstanceUnchanged verifies all substance fields on got match
// the fixture. Callers run after a Set* method to prove the operational
// mutation did not bleed into substance.
func assertSubstanceUnchanged(t *testing.T, got, fixture store.Memory) {
	t.Helper()
	if got.ID != fixture.ID {
		t.Errorf("ID changed: %q -> %q", fixture.ID, got.ID)
	}
	if got.Summary != fixture.Summary {
		t.Errorf("Summary changed: %q -> %q", fixture.Summary, got.Summary)
	}
	if text, _ := got.Content["text"].(string); text != "original body" {
		t.Errorf("Content.text changed: %v", got.Content["text"])
	}
	if !got.CreatedAt.Equal(fixture.CreatedAt) {
		t.Errorf("CreatedAt changed: %v -> %v", fixture.CreatedAt, got.CreatedAt)
	}
	if got.SessionID != fixture.SessionID {
		t.Errorf("SessionID changed: %q -> %q", fixture.SessionID, got.SessionID)
	}
	if got.ProvenanceActor != fixture.ProvenanceActor {
		t.Errorf("ProvenanceActor changed: %q -> %q", fixture.ProvenanceActor, got.ProvenanceActor)
	}
	if got.ProvenanceSourceType != fixture.ProvenanceSourceType {
		t.Errorf("ProvenanceSourceType changed: %q -> %q", fixture.ProvenanceSourceType, got.ProvenanceSourceType)
	}
	if got.ProvenanceTriggerEvent != fixture.ProvenanceTriggerEvent {
		t.Errorf("ProvenanceTriggerEvent changed: %q -> %q", fixture.ProvenanceTriggerEvent, got.ProvenanceTriggerEvent)
	}
}

// T5: SetType mutates only the type column. Substance fields remain
// untouched (ADR 0011 — operational metadata mutable, substance
// immutable).
func TestMemoryStore_SetType(t *testing.T) {
	ms, _, _ := newStore(t)
	fixture := insertedFixture(t, ms)

	if err := ms.SetType(fixture.ID, "semantic"); err != nil {
		t.Fatalf("SetType: %v", err)
	}

	got, err := ms.Get(fixture.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != "semantic" {
		t.Errorf("Type: got %q, want %q", got.Type, "semantic")
	}
	assertSubstanceUnchanged(t, got, fixture)
}

// T6: SetPromotionState mutates only the promotion_state column.
func TestMemoryStore_SetPromotionState(t *testing.T) {
	ms, _, _ := newStore(t)
	fixture := insertedFixture(t, ms)

	if err := ms.SetPromotionState(fixture.ID, "curated"); err != nil {
		t.Fatalf("SetPromotionState: %v", err)
	}

	got, _ := ms.Get(fixture.ID)
	if got.PromotionState != "curated" {
		t.Errorf("PromotionState: got %q, want %q", got.PromotionState, "curated")
	}
	assertSubstanceUnchanged(t, got, fixture)
}

// T7: SetLandmark and SetCentralityScore mutate only their respective
// columns. SetCentralityScore uses *float64 — nil sets the column to
// NULL (the default state for non-landmark memories); a value pointer
// sets a numeric centrality.
func TestMemoryStore_SetLandmarkAndCentralityScore(t *testing.T) {
	ms, _, _ := newStore(t)
	fixture := insertedFixture(t, ms)

	if err := ms.SetLandmark(fixture.ID, true); err != nil {
		t.Fatalf("SetLandmark true: %v", err)
	}

	score := 0.87
	if err := ms.SetCentralityScore(fixture.ID, &score); err != nil {
		t.Fatalf("SetCentralityScore: %v", err)
	}

	got, _ := ms.Get(fixture.ID)
	if !got.Landmark {
		t.Errorf("Landmark: got false, want true")
	}
	if got.CentralityScore == nil || *got.CentralityScore != 0.87 {
		t.Errorf("CentralityScore: got %v, want 0.87", got.CentralityScore)
	}
	assertSubstanceUnchanged(t, got, fixture)

	// Clearing centrality back to NULL (e.g. demote from landmark)
	if err := ms.SetCentralityScore(fixture.ID, nil); err != nil {
		t.Fatalf("SetCentralityScore nil: %v", err)
	}
	if err := ms.SetLandmark(fixture.ID, false); err != nil {
		t.Fatalf("SetLandmark false: %v", err)
	}
	got2, _ := ms.Get(fixture.ID)
	if got2.Landmark {
		t.Errorf("Landmark after clear: got true, want false")
	}
	if got2.CentralityScore != nil {
		t.Errorf("CentralityScore after clear: got %v, want nil", got2.CentralityScore)
	}
	assertSubstanceUnchanged(t, got2, fixture)
}

// T9: UpsertTag returns the same ID on re-call with the same name.
// Tag identity is by name; second upsert is a no-op (no new row).
func TestGraphStore_UpsertTag_Idempotent(t *testing.T) {
	_, gs, _ := newStore(t)

	id1, err := gs.UpsertTag("recall")
	if err != nil {
		t.Fatalf("first UpsertTag: %v", err)
	}
	if id1 == "" {
		t.Fatalf("expected non-empty tag ID")
	}

	id2, err := gs.UpsertTag("recall")
	if err != nil {
		t.Fatalf("second UpsertTag: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected same tag ID on re-upsert: %q -> %q", id1, id2)
	}

	// A different name produces a different ID.
	id3, err := gs.UpsertTag("graph")
	if err != nil {
		t.Fatalf("UpsertTag graph: %v", err)
	}
	if id3 == id1 {
		t.Errorf("different names should have different IDs")
	}
}

// T10: LinkTag connects a memory to a tag; MemoriesByTag returns the
// memory IDs for a given tag name. Tag-based recall uses joins on
// memory_tags (ADR 0010), not denormalized arrays.
func TestGraphStore_LinkTag_MemoriesByTag(t *testing.T) {
	ms, gs, _ := newStore(t)

	m, err := ms.Insert(store.Memory{Content: map[string]any{"text": "x"}})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	tagID, err := gs.UpsertTag("recall")
	if err != nil {
		t.Fatalf("UpsertTag: %v", err)
	}

	if err := gs.LinkTag(m.ID, tagID); err != nil {
		t.Fatalf("LinkTag: %v", err)
	}

	ids, err := gs.MemoriesByTag("recall")
	if err != nil {
		t.Fatalf("MemoriesByTag: %v", err)
	}
	if len(ids) != 1 || ids[0] != m.ID {
		t.Errorf("MemoriesByTag(recall): got %v, want [%q]", ids, m.ID)
	}

	none, err := gs.MemoriesByTag("never-used")
	if err != nil {
		t.Fatalf("MemoriesByTag never-used: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected empty result for unknown tag, got %v", none)
	}
}

// T11: UpsertEntity is idempotent on (type, display_name); LinkEntity
// connects a memory to an entity with a relationship label
// ("created_by" is the v0.30 case per ADR 0010).
func TestGraphStore_UpsertEntity_LinkEntity(t *testing.T) {
	ms, gs, v := newStore(t)

	id1, err := gs.UpsertEntity("user", "Vinicius")
	if err != nil {
		t.Fatalf("UpsertEntity 1: %v", err)
	}
	id2, err := gs.UpsertEntity("user", "Vinicius")
	if err != nil {
		t.Fatalf("UpsertEntity 2: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected idempotent UpsertEntity, got %q vs %q", id1, id2)
	}

	idOther, err := gs.UpsertEntity("user", "Other")
	if err != nil {
		t.Fatalf("UpsertEntity Other: %v", err)
	}
	if idOther == id1 {
		t.Errorf("different display_name should produce different entity ID")
	}

	m, err := ms.Insert(store.Memory{Content: map[string]any{"text": "x"}})
	if err != nil {
		t.Fatalf("Insert memory: %v", err)
	}

	if err := gs.LinkEntity(m.ID, id1, "created_by"); err != nil {
		t.Fatalf("LinkEntity: %v", err)
	}

	var gotRel string
	err = v.Query(
		`SELECT relationship FROM memory_entities WHERE memory_id = ? AND entity_id = ?`,
		[]any{m.ID, id1},
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&gotRel)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("query memory_entities: %v", err)
	}
	if gotRel != "created_by" {
		t.Errorf("relationship: got %q, want %q", gotRel, "created_by")
	}
}

// T12: RenameTag changes only tags.name. memory_tags rows continue to
// reference the same tag.id (no rewrite), and memory substance is
// untouched. ADR 0010 — graph-level operation.
func TestGraphStore_RenameTag(t *testing.T) {
	ms, gs, v := newStore(t)
	fixture := insertedFixture(t, ms)

	tagID, err := gs.UpsertTag("mcp")
	if err != nil {
		t.Fatalf("UpsertTag: %v", err)
	}
	if err := gs.LinkTag(fixture.ID, tagID); err != nil {
		t.Fatalf("LinkTag: %v", err)
	}

	if err := gs.RenameTag("mcp", "MCP"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}

	// Tag row's name is updated; ID is preserved.
	var name string
	err = v.Query(
		`SELECT name FROM tags WHERE id = ?`,
		[]any{tagID},
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&name)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	if name != "MCP" {
		t.Errorf("tag name: got %q, want %q", name, "MCP")
	}

	// Lookup by new name still returns the same memory (memory_tags
	// rows were not rewritten).
	ids, err := gs.MemoriesByTag("MCP")
	if err != nil {
		t.Fatalf("MemoriesByTag MCP: %v", err)
	}
	if len(ids) != 1 || ids[0] != fixture.ID {
		t.Errorf("MemoriesByTag(MCP): got %v, want [%q]", ids, fixture.ID)
	}

	// Memory substance is unchanged.
	got, err := ms.Get(fixture.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertSubstanceUnchanged(t, got, fixture)
}

// T13: MergeTags repoints memory_tags edges from source to target,
// drops the source tag row, and leaves memory substance untouched.
// Handles the case where a memory is already linked to both source
// and target without creating a duplicate edge.
func TestGraphStore_MergeTags(t *testing.T) {
	ms, gs, v := newStore(t)

	fixture := insertedFixture(t, ms)

	m2, err := ms.Insert(store.Memory{Content: map[string]any{"text": "y"}})
	if err != nil {
		t.Fatalf("Insert m2: %v", err)
	}

	srcID, err := gs.UpsertTag("foo")
	if err != nil {
		t.Fatalf("UpsertTag foo: %v", err)
	}
	tgtID, err := gs.UpsertTag("bar")
	if err != nil {
		t.Fatalf("UpsertTag bar: %v", err)
	}
	if err := gs.LinkTag(fixture.ID, srcID); err != nil {
		t.Fatalf("LinkTag fixture->foo: %v", err)
	}
	if err := gs.LinkTag(m2.ID, tgtID); err != nil {
		t.Fatalf("LinkTag m2->bar: %v", err)
	}
	// fixture is also already on bar to prove duplicate handling.
	if err := gs.LinkTag(fixture.ID, tgtID); err != nil {
		t.Fatalf("LinkTag fixture->bar: %v", err)
	}

	if err := gs.MergeTags("foo", "bar"); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	// Source tag is gone.
	var srcCount int
	err = v.Query(
		`SELECT COUNT(*) FROM tags WHERE id = ?`,
		[]any{srcID},
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&srcCount)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("query tags count: %v", err)
	}
	if srcCount != 0 {
		t.Errorf("expected source tag deleted, found %d", srcCount)
	}

	// Both memories now reachable via target tag.
	ids, err := gs.MemoriesByTag("bar")
	if err != nil {
		t.Fatalf("MemoriesByTag bar: %v", err)
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet[fixture.ID] || !idSet[m2.ID] || len(ids) != 2 {
		t.Errorf("MemoriesByTag(bar): got %v, want both fixture and m2", ids)
	}

	// Memory substance is unchanged.
	gotFixture, err := ms.Get(fixture.ID)
	if err != nil {
		t.Fatalf("Get fixture: %v", err)
	}
	assertSubstanceUnchanged(t, gotFixture, fixture)
}

// T14: MergeTags rejects source == target. Without the guard, a typo
// (or argument swap) would wipe all memory_tags edges for that tag and
// delete the tag itself — irrecoverable data loss from one mistaken
// keystroke.
func TestGraphStore_MergeTags_RejectsSelfMerge(t *testing.T) {
	ms, gs, _ := newStore(t)

	fixture := insertedFixture(t, ms)
	tagID, err := gs.UpsertTag("recall")
	if err != nil {
		t.Fatalf("UpsertTag: %v", err)
	}
	if err := gs.LinkTag(fixture.ID, tagID); err != nil {
		t.Fatalf("LinkTag: %v", err)
	}

	if err := gs.MergeTags("recall", "recall"); err == nil {
		t.Errorf("expected error from MergeTags(x, x), got nil")
	}

	// Tag and link still intact.
	ids, err := gs.MemoriesByTag("recall")
	if err != nil {
		t.Fatalf("MemoriesByTag: %v", err)
	}
	if len(ids) != 1 || ids[0] != fixture.ID {
		t.Errorf("expected link preserved after rejected self-merge, got %v", ids)
	}
}

// T16: The Memory returned by Insert has a CreatedAt that is byte-
// identical to what Get later returns. Without stripping the monotonic
// clock from the stored time, struct equality (==) would silently
// fail even though Equal() succeeds — a foot-gun for any caller that
// uses the returned Memory as a cache key or comparison target.
func TestMemoryStore_Insert_CreatedAtIsByteIdenticalToGet(t *testing.T) {
	ms, _, _ := newStore(t)

	inserted, err := ms.Insert(store.Memory{Content: map[string]any{"text": "x"}})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := ms.Get(inserted.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if inserted.CreatedAt != got.CreatedAt {
		t.Errorf("CreatedAt should be == after round-trip; inserted=%v got=%v",
			inserted.CreatedAt, got.CreatedAt)
	}
}

// T15: Empty names are not meaningful identifiers. UpsertTag and
// UpsertEntity reject empty inputs early so a buggy upstream caller
// doesn't silently create zombie rows.
func TestGraphStore_RejectsEmptyNames(t *testing.T) {
	_, gs, _ := newStore(t)

	if _, err := gs.UpsertTag(""); err == nil {
		t.Errorf("UpsertTag(\"\"): expected error")
	}
	if _, err := gs.UpsertEntity("", "X"); err == nil {
		t.Errorf("UpsertEntity(\"\", \"X\"): expected error for empty type")
	}
	if _, err := gs.UpsertEntity("user", ""); err == nil {
		t.Errorf("UpsertEntity(\"user\", \"\"): expected error for empty display_name")
	}
}
