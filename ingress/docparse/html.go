package docparse

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// xmlText walks decoder tokens, concatenating character data while dropping
// the content of <script> and <style> elements. It stops at io.EOF.
func xmlText(d *xml.Decoder) string {
	var sb strings.Builder
	var skipDepth int
	var skipTag string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if skipDepth > 0 {
				if name == skipTag {
					skipDepth++
				}
				continue
			}
			if name == "script" || name == "style" {
				skipDepth = 1
				skipTag = name
				continue
			}
		case xml.EndElement:
			if skipDepth > 0 {
				if strings.ToLower(t.Name.Local) == skipTag {
					skipDepth--
				}
				continue
			}
		case xml.CharData:
			if skipDepth == 0 {
				sb.Write(t)
				sb.WriteByte(' ')
			}
		}
	}
	return sb.String()
}

func htmlDecoder(r io.Reader) *xml.Decoder {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.AutoClose = xml.HTMLAutoClose
	return d
}

var tagStripRe = regexp.MustCompile(`(?is)<script.*?</script>|<style.*?</style>|<[^>]*>`)

// stripTagsFallback is used when the XML decode fails outright.
func stripTagsFallback(html string) string {
	return tagStripRe.ReplaceAllString(html, " ")
}

// splitHTMLChapters splits html on <h1> (falling back to <h2>) elements,
// using tag depth to find each heading's boundaries.
func splitHTMLChapters(html string, tag string) []Chapter {
	openRe := regexp.MustCompile(`(?is)<` + tag + `[^>]*>`)
	locs := openRe.FindAllStringIndex(html, -1)
	if len(locs) == 0 {
		return nil
	}
	closeRe := regexp.MustCompile(`(?is)</` + tag + `>`)
	chapters := make([]Chapter, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(html)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		segment := html[start:end]

		title := ""
		if cm := closeRe.FindStringIndex(segment); cm != nil {
			headingHTML := segment[loc[1]-start : cm[0]]
			title = strings.TrimSpace(htmlToText(headingHTML))
		}

		text := strings.TrimSpace(htmlToText(segment))
		chapters = append(chapters, Chapter{Index: i + 1, Title: title, Text: text})
	}
	return chapters
}

// htmlToText extracts text from an HTML fragment, falling back to a regex
// tag-strip if the XML decode fails.
func htmlToText(html string) string {
	d := htmlDecoder(strings.NewReader(html))
	text := xmlText(d)
	if strings.TrimSpace(text) == "" && strings.TrimSpace(html) != "" {
		return stripTagsFallback(html)
	}
	return text
}

func extractHTML(path string) (Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	html := normalizeNewlines(string(b))

	chapters := splitHTMLChapters(html, "h1")
	if chapters == nil {
		chapters = splitHTMLChapters(html, "h2")
	}
	if chapters == nil {
		text := strings.TrimSpace(htmlToText(html))
		chapters = []Chapter{{Index: 1, Text: text}}
	}

	return Document{
		Title:    stripExt(filepath.Base(path)),
		Format:   "html",
		Chapters: chapters,
	}, nil
}
