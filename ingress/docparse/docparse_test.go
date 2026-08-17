package docparse

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract_DispatchIsCaseInsensitive(t *testing.T) {
	entries := minimalEPUBEntries(`<?xml version="1.0"?>
<package><metadata><dc:title>T</dc:title></metadata>
<manifest><item id="a" href="a.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="a"/></spine></package>`, map[string]string{
		"a.xhtml": chapterDoc("A", "text"),
	})
	path := writeZipFile(t, filepath.Join(t.TempDir(), "book.EPUB"), entries)

	doc, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != "epub" {
		t.Errorf("format = %q, want epub", doc.Format)
	}
}

func TestExtract_UnsupportedFormatError(t *testing.T) {
	cases := []struct {
		path     string
		wantSubs []string
	}{
		{"book.pdf", []string{"pdftotext", "--text", ".txt", ".md", ".html", ".epub", ".docx"}},
		{"book.mobi", []string{"ebook-convert"}},
		{"book.rtf", []string{".txt", ".md", ".html", ".epub", ".docx", "--text"}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.path)
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Extract(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var uf *UnsupportedFormatError
			if !errors.As(err, &uf) {
				t.Fatalf("errors.As failed for %v", err)
			}
			msg := uf.Error()
			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("message missing %q:\n%s", sub, msg)
				}
			}
		})
	}
}

func TestExtractPlain_StdinReader(t *testing.T) {
	r := strings.NewReader("Chapter 1\nHello.\n\nChapter 2\nWorld.\n")
	doc, err := ExtractPlain(r, "Fallback Title")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2: %+v", len(doc.Chapters), doc.Chapters)
	}
	if doc.Format != "text" {
		t.Errorf("format = %q, want text", doc.Format)
	}
}
