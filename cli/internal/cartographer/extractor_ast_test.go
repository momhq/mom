package cartographer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestASTExtractor_Matches(t *testing.T) {
	e := NewTreeSitterASTExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"cmd/server.go", true},
		{"README.md", false},
		{"package.json", false},
		{"app.py", false}, // python not in v0.8.0
	}

	for _, tt := range tests {
		if got := e.Matches(tt.path); got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestASTExtractor_GoFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sample.go"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	e := NewTreeSitterASTExtractor()
	src := Source{
		Path:      "testdata/sample.go",
		Content:   data,
		Extension: ".go",
	}

	drafts, err := e.Extract(context.Background(), src)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(drafts) == 0 {
		t.Fatal("expected at least one AST draft from sample.go")
	}

	// All drafts should be EXTRACTED.
	for _, d := range drafts {
		if d.Confidence != ConfidenceExtracted {
			t.Errorf("AST draft %q has confidence %q, want EXTRACTED", d.Summary, d.Confidence)
		}
		if d.Type != "fact" {
			t.Errorf("AST draft %q has type %q, want fact", d.Summary, d.Type)
		}
	}

	// Count types and functions.
	typeCount := 0
	funcCount := 0
	for _, d := range drafts {
		switch d.Content["kind"] {
		case "type":
			typeCount++
		case "function":
			funcCount++
		}
	}

	// sample.go has Server and Config types.
	if typeCount < 2 {
		t.Errorf("expected >= 2 type drafts, got %d", typeCount)
	}

	// sample.go has NewServer (exported).
	if funcCount < 1 {
		t.Errorf("expected >= 1 exported function draft, got %d", funcCount)
	}

	// Unexported functions should NOT appear.
	for _, d := range drafts {
		if name, ok := d.Content["name"].(string); ok && name == "unexportedFunc" {
			t.Error("unexported function should not produce a draft")
		}
	}
}

func TestASTExtractor_Provenance(t *testing.T) {
	src := Source{
		Path:      "cmd/main.go",
		Content:   []byte("package cmd\n\n// MyFunc does something important.\nfunc MyFunc() {}\n"),
		Extension: ".go",
	}

	e := NewTreeSitterASTExtractor()
	drafts, err := e.Extract(context.Background(), src)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(drafts) == 0 {
		t.Fatal("expected at least one draft")
	}

	for _, d := range drafts {
		if d.Provenance.SourceFile != "cmd/main.go" {
			t.Errorf("Provenance.SourceFile = %q, want cmd/main.go", d.Provenance.SourceFile)
		}
		if d.Provenance.TriggerEvent != TriggerEvent {
			t.Errorf("Provenance.TriggerEvent = %q, want %q", d.Provenance.TriggerEvent, TriggerEvent)
		}
		if d.Provenance.SourceHash == "" {
			t.Error("Provenance.SourceHash must not be empty")
		}
		if d.Provenance.SourceLines == "" {
			t.Error("Provenance.SourceLines must not be empty")
		}
	}
}

func TestASTExtractor_DocComment(t *testing.T) {
	src := Source{
		Path:      "api.go",
		Content:   []byte("package api\n\n// Handler processes incoming HTTP requests.\nfunc Handler() {}\n"),
		Extension: ".go",
	}

	e := NewTreeSitterASTExtractor()
	drafts, err := e.Extract(context.Background(), src)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(drafts) == 0 {
		t.Fatal("expected at least one draft")
	}

	found := false
	for _, d := range drafts {
		if name, ok := d.Content["name"].(string); ok && name == "Handler" {
			if doc, ok := d.Content["doc"].(string); ok && doc != "" {
				found = true
				_ = doc
			}
		}
	}
	if !found {
		t.Error("expected doc comment to be captured for Handler function")
	}
}
