package docparse

import (
	"path/filepath"
	"strings"
	"testing"
)

const docxNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func docxParagraphXML(style, text string) string {
	styleXML := ""
	if style != "" {
		styleXML = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + styleXML + `<w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func docxDocument(paragraphsXML string) string {
	return `<?xml version="1.0"?>
<w:document ` + docxNS + `><w:body>` + paragraphsXML + `</w:body></w:document>`
}

func TestExtractDocx_SplitsOnHeading1(t *testing.T) {
	body := docxParagraphXML("Heading1", "One") + docxParagraphXML("", "First body.") +
		docxParagraphXML("Heading1", "Two") + docxParagraphXML("", "Second body.")
	entries := map[string]string{"word/document.xml": docxDocument(body)}
	path := writeZipFile(t, filepath.Join(t.TempDir(), "doc.docx"), entries)

	doc, err := extractDocx(path)
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

func TestExtractDocx_FallsBackToHeading2(t *testing.T) {
	body := docxParagraphXML("Heading2", "Alpha") + docxParagraphXML("", "Body.") +
		docxParagraphXML("Heading2", "Beta") + docxParagraphXML("", "Body 2.")
	entries := map[string]string{"word/document.xml": docxDocument(body)}
	path := writeZipFile(t, filepath.Join(t.TempDir(), "doc.docx"), entries)

	doc, err := extractDocx(path)
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

func TestExtractDocx_NoHeadingStylesFallsBackToFlatText(t *testing.T) {
	body := docxParagraphXML("", "Chapter 1") + docxParagraphXML("", "Intro text.") +
		docxParagraphXML("", "Chapter 2") + docxParagraphXML("", "More text.")
	entries := map[string]string{"word/document.xml": docxDocument(body)}
	path := writeZipFile(t, filepath.Join(t.TempDir(), "doc.docx"), entries)

	doc, err := extractDocx(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2 (flat-text fallback): %+v", len(doc.Chapters), doc.Chapters)
	}
}

func TestExtractDocx_ParagraphAndBreakSpacing(t *testing.T) {
	body := `<w:p><w:r><w:t>Line one</w:t></w:r><w:r><w:br/></w:r><w:r><w:t>Line two</w:t></w:r></w:p>`
	entries := map[string]string{"word/document.xml": docxDocument(body)}
	path := writeZipFile(t, filepath.Join(t.TempDir(), "doc.docx"), entries)

	doc, err := extractDocx(path)
	if err != nil {
		t.Fatal(err)
	}
	full := doc.FullText()
	if !strings.Contains(full, "Line one\nLine two") {
		t.Errorf("w:br not rendered as newline: %q", full)
	}
}

func TestExtractDocx_ReadsCoreMetadata(t *testing.T) {
	body := docxParagraphXML("", "Text.")
	core := `<?xml version="1.0"?>
<cp:coreProperties xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title>My Doc</dc:title><dc:creator>Jane Doe</dc:creator>
</cp:coreProperties>`
	entries := map[string]string{
		"word/document.xml": docxDocument(body),
		"docProps/core.xml": core,
	}
	path := writeZipFile(t, filepath.Join(t.TempDir(), "doc.docx"), entries)

	doc, err := extractDocx(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "My Doc" || doc.Author != "Jane Doe" {
		t.Errorf("metadata = %q / %q, want %q / %q", doc.Title, doc.Author, "My Doc", "Jane Doe")
	}
}

func TestExtractDocx_MissingDocumentXMLErrors(t *testing.T) {
	path := writeZipFile(t, filepath.Join(t.TempDir(), "doc.docx"), map[string]string{
		"docProps/core.xml": "<cp:coreProperties/>",
	})
	_, err := extractDocx(path)
	if err == nil {
		t.Fatal("expected error for missing word/document.xml, got nil")
	}
}
