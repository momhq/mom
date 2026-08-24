package docparse

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"path"
	"strings"
)

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubOPF struct {
	Metadata struct {
		Title   string `xml:"title"`
		Creator string `xml:"creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		ItemRefs []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			buf := make([]byte, 0, f.UncompressedSize64)
			b := make([]byte, 32*1024)
			for {
				n, err := rc.Read(b)
				buf = append(buf, b[:n]...)
				if err != nil {
					break
				}
			}
			return buf, nil
		}
	}
	return nil, fmt.Errorf("epub: entry %q not found", name)
}

func extractEPUB(path_ string) (Document, error) {
	zr, err := zip.OpenReader(path_)
	if err != nil {
		return Document{}, err
	}
	defer zr.Close()

	containerXML, err := readZipEntry(&zr.Reader, "META-INF/container.xml")
	if err != nil {
		return Document{}, fmt.Errorf("epub: missing META-INF/container.xml: %w", err)
	}
	var container epubContainer
	if err := xml.Unmarshal(containerXML, &container); err != nil {
		return Document{}, fmt.Errorf("epub: parsing container.xml: %w", err)
	}
	if len(container.Rootfiles) == 0 {
		return Document{}, fmt.Errorf("epub: container.xml has no rootfile")
	}
	opfPath := container.Rootfiles[0].FullPath
	opfDir := path.Dir(opfPath)

	opfXML, err := readZipEntry(&zr.Reader, opfPath)
	if err != nil {
		return Document{}, fmt.Errorf("epub: reading OPF %q: %w", opfPath, err)
	}
	var opf epubOPF
	if err := xml.Unmarshal(opfXML, &opf); err != nil {
		return Document{}, fmt.Errorf("epub: parsing OPF: %w", err)
	}

	type manifestItem struct {
		href       string
		properties string
	}
	manifest := make(map[string]manifestItem, len(opf.Manifest.Items))
	manifestOrder := make([]string, 0, len(opf.Manifest.Items))
	for _, item := range opf.Manifest.Items {
		manifest[item.ID] = manifestItem{href: item.Href, properties: item.Properties}
		manifestOrder = append(manifestOrder, item.ID)
	}

	resolveHref := func(href string) string {
		if opfDir == "." {
			return href
		}
		return path.Join(opfDir, href)
	}

	// Chapter order follows the spine, skipping non-linear itemrefs and nav
	// manifest items. Falls back to manifest order if the spine is empty.
	var ids []string
	for _, ref := range opf.Spine.ItemRefs {
		if ref.Linear == "no" {
			continue
		}
		item, ok := manifest[ref.IDRef]
		if !ok || strings.Contains(item.properties, "nav") {
			continue
		}
		ids = append(ids, ref.IDRef)
	}
	if len(ids) == 0 {
		for _, id := range manifestOrder {
			item := manifest[id]
			if strings.Contains(item.properties, "nav") {
				continue
			}
			ids = append(ids, id)
		}
	}

	chapters := make([]Chapter, 0, len(ids))
	for i, id := range ids {
		item, ok := manifest[id]
		if !ok {
			continue
		}
		docXML, err := readZipEntry(&zr.Reader, resolveHref(item.href))
		if err != nil {
			continue
		}
		text, title := epubDocText(docXML)
		chapters = append(chapters, Chapter{Index: i + 1, Title: title, Text: strings.TrimSpace(text)})
	}

	return Document{
		Title:    opf.Metadata.Title,
		Author:   opf.Metadata.Creator,
		Format:   "epub",
		Chapters: chapters,
	}, nil
}

// epubDocText extracts plain text and a title (from <h1>/<h2>/<title>) from
// one spine XHTML document.
func epubDocText(docXML []byte) (text string, title string) {
	d := htmlDecoder(strings.NewReader(string(docXML)))

	var sb strings.Builder
	var titleFromTag string
	var h1, h2 string
	var captureDepth int
	var captureTag string
	var captureBuf strings.Builder
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
			if captureDepth == 0 && (name == "h1" || name == "h2" || name == "title") {
				captureDepth = 1
				captureTag = name
				captureBuf.Reset()
				continue
			}
			if captureDepth > 0 && name == captureTag {
				captureDepth++
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if skipDepth > 0 {
				if name == skipTag {
					skipDepth--
				}
				continue
			}
			if captureDepth > 0 && name == captureTag {
				captureDepth--
				if captureDepth == 0 {
					val := strings.TrimSpace(captureBuf.String())
					switch captureTag {
					case "h1":
						if h1 == "" {
							h1 = val
						}
					case "h2":
						if h2 == "" {
							h2 = val
						}
					case "title":
						if titleFromTag == "" {
							titleFromTag = val
						}
					}
				}
				continue
			}
		case xml.CharData:
			if skipDepth > 0 {
				continue
			}
			if captureDepth > 0 {
				captureBuf.Write(t)
				continue
			}
			sb.Write(t)
			sb.WriteByte(' ')
		}
	}

	title = h1
	if title == "" {
		title = h2
	}
	if title == "" {
		title = titleFromTag
	}
	return sb.String(), title
}
