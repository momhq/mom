package kb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validDoc() *Doc {
	return &Doc{
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
}

func TestValidate_ValidDoc(t *testing.T) {
	doc := validDoc()
	if err := doc.Validate(); err != nil {
		t.Fatalf("expected valid doc, got: %v", err)
	}
}

func TestValidate_InvalidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"uppercase", "InvalidID"},
		{"spaces", "has spaces"},
		{"underscores", "has_underscores"},
		{"empty", ""},
		{"starts with hyphen", "-starts-bad"},
		{"ends with hyphen", "ends-bad-"},
		{"double hyphen", "double--hyphen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validDoc()
			doc.ID = tt.id
			if err := doc.Validate(); err == nil {
				t.Errorf("expected error for id %q, got nil", tt.id)
			}
		})
	}
}

func TestValidate_ValidIDs(t *testing.T) {
	tests := []string{"simple", "kebab-case", "multi-word-id", "has-123-numbers"}
	for _, id := range tests {
		t.Run(id, func(t *testing.T) {
			doc := validDoc()
			doc.ID = id
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for id %q, got: %v", id, err)
			}
		})
	}
}

func TestValidate_InvalidType(t *testing.T) {
	doc := validDoc()
	doc.Type = "invalid-type"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestValidate_AllValidTypes(t *testing.T) {
	types := []string{"constraint", "skill", "identity", "decision", "fact", "feedback", "reference", "metric"}
	for _, tp := range types {
		t.Run(tp, func(t *testing.T) {
			doc := validDoc()
			doc.Type = tp
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for type %q, got: %v", tp, err)
			}
		})
	}
}

func TestValidate_SummaryField(t *testing.T) {
	doc := validDoc()
	doc.Summary = "A concise one-line description"
	if err := doc.Validate(); err != nil {
		t.Fatalf("expected valid doc with summary, got: %v", err)
	}
	if doc.Summary != "A concise one-line description" {
		t.Errorf("expected summary %q, got %q", "A concise one-line description", doc.Summary)
	}
}

func TestValidate_RuleTypeRejected(t *testing.T) {
	doc := validDoc()
	doc.Type = "rule"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for deprecated type 'rule'")
	}
}

func TestValidate_PatternTypeRejected(t *testing.T) {
	doc := validDoc()
	doc.Type = "pattern"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for deprecated type 'pattern'")
	}
}

func TestValidate_InvalidLifecycle(t *testing.T) {
	doc := validDoc()
	doc.Lifecycle = "temporary"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for invalid lifecycle")
	}
}

func TestValidate_AllValidLifecycles(t *testing.T) {
	lifecycles := []string{"permanent", "learning", "state"}
	for _, lc := range lifecycles {
		t.Run(lc, func(t *testing.T) {
			doc := validDoc()
			doc.Lifecycle = lc
			if err := doc.Validate(); err != nil {
				t.Errorf("expected valid for lifecycle %q, got: %v", lc, err)
			}
		})
	}
}

func TestValidate_InvalidScope(t *testing.T) {
	doc := validDoc()
	doc.Scope = "global"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestValidate_EmptyTags(t *testing.T) {
	doc := validDoc()
	doc.Tags = []string{}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for empty tags")
	}
}

func TestValidate_InvalidTagFormat(t *testing.T) {
	doc := validDoc()
	doc.Tags = []string{"valid-tag", "INVALID"}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for invalid tag format")
	}
}

func TestValidate_EmptyCreatedBy(t *testing.T) {
	doc := validDoc()
	doc.CreatedBy = ""
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for empty created_by")
	}
}

func TestValidate_EmptyUpdatedBy(t *testing.T) {
	doc := validDoc()
	doc.UpdatedBy = ""
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for empty updated_by")
	}
}

func TestValidate_NilContent(t *testing.T) {
	doc := validDoc()
	doc.Content = nil
	if err := doc.Validate(); err == nil {
		t.Fatal("expected error for nil content")
	}
}

func TestLoadDoc_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte(`{
		"id": "test-doc",
		"type": "fact",
		"lifecycle": "state",
		"scope": "project",
		"tags": ["test"],
		"created": "2026-04-13T00:00:00Z",
		"created_by": "owner",
		"updated": "2026-04-13T00:00:00Z",
		"updated_by": "leo",
		"content": {"fact": "test"}
	}`), 0644)

	doc, err := LoadDoc(path)
	if err != nil {
		t.Fatalf("LoadDoc failed: %v", err)
	}
	if doc.ID != "test-doc" {
		t.Errorf("expected id %q, got %q", "test-doc", doc.ID)
	}
}

func TestLoadDoc_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte(`{not json}`), 0644)

	if _, err := LoadDoc(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadDoc_FileNotFound(t *testing.T) {
	if _, err := LoadDoc("/nonexistent/path.json"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSaveDoc_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")

	original := validDoc()
	if err := SaveDoc(path, original); err != nil {
		t.Fatalf("SaveDoc failed: %v", err)
	}

	loaded, err := LoadDoc(path)
	if err != nil {
		t.Fatalf("LoadDoc failed: %v", err)
	}

	if loaded.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", original.ID, loaded.ID)
	}
	if loaded.Type != original.Type {
		t.Errorf("Type mismatch: %q vs %q", original.Type, loaded.Type)
	}
	if len(loaded.Tags) != len(original.Tags) {
		t.Errorf("Tags length mismatch: %d vs %d", len(original.Tags), len(loaded.Tags))
	}
}
