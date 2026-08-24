package docparse

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"strings"
)

type docxCoreProps struct {
	Title   string `xml:"title"`
	Creator string `xml:"creator"`
}

type docxParagraph struct {
	Style string
	Text  string
}

// parseDocxParagraphs walks word/document.xml, returning one docxParagraph
// per w:p element: its pStyle (if any) and its concatenated w:t text, with
// w:br as newline and w:tab as tab.
func parseDocxParagraphs(documentXML []byte) []docxParagraph {
	d := xml.NewDecoder(strings.NewReader(string(documentXML)))

	var paragraphs []docxParagraph
	var inParagraph bool
	var style string
	var text strings.Builder
	var inText bool

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inParagraph = true
				style = ""
				text.Reset()
			case "pStyle":
				if inParagraph {
					for _, a := range t.Attr {
						if a.Name.Local == "val" {
							style = a.Value
						}
					}
				}
			case "t":
				inText = true
			case "br":
				if inParagraph {
					text.WriteByte('\n')
				}
			case "tab":
				if inParagraph {
					text.WriteByte('\t')
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if inParagraph {
					paragraphs = append(paragraphs, docxParagraph{Style: style, Text: text.String()})
				}
				inParagraph = false
			case "t":
				inText = false
			}
		case xml.CharData:
			if inParagraph && inText {
				text.Write(t)
			}
		}
	}
	return paragraphs
}

func extractDocx(path string) (Document, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Document{}, err
	}
	defer zr.Close()

	documentXML, err := readZipEntry(&zr.Reader, "word/document.xml")
	if err != nil {
		return Document{}, fmt.Errorf("docx: missing word/document.xml: %w", err)
	}

	var title, author string
	if coreXML, err := readZipEntry(&zr.Reader, "docProps/core.xml"); err == nil {
		var core docxCoreProps
		if err := xml.Unmarshal(coreXML, &core); err == nil {
			title = core.Title
			author = core.Creator
		}
	}

	paragraphs := parseDocxParagraphs(documentXML)
	chapters := splitDocxChapters(paragraphs, "Heading1")
	if chapters == nil {
		chapters = splitDocxChapters(paragraphs, "Heading2")
	}
	if chapters == nil {
		var texts []string
		for _, p := range paragraphs {
			texts = append(texts, p.Text)
		}
		chapters = SplitFlatText(strings.Join(texts, "\n\n"))
	}

	return Document{
		Title:    title,
		Author:   author,
		Format:   "docx",
		Chapters: chapters,
	}, nil
}

// splitDocxChapters splits paragraphs on those whose pStyle matches
// headingStyle, returning nil if no paragraph has that style.
func splitDocxChapters(paragraphs []docxParagraph, headingStyle string) []Chapter {
	var starts []int
	for i, p := range paragraphs {
		if p.Style == headingStyle {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return nil
	}
	chapters := make([]Chapter, 0, len(starts))
	for i, start := range starts {
		end := len(paragraphs)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		var texts []string
		for _, p := range paragraphs[start:end] {
			texts = append(texts, p.Text)
		}
		chapters = append(chapters, Chapter{
			Index: i + 1,
			Title: strings.TrimSpace(paragraphs[start].Text),
			Text:  strings.TrimSpace(strings.Join(texts, "\n\n")),
		})
	}
	return chapters
}
