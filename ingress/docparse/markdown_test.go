package docparse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMarkdownFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractMarkdown_H1Chapters(t *testing.T) {
	path := writeMarkdownFile(t, "# One\nFirst.\n\n# Two\nSecond.\n")
	doc, err := extractMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2: %+v", len(doc.Chapters), doc.Chapters)
	}
	if doc.Chapters[0].Title != "One" || doc.Chapters[1].Title != "Two" {
		t.Errorf("titles = %q, %q", doc.Chapters[0].Title, doc.Chapters[1].Title)
	}
}

func TestExtractMarkdown_FallsBackToH2(t *testing.T) {
	path := writeMarkdownFile(t, "## Alpha\nFirst.\n\n## Beta\nSecond.\n")
	doc, err := extractMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2: %+v", len(doc.Chapters), doc.Chapters)
	}
	if doc.Chapters[0].Title != "Alpha" || doc.Chapters[1].Title != "Beta" {
		t.Errorf("titles = %q, %q", doc.Chapters[0].Title, doc.Chapters[1].Title)
	}
}

func TestExtractMarkdown_IgnoresHashInsideFencedCode(t *testing.T) {
	content := "# Real Heading\n\n```\n# not a heading\n```\n\nBody text.\n"
	path := writeMarkdownFile(t, content)
	doc, err := extractMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1: %+v", len(doc.Chapters), doc.Chapters)
	}
	if doc.Chapters[0].Title != "Real Heading" {
		t.Errorf("title = %q, want %q", doc.Chapters[0].Title, "Real Heading")
	}
	if !strings.Contains(doc.Chapters[0].Text, "# not a heading") {
		t.Errorf("fenced content lost: %q", doc.Chapters[0].Text)
	}
}

func TestExtractMarkdown_ReadsFrontmatter(t *testing.T) {
	content := "---\ntitle: My Book\nauthor: Jane Doe\n---\n# One\nBody.\n"
	path := writeMarkdownFile(t, content)
	doc, err := extractMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "My Book" {
		t.Errorf("title = %q, want %q", doc.Title, "My Book")
	}
	if doc.Author != "Jane Doe" {
		t.Errorf("author = %q, want %q", doc.Author, "Jane Doe")
	}
}

func TestExtractMarkdown_NoHeadingsIsOneChapter(t *testing.T) {
	path := writeMarkdownFile(t, "Just prose.\nMore prose.\n")
	doc, err := extractMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1: %+v", len(doc.Chapters), doc.Chapters)
	}
}
