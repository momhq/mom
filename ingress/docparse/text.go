package docparse

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// chapterHeadingRe splits flat text into chapters. A heading line looks like
// "Chapter 5" or "Capítulo 5: ..."; when it matches zero times the whole
// document is treated as one chapter.
var chapterHeadingRe = regexp.MustCompile(`(?im)^\s*(chapter|cap[ií]tulo)\s+\d+`)

// SplitFlatText is the heading-regex fallback for formats with no structure.
// Text preceding the first heading (front matter, ToC) is discarded from a
// multi-chapter document; a document with no heading matches becomes one
// chapter holding the whole text.
func SplitFlatText(text string) []Chapter {
	locs := chapterHeadingRe.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return []Chapter{{Index: 1, Text: strings.TrimSpace(text)}}
	}
	chapters := make([]Chapter, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		segment := strings.TrimSpace(text[start:end])
		chapters = append(chapters, Chapter{Index: i + 1, Title: firstLine(segment), Text: segment})
	}
	return chapters
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func stripExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func extractText(path string) (Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := normalizeNewlines(string(b))
	return Document{
		Title:    stripExt(filepath.Base(path)),
		Format:   "text",
		Chapters: SplitFlatText(text),
	}, nil
}
