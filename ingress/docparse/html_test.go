package docparse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHTMLFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractHTML_StripsTagsAndDecodesEntities(t *testing.T) {
	content := "<html><body><p>Tom &amp; Jerry&nbsp;&#8212; a tale.</p></body></html>"
	path := writeHTMLFile(t, content)
	doc, err := extractHTML(path)
	if err != nil {
		t.Fatal(err)
	}
	full := doc.FullText()
	if strings.Contains(full, "<p>") || strings.Contains(full, "&amp;") {
		t.Errorf("tags/entities not decoded: %q", full)
	}
	if !strings.Contains(full, "Tom & Jerry") {
		t.Errorf("entity decode failed: %q", full)
	}
	if !strings.Contains(full, "—") {
		t.Errorf("em-dash entity not decoded: %q", full)
	}
}

func TestExtractHTML_DropsScriptAndStyle(t *testing.T) {
	content := `<html><head><style>body{color:red}</style></head><body>
<script>alert('hi')</script>
<p>Visible text.</p>
</body></html>`
	path := writeHTMLFile(t, content)
	doc, err := extractHTML(path)
	if err != nil {
		t.Fatal(err)
	}
	full := doc.FullText()
	if strings.Contains(full, "alert") || strings.Contains(full, "color:red") {
		t.Errorf("script/style content leaked: %q", full)
	}
	if !strings.Contains(full, "Visible text.") {
		t.Errorf("visible text missing: %q", full)
	}
}

func TestExtractHTML_ChaptersOnH1(t *testing.T) {
	content := "<html><body><h1>One</h1><p>First.</p><h1>Two</h1><p>Second.</p></body></html>"
	path := writeHTMLFile(t, content)
	doc, err := extractHTML(path)
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

func TestExtractHTML_MalformedMarkupFallsBackToTagStrip(t *testing.T) {
	content := "<html><body><p>Unclosed tag <b>bold text\n<p>More text without closing"
	path := writeHTMLFile(t, content)
	doc, err := extractHTML(path)
	if err != nil {
		t.Fatal(err)
	}
	full := doc.FullText()
	if !strings.Contains(full, "bold text") || !strings.Contains(full, "More text") {
		t.Errorf("fallback tag-strip lost text: %q", full)
	}
}
