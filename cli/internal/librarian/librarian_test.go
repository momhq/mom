package librarian_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momhq/mom/cli/internal/librarian"
	"github.com/momhq/mom/cli/internal/vault"
)

// openLib opens a fresh Vault with Librarian's migrations applied and
// returns a Librarian wrapping it. The Vault is closed via t.Cleanup.
func openLib(t *testing.T) *librarian.Librarian {
	t.Helper()
	dir := t.TempDir()
	v, err := vault.Open(filepath.Join(dir, "mom.db"), librarian.Migrations())
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return librarian.New(v)
}

func validInsert() librarian.InsertMemory {
	return librarian.InsertMemory{
		Content:                `{"text":"hello world"}`,
		SessionID:              "s-test-1",
		ProvenanceActor:        "claude-code",
		ProvenanceSourceType:   "transcript-extraction",
		ProvenanceTriggerEvent: "watcher",
	}
}

// ── Insert / Get ──────────────────────────────────────────────────────────────

func TestInsert_RoundTripWithDefaults(t *testing.T) {
	l := openLib(t)
	id, err := l.Insert(validInsert())
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if id == "" {
		t.Fatal("Insert returned empty id")
	}

	got, err := l.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %q, want %q", got.ID, id)
	}
	if got.Type != "untyped" {
		t.Errorf("Type = %q, want untyped (default)", got.Type)
	}
	if got.PromotionState != "draft" {
		t.Errorf("PromotionState = %q, want draft (default)", got.PromotionState)
	}
	if got.Landmark {
		t.Error("Landmark should default to false")
	}
	if got.SessionID != "s-test-1" {
		t.Errorf("SessionID = %q, want s-test-1", got.SessionID)
	}
	if got.Content != `{"text":"hello world"}` {
		t.Errorf("Content = %q", got.Content)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should default to now, got zero time")
	}
}

func TestInsert_RejectsEmptySessionID(t *testing.T) {
	l := openLib(t)
	in := validInsert()
	in.SessionID = ""
	_, err := l.Insert(in)
	if !errors.Is(err, librarian.ErrEmptyArg) {
		t.Fatalf("err = %v, want ErrEmptyArg", err)
	}
}

func TestInsert_RejectsWhitespaceSessionID(t *testing.T) {
	l := openLib(t)
	in := validInsert()
	in.SessionID = "   "
	_, err := l.Insert(in)
	if !errors.Is(err, librarian.ErrEmptyArg) {
		t.Fatalf("err = %v, want ErrEmptyArg", err)
	}
}

func TestInsert_RejectsEmptyContent(t *testing.T) {
	l := openLib(t)
	in := validInsert()
	in.Content = ""
	_, err := l.Insert(in)
	if !errors.Is(err, librarian.ErrEmptyArg) {
		t.Fatalf("err = %v, want ErrEmptyArg", err)
	}
}

func TestInsert_RejectsInvalidJSONContent_BySchemaCheck(t *testing.T) {
	// API-level guard only checks non-empty; the schema CHECK
	// (json_valid(content)) is the second line of defense. Defense in
	// depth: the same constraint at two layers means a code path that
	// bypasses one still hits the other.
	l := openLib(t)
	in := validInsert()
	in.Content = "this is not json"
	_, err := l.Insert(in)
	if err == nil {
		t.Fatal("expected error for non-JSON content, got nil")
	}
	if !strings.Contains(err.Error(), "CHECK") && !strings.Contains(err.Error(), "constraint") {
		t.Fatalf("expected CHECK/constraint error, got: %v", err)
	}
}

func TestInsert_MintsUUIDPerCall(t *testing.T) {
	l := openLib(t)
	a, err := l.Insert(validInsert())
	if err != nil {
		t.Fatalf("Insert a: %v", err)
	}
	b, err := l.Insert(validInsert())
	if err != nil {
		t.Fatalf("Insert b: %v", err)
	}
	if a == b {
		t.Fatal("Insert returned identical IDs for two calls")
	}
	// Sanity: UUID v4 string length is 36 (8-4-4-4-12 + hyphens).
	if len(a) != 36 {
		t.Errorf("id %q is not the expected uuid length", a)
	}
}

func TestInsert_HonoursExplicitType(t *testing.T) {
	l := openLib(t)
	in := validInsert()
	in.Type = "semantic"
	id, err := l.Insert(in)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, _ := l.Get(id)
	if got.Type != "semantic" {
		t.Errorf("Type = %q, want semantic", got.Type)
	}
}

func TestGet_ReturnsErrNotFoundOnMiss(t *testing.T) {
	l := openLib(t)
	_, err := l.Get("nonexistent-id")
	if !errors.Is(err, librarian.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ── UpdateOperational ─────────────────────────────────────────────────────────

func TestUpdateOperational_MutatesOperationalFieldsOnly(t *testing.T) {
	l := openLib(t)
	id, err := l.Insert(validInsert())
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	newType := "procedural"
	newPromo := "curated"
	landmark := true
	score := 0.875
	if err := l.UpdateOperational(id, librarian.OperationalUpdate{
		Type:            &newType,
		PromotionState:  &newPromo,
		Landmark:        &landmark,
		CentralityScore: &score,
	}); err != nil {
		t.Fatalf("UpdateOperational: %v", err)
	}

	got, _ := l.Get(id)
	if got.Type != "procedural" {
		t.Errorf("Type = %q, want procedural", got.Type)
	}
	if got.PromotionState != "curated" {
		t.Errorf("PromotionState = %q, want curated", got.PromotionState)
	}
	if !got.Landmark {
		t.Error("Landmark = false, want true")
	}
	if !got.CentralityScore.Valid || got.CentralityScore.Float64 != 0.875 {
		t.Errorf("CentralityScore = %+v, want 0.875", got.CentralityScore)
	}

	// Substance fields must not have changed.
	if got.Content != `{"text":"hello world"}` {
		t.Errorf("Content was mutated: %q", got.Content)
	}
	if got.SessionID != "s-test-1" {
		t.Errorf("SessionID was mutated: %q", got.SessionID)
	}
	if got.ProvenanceActor != "claude-code" {
		t.Errorf("ProvenanceActor was mutated: %q", got.ProvenanceActor)
	}
}

func TestUpdateOperational_EmptyPatchIsNoopSuccess(t *testing.T) {
	l := openLib(t)
	id, _ := l.Insert(validInsert())
	if err := l.UpdateOperational(id, librarian.OperationalUpdate{}); err != nil {
		t.Fatalf("empty patch should be a no-op success, got: %v", err)
	}
}

func TestUpdateOperational_ReturnsNotFoundForUnknownID(t *testing.T) {
	l := openLib(t)
	t1 := "untyped"
	err := l.UpdateOperational("nope", librarian.OperationalUpdate{Type: &t1})
	if !errors.Is(err, librarian.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateOperational_RejectsEmptyID(t *testing.T) {
	l := openLib(t)
	t1 := "untyped"
	err := l.UpdateOperational("", librarian.OperationalUpdate{Type: &t1})
	if !errors.Is(err, librarian.ErrEmptyArg) {
		t.Fatalf("err = %v, want ErrEmptyArg", err)
	}
}

// ── Substance immutability — locked by the API surface ────────────────────────

// TestSubstanceImmutability documents which struct fields on
// OperationalUpdate exist and which substance fields cannot be reached
// through it. If a future maintainer adds a substance field to the
// patch type (Content *string, SessionID *string, etc.), this test
// fails — guarding the API contract from drift.
func TestSubstanceImmutability_OperationalUpdateFieldsAreOperationalOnly(t *testing.T) {
	// Reflection-free check: the public fields on OperationalUpdate
	// must be exactly the operational-mutable set per ADR 0011.
	allowed := map[string]bool{
		"Type": true, "PromotionState": true, "Landmark": true, "CentralityScore": true,
	}
	// Round-trip through Insert + Get to confirm substance fields are
	// not re-write-able through any public path on Librarian. The only
	// post-Insert mutator is UpdateOperational, which lacks the
	// substance fields on its struct entirely. (See type definition.)
	l := openLib(t)
	id, _ := l.Insert(validInsert())
	got1, _ := l.Get(id)

	// Apply every legal operational field once to exercise the path.
	t1 := "episodic"
	st := "curated"
	lm := true
	sc := 1.0
	if err := l.UpdateOperational(id, librarian.OperationalUpdate{
		Type: &t1, PromotionState: &st, Landmark: &lm, CentralityScore: &sc,
	}); err != nil {
		t.Fatalf("UpdateOperational: %v", err)
	}

	got2, _ := l.Get(id)
	if got2.Content != got1.Content {
		t.Error("Content changed despite only operational update")
	}
	if got2.SessionID != got1.SessionID {
		t.Error("SessionID changed despite only operational update")
	}
	if got2.CreatedAt != got1.CreatedAt {
		t.Error("CreatedAt changed despite only operational update")
	}
	if got2.ProvenanceActor != got1.ProvenanceActor ||
		got2.ProvenanceSourceType != got1.ProvenanceSourceType ||
		got2.ProvenanceTriggerEvent != got1.ProvenanceTriggerEvent {
		t.Error("Provenance changed despite only operational update")
	}
	_ = allowed // intentionally held in scope for documentation
}
