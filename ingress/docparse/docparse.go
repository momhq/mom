// Package docparse extracts plain text and chapter structure from documents.
//
// It is a leaf package (ADR 0017): ingress/ translates external input into
// in-process events, and docparse imports only stdlib + yaml.v3 so it stays
// testable in isolation from the rest of MOM.
package docparse

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Chapter is one unit of a Document's structure.
type Chapter struct {
	Index int    // 1-based, chapter order as the format declares it
	Title string // format-derived heading; "" when the format has none
	Text  string // plain text, unbounded (event-size bounding is the caller's job)
}

// Document is the result of extracting a file.
type Document struct {
	Title    string // format metadata, else input file base name minus extension
	Author   string // format metadata; "" when absent
	Format   string // "text" | "markdown" | "html" | "epub" | "docx"
	Chapters []Chapter
}

// FullText joins chapter text with a blank line.
func (d Document) FullText() string {
	texts := make([]string, len(d.Chapters))
	for i, ch := range d.Chapters {
		texts[i] = ch.Text
	}
	return strings.Join(texts, "\n\n")
}

// UnsupportedFormatError is returned by Extract for a format MOM cannot
// parse. Callers can distinguish it from other errors via errors.As.
type UnsupportedFormatError struct {
	Ext  string
	Path string
}

func (e *UnsupportedFormatError) Error() string {
	switch e.Ext {
	case ".pdf":
		return fmt.Sprintf(`ingest: MOM cannot parse %s natively yet

Native formats: .txt .md .html .epub .docx
Convert first, then ingest the text:

  pdftotext -layout book.pdf book.txt
  mom ingest --text book.txt --title "Book Title" --author "Author Name"`, e.Ext)
	case ".mobi", ".azw", ".azw3":
		return fmt.Sprintf(`ingest: %s is not supported and is not planned

Convert to EPUB with Calibre, then ingest normally:

  ebook-convert book%s book.epub
  mom ingest book.epub --author "Author Name"`, e.Ext, e.Ext)
	default:
		return fmt.Sprintf(`ingest: unsupported format %q

Native formats: .txt .md .html .epub .docx
Anything else: convert to plain text with any tool, then

  mom ingest --text converted.txt --title "..."`, e.Ext)
	}
}

// Extract dispatches on the lowercased file extension.
// Returns *UnsupportedFormatError for a format MOM cannot parse.
func Extract(path string) (Document, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".text":
		return extractText(path)
	case ".md", ".markdown":
		return extractMarkdown(path)
	case ".html", ".htm", ".xhtml":
		return extractHTML(path)
	case ".epub":
		return extractEPUB(path)
	case ".docx":
		return extractDocx(path)
	default:
		return Document{}, &UnsupportedFormatError{Ext: ext, Path: path}
	}
}

// ExtractPlain parses r as UTF-8 text or markdown (the --text / stdin path).
// title seeds Document.Title when the content carries no heading metadata.
func ExtractPlain(r io.Reader, title string) (Document, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return Document{}, err
	}
	text := normalizeNewlines(string(b))
	return Document{
		Title:    title,
		Format:   "text",
		Chapters: SplitFlatText(text),
	}, nil
}
