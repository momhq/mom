package docparse

import (
	"path/filepath"
	"strings"
	"testing"
)

func minimalEPUBEntries(opf string, docs map[string]string) map[string]string {
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`,
		"OEBPS/content.opf": opf,
	}
	for name, content := range docs {
		entries["OEBPS/"+name] = content
	}
	return entries
}

func chapterDoc(title, body string) string {
	return `<html><body><h1>` + title + `</h1><p>` + body + `</p></body></html>`
}

func TestExtractEPUB_UsesSpineOrderNotManifestOrder(t *testing.T) {
	opf := `<?xml version="1.0"?>
<package>
  <metadata><dc:title>Test Book</dc:title><dc:creator>Test Author</dc:creator></metadata>
  <manifest>
    <item id="c" href="c.xhtml" media-type="application/xhtml+xml"/>
    <item id="a" href="a.xhtml" media-type="application/xhtml+xml"/>
    <item id="b" href="b.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="a"/>
    <itemref idref="b"/>
    <itemref idref="c"/>
  </spine>
</package>`
	entries := minimalEPUBEntries(opf, map[string]string{
		"a.xhtml": chapterDoc("A", "Text A"),
		"b.xhtml": chapterDoc("B", "Text B"),
		"c.xhtml": chapterDoc("C", "Text C"),
	})
	path := writeZipFile(t, filepath.Join(t.TempDir(), "book.epub"), entries)

	doc, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 3 {
		t.Fatalf("got %d chapters, want 3: %+v", len(doc.Chapters), doc.Chapters)
	}
	wantTitles := []string{"A", "B", "C"}
	for i, ch := range doc.Chapters {
		if ch.Index != i+1 {
			t.Errorf("chapter %d index = %d, want %d", i, ch.Index, i+1)
		}
		if ch.Title != wantTitles[i] {
			t.Errorf("chapter %d title = %q, want %q (spine order violated)", i, ch.Title, wantTitles[i])
		}
	}
	if doc.Title != "Test Book" || doc.Author != "Test Author" {
		t.Errorf("metadata = %q / %q, want %q / %q", doc.Title, doc.Author, "Test Book", "Test Author")
	}
}

func TestExtractEPUB_SkipsNonLinearAndNav(t *testing.T) {
	opf := `<?xml version="1.0"?>
<package>
  <metadata><dc:title>T</dc:title><dc:creator>A</dc:creator></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="notes" href="notes.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="nav"/>
    <itemref idref="notes" linear="no"/>
    <itemref idref="ch1"/>
  </spine>
</package>`
	entries := minimalEPUBEntries(opf, map[string]string{
		"nav.xhtml":   chapterDoc("Nav", "nav text"),
		"notes.xhtml": chapterDoc("Notes", "notes text"),
		"ch1.xhtml":   chapterDoc("Chapter 1", "chapter text"),
	})
	path := writeZipFile(t, filepath.Join(t.TempDir(), "book.epub"), entries)

	doc, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1: %+v", len(doc.Chapters), doc.Chapters)
	}
	if doc.Chapters[0].Title != "Chapter 1" {
		t.Errorf("title = %q, want %q", doc.Chapters[0].Title, "Chapter 1")
	}
}

func TestExtractEPUB_ResolvesHrefsRelativeToOPFDir(t *testing.T) {
	opf := `<?xml version="1.0"?>
<package>
  <metadata><dc:title>T</dc:title></metadata>
  <manifest><item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	entries := minimalEPUBEntries(opf, map[string]string{
		"ch1.xhtml": chapterDoc("Chapter 1", "hello"),
	})
	path := writeZipFile(t, filepath.Join(t.TempDir(), "book.epub"), entries)

	doc, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 1 || !strings.Contains(doc.Chapters[0].Text, "hello") {
		t.Fatalf("href not resolved relative to OPF dir: %+v", doc.Chapters)
	}
}

func TestExtractEPUB_MissingContainerXMLErrors(t *testing.T) {
	path := writeZipFile(t, filepath.Join(t.TempDir(), "book.epub"), map[string]string{
		"OEBPS/content.opf": "<package/>",
	})
	_, err := extractEPUB(path)
	if err == nil {
		t.Fatal("expected error for missing container.xml, got nil")
	}
}

func TestExtractEPUB_EmptySpineFallsBackToManifestOrder(t *testing.T) {
	opf := `<?xml version="1.0"?>
<package>
  <metadata><dc:title>T</dc:title></metadata>
  <manifest>
    <item id="a" href="a.xhtml" media-type="application/xhtml+xml"/>
    <item id="b" href="b.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine></spine>
</package>`
	entries := minimalEPUBEntries(opf, map[string]string{
		"a.xhtml": chapterDoc("A", "text a"),
		"b.xhtml": chapterDoc("B", "text b"),
	})
	path := writeZipFile(t, filepath.Join(t.TempDir(), "book.epub"), entries)

	doc, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2: %+v", len(doc.Chapters), doc.Chapters)
	}
	if doc.Chapters[0].Title != "A" || doc.Chapters[1].Title != "B" {
		t.Errorf("titles = %q, %q, want manifest order A, B", doc.Chapters[0].Title, doc.Chapters[1].Title)
	}
}
