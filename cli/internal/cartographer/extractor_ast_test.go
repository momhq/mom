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
		{"app.py", true},
		{"server.ts", true},
		{"component.tsx", true},
		{"main.rs", true},
		{"Foo.java", true},
		{"util.rb", true},
		{"parser.c", true},
		{"parser.h", true},
		{"engine.cpp", true},
		{"service.cs", true},
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

// assertLanguageFixture is a helper that loads a fixture file and asserts
// at least one draft with the given symbol name and language tag.
func assertLanguageFixture(t *testing.T, fixture, ext, lang, wantSymbol string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", fixture, err)
	}

	e := NewTreeSitterASTExtractor()
	src := Source{
		Path:      "testdata/" + fixture,
		Content:   data,
		Extension: ext,
	}

	drafts, err := e.Extract(context.Background(), src)
	if err != nil {
		t.Fatalf("Extract(%s): %v", fixture, err)
	}
	if len(drafts) == 0 {
		t.Fatalf("Extract(%s): expected ≥1 draft, got 0", fixture)
	}

	// All drafts must be fact/EXTRACTED and carry the language tag.
	for _, d := range drafts {
		if d.Type != "fact" {
			t.Errorf("%s: draft %q type = %q, want fact", fixture, d.Summary, d.Type)
		}
		if d.Confidence != ConfidenceExtracted {
			t.Errorf("%s: draft %q confidence = %q, want EXTRACTED", fixture, d.Summary, d.Confidence)
		}
		hasLang := false
		for _, tag := range d.Tags {
			if tag == lang {
				hasLang = true
				break
			}
		}
		if !hasLang {
			t.Errorf("%s: draft %q missing language tag %q, tags=%v", fixture, d.Summary, lang, d.Tags)
		}
	}

	// At least one draft must match the expected symbol.
	found := false
	for _, d := range drafts {
		if sym, ok := d.Content["symbol"].(string); ok && sym == wantSymbol {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("%s: no draft with symbol=%q; drafts=%v", fixture, wantSymbol, draftSymbols(drafts))
	}
}

func draftSymbols(drafts []Draft) []string {
	var out []string
	for _, d := range drafts {
		if sym, ok := d.Content["symbol"].(string); ok {
			out = append(out, sym)
		} else if name, ok := d.Content["name"].(string); ok {
			out = append(out, name)
		}
	}
	return out
}

func TestASTExtractor_Python(t *testing.T) {
	assertLanguageFixture(t, "sample.py", ".py", "python", "DataProcessor")
}

func TestASTExtractor_Ruby(t *testing.T) {
	assertLanguageFixture(t, "sample.rb", ".rb", "ruby", "DataProcessor")
}

func TestASTExtractor_JavaScript(t *testing.T) {
	assertLanguageFixture(t, "sample.js", ".js", "javascript", "DataProcessor")
}

func TestASTExtractor_TypeScript(t *testing.T) {
	assertLanguageFixture(t, "sample.ts", ".ts", "typescript", "DataProcessor")
}

func TestASTExtractor_TSX(t *testing.T) {
	assertLanguageFixture(t, "sample.tsx", ".tsx", "tsx", "FormManager")
}

func TestASTExtractor_Rust(t *testing.T) {
	assertLanguageFixture(t, "sample.rs", ".rs", "rust", "DataProcessor")
}

func TestASTExtractor_Java(t *testing.T) {
	assertLanguageFixture(t, "sample.java", ".java", "java", "DataProcessor")
}

func TestASTExtractor_C(t *testing.T) {
	assertLanguageFixture(t, "sample.c", ".c", "c", "load_config")
}

func TestASTExtractor_CPP(t *testing.T) {
	assertLanguageFixture(t, "sample.cpp", ".cpp", "cpp", "DataProcessor")
}

func TestASTExtractor_CSharp(t *testing.T) {
	assertLanguageFixture(t, "sample.cs", ".cs", "csharp", "DataProcessor")
}

func TestASTExtractor_MultiLanguageIntegration(t *testing.T) {
	// Integration test: scan the testdata directory and assert ≥1 draft per language.
	type langCase struct {
		ext  string
		lang string
	}
	cases := []langCase{
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".tsx", "tsx"},
		{".rs", "rust"},
		{".java", "java"},
		{".rb", "ruby"},
		{".c", "c"},
		{".cpp", "cpp"},
		{".cs", "csharp"},
	}

	e := NewTreeSitterASTExtractor()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.lang, func(t *testing.T) {
			// Find a fixture with this extension.
			entries, err := os.ReadDir("testdata")
			if err != nil {
				t.Fatalf("reading testdata: %v", err)
			}

			var data []byte
			var found string
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if filepath.Ext(entry.Name()) == tc.ext {
					data, err = os.ReadFile(filepath.Join("testdata", entry.Name()))
					if err != nil {
						t.Fatalf("reading %s: %v", entry.Name(), err)
					}
					found = entry.Name()
					break
				}
			}
			if found == "" {
				t.Skipf("no fixture for %s", tc.ext)
			}

			src := Source{Path: "testdata/" + found, Content: data, Extension: tc.ext}
			drafts, err := e.Extract(context.Background(), src)
			if err != nil {
				t.Fatalf("Extract(%s): %v", found, err)
			}
			if len(drafts) == 0 {
				t.Errorf("%s (%s): expected ≥1 draft, got 0", found, tc.lang)
			}
		})
	}
}
