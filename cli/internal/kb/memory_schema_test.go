package kb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// legacyDocJSON is a memory JSON without any of the new v0.8 fields.
const legacyDocJSON = `{
	"id": "legacy-doc",
	"type": "fact",
	"lifecycle": "state",
	"scope": "project",
	"tags": ["test"],
	"created": "2026-04-13T00:00:00Z",
	"created_by": "owner",
	"updated": "2026-04-13T00:00:00Z",
	"updated_by": "leo",
	"content": {"fact": "legacy memory without new fields"}
}`

// newShapeDocJSON is a memory JSON with all v0.8 fields populated.
const newShapeDocJSON = `{
	"id": "new-shape-doc",
	"type": "fact",
	"lifecycle": "state",
	"scope": "project",
	"tags": ["test"],
	"created": "2026-04-13T00:00:00Z",
	"created_by": "owner",
	"updated": "2026-04-13T00:00:00Z",
	"updated_by": "leo",
	"confidence": "EXTRACTED",
	"promotion_state": "curated",
	"classification": "INTERNAL",
	"compartments": {"project": ["alpha", "beta"], "department": ["engineering"]},
	"provenance": {
		"runtime": "claude-code",
		"session_id": "sess-abc123",
		"trigger_event": "session.end",
		"commit_sha": "deadbeef",
		"raw_exhaust_ref": ".mom/cache/exhaust-abc123.json"
	},
	"landmark": true,
	"centrality_score": 0.85,
	"content": {"fact": "new memory with all fields"}
}`

// TestLegacyDoc_LoadFillsDefaults verifies that a legacy memory file (no new fields)
// loads without error and gets safe defaults applied.
func TestLegacyDoc_LoadFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-doc.json")
	if err := os.WriteFile(path, []byte(legacyDocJSON), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	doc, err := LoadDoc(path)
	if err != nil {
		t.Fatalf("LoadDoc failed: %v", err)
	}

	if doc.Confidence != "INFERRED" {
		t.Errorf("expected default confidence INFERRED, got %q", doc.Confidence)
	}
	if doc.PromotionState != "draft" {
		t.Errorf("expected default promotion_state draft, got %q", doc.PromotionState)
	}
	if doc.Classification != "INTERNAL" {
		t.Errorf("expected default classification INTERNAL, got %q", doc.Classification)
	}
	if doc.Compartments == nil {
		t.Error("expected compartments to be non-nil (empty map), got nil")
	}
	if len(doc.Compartments) != 0 {
		t.Errorf("expected empty compartments, got %v", doc.Compartments)
	}
	if doc.Provenance == nil {
		t.Error("expected provenance to be non-nil empty struct, got nil")
	}
	if doc.Landmark {
		t.Error("expected landmark to be false by default")
	}
	if doc.CentralityScore != nil {
		t.Errorf("expected centrality_score nil by default, got %v", doc.CentralityScore)
	}
}

// TestLegacyDoc_ValidatesCleanly confirms validation passes for legacy-shape docs
// after defaults are applied.
func TestLegacyDoc_ValidatesCleanly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-doc.json")
	if err := os.WriteFile(path, []byte(legacyDocJSON), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	doc, err := LoadDoc(path)
	if err != nil {
		t.Fatalf("LoadDoc failed: %v", err)
	}
	if err := doc.Validate(); err != nil {
		t.Errorf("legacy doc validation failed: %v", err)
	}
}

// TestNewShapeDoc_RoundTrip verifies that a fully-populated new-shape doc
// survives save → load → save without field mutation.
func TestNewShapeDoc_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new-shape-doc.json")
	if err := os.WriteFile(path, []byte(newShapeDocJSON), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// First load.
	doc, err := LoadDoc(path)
	if err != nil {
		t.Fatalf("first LoadDoc failed: %v", err)
	}

	// Persist to a second path.
	path2 := filepath.Join(dir, "round-trip.json")
	if err := SaveDoc(path2, doc); err != nil {
		t.Fatalf("SaveDoc failed: %v", err)
	}

	// Reload from the saved copy.
	doc2, err := LoadDoc(path2)
	if err != nil {
		t.Fatalf("second LoadDoc failed: %v", err)
	}

	if doc2.Confidence != "EXTRACTED" {
		t.Errorf("confidence mismatch: got %q", doc2.Confidence)
	}
	if doc2.PromotionState != "curated" {
		t.Errorf("promotion_state mismatch: got %q", doc2.PromotionState)
	}
	if doc2.Classification != "INTERNAL" {
		t.Errorf("classification mismatch: got %q", doc2.Classification)
	}
	if len(doc2.Compartments["project"]) != 2 {
		t.Errorf("compartments[project] mismatch: got %v", doc2.Compartments["project"])
	}
	if doc2.Provenance == nil || doc2.Provenance.Runtime != "claude-code" {
		t.Errorf("provenance.runtime mismatch: got %v", doc2.Provenance)
	}
	if doc2.Provenance.RawExhaustRef != ".mom/cache/exhaust-abc123.json" {
		t.Errorf("provenance.raw_exhaust_ref mismatch: got %q", doc2.Provenance.RawExhaustRef)
	}
	if !doc2.Landmark {
		t.Error("landmark should be true")
	}
	if doc2.CentralityScore == nil || *doc2.CentralityScore != 0.85 {
		t.Errorf("centrality_score mismatch: got %v", doc2.CentralityScore)
	}
}

// TestValidate_InvalidConfidence rejects unknown confidence values.
func TestValidate_InvalidConfidence(t *testing.T) {
	doc := docWithDefaults()
	doc.Confidence = "RANDOM"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for invalid confidence, got nil")
	}
}

// TestValidate_ValidConfidenceValues accepts all three valid confidence values.
func TestValidate_ValidConfidenceValues(t *testing.T) {
	for _, c := range []string{"EXTRACTED", "INFERRED", "AMBIGUOUS"} {
		t.Run(c, func(t *testing.T) {
			doc := docWithDefaults()
			doc.Confidence = c
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for confidence %q, got: %v", c, err)
			}
		})
	}
}

// TestValidate_InvalidClassification rejects unknown classification values.
func TestValidate_InvalidClassification(t *testing.T) {
	for _, bad := range []string{"SUPER_SECRET", "SECRET", "TOP_SECRET", "public"} {
		t.Run(bad, func(t *testing.T) {
			doc := docWithDefaults()
			doc.Classification = bad
			if err := doc.Validate(); err == nil {
				t.Errorf("expected error for invalid classification %q, got nil", bad)
			}
		})
	}
}

// TestValidate_ValidClassificationValues accepts all three valid values.
func TestValidate_ValidClassificationValues(t *testing.T) {
	for _, c := range []string{"PUBLIC", "INTERNAL", "CONFIDENTIAL"} {
		t.Run(c, func(t *testing.T) {
			doc := docWithDefaults()
			doc.Classification = c
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for classification %q, got: %v", c, err)
			}
		})
	}
}

// TestValidate_InvalidPromotionState rejects unknown promotion states.
func TestValidate_InvalidPromotionState(t *testing.T) {
	doc := docWithDefaults()
	doc.PromotionState = "active" // not in enum
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for invalid promotion_state, got nil")
	}
}

// TestValidate_ValidPromotionStates accepts all four valid states.
func TestValidate_ValidPromotionStates(t *testing.T) {
	for _, s := range []string{"draft", "curated", "validated", "deprecated"} {
		t.Run(s, func(t *testing.T) {
			doc := docWithDefaults()
			doc.PromotionState = s
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for promotion_state %q, got: %v", s, err)
			}
		})
	}
}

// TestValidate_CentralityScoreOutOfRange rejects scores outside 0–1.
func TestValidate_CentralityScoreOutOfRange(t *testing.T) {
	for _, bad := range []float64{-0.01, 1.01, -100, 2.5} {
		t.Run("", func(t *testing.T) {
			doc := docWithDefaults()
			score := bad
			doc.CentralityScore = &score
			if err := doc.Validate(); err == nil {
				t.Errorf("expected error for centrality_score %v, got nil", bad)
			}
		})
	}
}

// TestValidate_CentralityScoreInRange accepts scores within 0–1.
func TestValidate_CentralityScoreInRange(t *testing.T) {
	for _, good := range []float64{0.0, 0.5, 1.0, 0.999} {
		t.Run("", func(t *testing.T) {
			doc := docWithDefaults()
			score := good
			doc.CentralityScore = &score
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for centrality_score %v, got: %v", good, err)
			}
		})
	}
}

// TestValidate_Compartments_CustomerDimensions confirms arbitrary dimension keys are accepted.
func TestValidate_Compartments_CustomerDimensions(t *testing.T) {
	doc := docWithDefaults()
	doc.Compartments = map[string][]string{
		"project":    {"alpha", "beta"},
		"department": {"engineering"},
		"geography":  {"eu-only"},
	}
	if err := doc.Validate(); err != nil {
		t.Errorf("expected valid with customer compartments, got: %v", err)
	}
}

// TestApplyDefaults_Idempotent verifies calling ApplyDefaults twice doesn't change values.
func TestApplyDefaults_Idempotent(t *testing.T) {
	doc := docWithDefaults()
	doc.Confidence = "EXTRACTED"
	doc.PromotionState = "validated"
	doc.Classification = "CONFIDENTIAL"

	doc.ApplyDefaults()
	doc.ApplyDefaults()

	if doc.Confidence != "EXTRACTED" {
		t.Errorf("ApplyDefaults overwrote existing confidence: got %q", doc.Confidence)
	}
	if doc.PromotionState != "validated" {
		t.Errorf("ApplyDefaults overwrote existing promotion_state: got %q", doc.PromotionState)
	}
	if doc.Classification != "CONFIDENTIAL" {
		t.Errorf("ApplyDefaults overwrote existing classification: got %q", doc.Classification)
	}
}

// TestProvenanceRawExhaustRef verifies raw_exhaust_ref lives inside provenance.
func TestProvenanceRawExhaustRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prov-doc.json")
	if err := os.WriteFile(path, []byte(newShapeDocJSON), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	doc, err := LoadDoc(path)
	if err != nil {
		t.Fatalf("LoadDoc failed: %v", err)
	}
	if doc.Provenance == nil {
		t.Fatal("provenance should not be nil")
	}
	if doc.Provenance.RawExhaustRef == "" {
		t.Error("raw_exhaust_ref should be set inside provenance")
	}
}

// findRepoMemoryDir walks up from the current directory looking for .mom/memory.
func findRepoMemoryDir() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, ".mom", "memory")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// TestLiveMemoryFiles verifies all memory files in the repo's own .mom/memory/
// continue to load and validate without error after the schema evolution.
func TestLiveMemoryFiles(t *testing.T) {
	memoryDir, found := findRepoMemoryDir()
	if !found {
		t.Skip("skipping live memory test: .mom/memory not found in any ancestor directory")
	}

	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		t.Skipf("skipping live memory test: cannot read %s: %v", memoryDir, err)
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(memoryDir, e.Name())
			doc, err := LoadDoc(path)
			if err != nil {
				t.Fatalf("LoadDoc failed: %v", err)
			}
			if err := doc.Validate(); err != nil {
				t.Errorf("Validate failed: %v", err)
			}
		})
	}
}

// TestNewDoc_WriteEmitsNewFields ensures new captures emit all applicable fields.
func TestNewDoc_WriteEmitsNewFields(t *testing.T) {
	score := 0.72
	doc := &Doc{
		ID:             "capture-test",
		Type:           "fact",
		Lifecycle:      "state",
		Scope:          "project",
		Tags:           []string{"capture"},
		Created:        time.Now().UTC(),
		CreatedBy:      "claude-code",
		Updated:        time.Now().UTC(),
		UpdatedBy:      "claude-code",
		Confidence:     "INFERRED",
		PromotionState: "draft",
		Classification: "INTERNAL",
		Compartments:   map[string][]string{},
		Provenance: &Provenance{
			Runtime:      "claude-code",
			SessionID:    "sess-xyz",
			TriggerEvent: "session.end",
		},
		Landmark:        false,
		CentralityScore: &score,
		Content:         map[string]any{"fact": "freshly captured fact"},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "capture-test.json")

	if err := SaveDoc(path, doc); err != nil {
		t.Fatalf("SaveDoc failed: %v", err)
	}

	// Verify JSON contains the expected keys.
	data, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing saved JSON: %v", err)
	}

	for _, key := range []string{"confidence", "promotion_state", "classification", "provenance", "centrality_score"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected key %q in saved JSON, not found", key)
		}
	}

	prov, ok := raw["provenance"].(map[string]any)
	if !ok {
		t.Fatal("provenance is not an object")
	}
	if prov["runtime"] != "claude-code" {
		t.Errorf("provenance.runtime mismatch: got %v", prov["runtime"])
	}
}

// TestValidate_PatternType accepts "pattern" as a valid type.
func TestValidate_PatternType(t *testing.T) {
	doc := docWithDefaults()
	doc.Type = "pattern"
	doc.Content = map[string]any{"pattern": "structural pattern observed"}
	if err := doc.Validate(); err != nil {
		t.Errorf("expected valid for type pattern, got: %v", err)
	}
}

// TestValidate_LearningType accepts "learning" as a valid type.
func TestValidate_LearningType(t *testing.T) {
	doc := docWithDefaults()
	doc.Type = "learning"
	doc.Content = map[string]any{"learning": "something learned"}
	if err := doc.Validate(); err != nil {
		t.Errorf("expected valid for type learning, got: %v", err)
	}
}

// TestValidate_AllKnownTypes ensures all documented types pass validation.
func TestValidate_AllKnownTypes(t *testing.T) {
	validTypeContents := map[string]map[string]any{
		"constraint":  {"constraint": "a constraint"},
		"skill":       {"description": "a skill"},
		"identity":    {"what": "identity"},
		"decision":    {"decision": "a decision"},
		"fact":        {"fact": "a fact"},
		"feedback":    {"feedback": "feedback text"},
		"reference":   {"description": "a reference"},
		"session-log": {"session_id": "sess-1"},
		"pattern":     {"pattern": "a pattern"},
		"learning":    {"learning": "a learning"},
	}
	for typ, content := range validTypeContents {
		t.Run(typ, func(t *testing.T) {
			doc := docWithDefaults()
			doc.Type = typ
			doc.Content = content
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for type %q, got: %v", typ, err)
			}
		})
	}
}

// docWithDefaults returns a minimal valid Doc with defaults pre-applied.
func docWithDefaults() *Doc {
	doc := &Doc{
		ID:        "test-doc",
		Type:      "fact",
		Lifecycle: "state",
		Scope:     "project",
		Tags:      []string{"test"},
		Created:   time.Now().UTC(),
		CreatedBy: "owner",
		Updated:   time.Now().UTC(),
		UpdatedBy: "leo",
		Content:   map[string]any{"fact": "a fact"},
	}
	doc.ApplyDefaults()
	return doc
}
