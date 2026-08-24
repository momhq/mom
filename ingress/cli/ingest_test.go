package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momhq/mom/ingress/docparse"
	"github.com/momhq/mom/shared/project"
	"github.com/momhq/mom/storage/ledger"
	"github.com/spf13/cobra"
)

func TestWindowOversizedChapters_HeadinglessDocument(t *testing.T) {
	words := make([]string, 0, 3000)
	for i := 0; i < 3000; i++ {
		words = append(words, "word")
	}
	text := strings.Join(words, " ") // ~15000 chars, no heading

	got := windowOversizedChapters(docparse.SplitFlatText(text))
	if len(got) < 2 {
		t.Fatalf("got %d window(s), want > 1 for a %d-char heading-less document", len(got), len(text))
	}

	var rebuilt []string
	for i, ch := range got {
		if len(ch.Text) > documentEventCharBudget {
			t.Errorf("window %d text is %d chars, want <= %d", i, len(ch.Text), documentEventCharBudget)
		}
		if ch.Index != 1 {
			t.Errorf("window %d index = %d, want 1 (single logical chapter)", i, ch.Index)
		}
		rebuilt = append(rebuilt, ch.Text)
	}
	if got, want := strings.Join(rebuilt, " "), text; got != want {
		t.Errorf("windowed text does not reconstruct the original (order/content lost)")
	}
}

// bindTestProject creates a temp project dir bound via .mom-project.yaml,
// chdirs into it (restored on cleanup), and isolates the central Ledger
// under a temp $HOME.
func bindTestProject(t *testing.T, projectID string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := t.TempDir()
	if err := project.WriteBinding(projDir, projectID, false); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func resetIngestFlags(t *testing.T) {
	t.Helper()
	ingestAuthor, ingestTitle, ingestText = "", "", false
	t.Cleanup(func() { ingestAuthor, ingestTitle, ingestText = "", "", false })
}

func documentChapterEvents(t *testing.T, ldir string) (titles []string, payloads []map[string]any) {
	t.Helper()
	led, err := ledger.Open(ldir)
	if err != nil {
		t.Fatalf("opening ledger: %v", err)
	}
	defer led.Close()

	it := led.Iterate(0)
	defer it.Close()
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		if string(rec.Event.Type) != "capture.document_chapter.observed" {
			continue
		}
		title, _ := rec.Event.Payload["chapter_title"].(string)
		titles = append(titles, title)
		payloads = append(payloads, rec.Event.Payload)
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	return titles, payloads
}

func TestRunIngest_EndToEnd_Markdown(t *testing.T) {
	bindTestProject(t, "test-proj")
	resetIngestFlags(t)
	ingestAuthor = "Jane Author"

	docFile := filepath.Join(t.TempDir(), "book.md")
	content := "# Chapter 1\nIntro text.\n\n# Chapter 2\nMore text.\n"
	if err := os.WriteFile(docFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	if err := runIngest(cmd, []string{docFile}); err != nil {
		t.Fatalf("runIngest: %v", err)
	}

	ldir, err := ledgerDir()
	if err != nil {
		t.Fatal(err)
	}
	titles, payloads := documentChapterEvents(t, ldir)
	if len(titles) != 2 {
		t.Fatalf("got %d document_chapter events, want 2: %v", len(titles), titles)
	}
	if titles[0] != "Chapter 1" || titles[1] != "Chapter 2" {
		t.Errorf("chapter titles = %v, want [Chapter 1, Chapter 2]", titles)
	}
	for _, p := range payloads {
		if got, _ := p["project_id"].(string); got != "test-proj" {
			t.Errorf("project_id = %q, want test-proj", got)
		}
		if got, _ := p["doc_author"].(string); got != "Jane Author" {
			t.Errorf("doc_author = %q, want Jane Author", got)
		}
		if got, _ := p["source_class"].(string); got != "document" {
			t.Errorf("source_class = %q, want document", got)
		}
	}
}

func TestRunIngest_EndToEnd_EPUB(t *testing.T) {
	bindTestProject(t, "test-proj")
	resetIngestFlags(t)

	opf := `<?xml version="1.0"?>
<package>
  <metadata><dc:title>Test Book</dc:title><dc:creator>Test Author</dc:creator></metadata>
  <manifest>
    <item id="a" href="a.xhtml" media-type="application/xhtml+xml"/>
    <item id="b" href="b.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="a"/>
    <itemref idref="b"/>
  </spine>
</package>`
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`,
		"OEBPS/content.opf": opf,
		"OEBPS/a.xhtml":     `<html><body><h1>A</h1><p>Text A</p></body></html>`,
		"OEBPS/b.xhtml":     `<html><body><h1>B</h1><p>Text B</p></body></html>`,
	}
	docFile := filepath.Join(t.TempDir(), "book.epub")
	f, err := os.Create(docFile)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cmd := &cobra.Command{}
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	if err := runIngest(cmd, []string{docFile}); err != nil {
		t.Fatalf("runIngest: %v", err)
	}

	ldir, err := ledgerDir()
	if err != nil {
		t.Fatal(err)
	}
	titles, payloads := documentChapterEvents(t, ldir)
	if len(titles) != 2 {
		t.Fatalf("got %d document_chapter events, want 2: %v", len(titles), titles)
	}
	if titles[0] != "A" || titles[1] != "B" {
		t.Errorf("chapter titles = %v, want [A, B] (spine order)", titles)
	}
	for _, p := range payloads {
		if got, _ := p["doc_title"].(string); got != "Test Book" {
			t.Errorf("doc_title = %q, want Test Book (from OPF metadata, not filename)", got)
		}
		if got, _ := p["doc_author"].(string); got != "Test Author" {
			t.Errorf("doc_author = %q, want Test Author", got)
		}
	}
}

func TestRunIngest_SecondIngestOfSameContentAppendsNothing(t *testing.T) {
	bindTestProject(t, "test-proj")
	resetIngestFlags(t)

	docFile := filepath.Join(t.TempDir(), "book.md")
	content := "# Chapter 1\nIntro text.\n\n# Chapter 2\nMore text.\n"
	if err := os.WriteFile(docFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func() string {
		cmd := &cobra.Command{}
		out := new(bytes.Buffer)
		cmd.SetOut(out)
		if err := runIngest(cmd, []string{docFile}); err != nil {
			t.Fatalf("runIngest: %v", err)
		}
		return out.String()
	}

	run()

	ldir, err := ledgerDir()
	if err != nil {
		t.Fatal(err)
	}
	led, err := ledger.Open(ldir)
	if err != nil {
		t.Fatalf("opening ledger: %v", err)
	}
	headAfterFirst, _ := led.HeadOffset()
	led.Close()

	secondOut := run()
	if !strings.Contains(secondOut, "already ingested") {
		t.Errorf("second ingest output = %q, want it to report already-ingested", secondOut)
	}

	led, err = ledger.Open(ldir)
	if err != nil {
		t.Fatalf("re-opening ledger: %v", err)
	}
	defer led.Close()
	headAfterSecond, _ := led.HeadOffset()
	if headAfterSecond != headAfterFirst {
		t.Errorf("ledger head after second ingest = %d, want unchanged %d (no new events appended)", headAfterSecond, headAfterFirst)
	}
}

func TestRunIngest_UnsupportedFormatIsActionable(t *testing.T) {
	bindTestProject(t, "test-proj")
	resetIngestFlags(t)

	docFile := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(docFile, []byte("%PDF-1.4 fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	err := runIngest(cmd, []string{docFile})
	if err == nil {
		t.Fatal("expected an error for an unsupported .pdf input")
	}
	if !strings.Contains(err.Error(), "--text") || !strings.Contains(err.Error(), "pdftotext") {
		t.Errorf("error = %q, want it to mention --text and pdftotext", err.Error())
	}
}

func TestRunIngest_TextFlagAcceptsAnyExtension(t *testing.T) {
	bindTestProject(t, "test-proj")
	resetIngestFlags(t)
	ingestText = true

	docFile := filepath.Join(t.TempDir(), "book.pdf") // plain text under a .pdf name
	if err := os.WriteFile(docFile, []byte("Chapter 1\nIntro.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	if err := runIngest(cmd, []string{docFile}); err != nil {
		t.Fatalf("runIngest: %v", err)
	}

	ldir, err := ledgerDir()
	if err != nil {
		t.Fatal(err)
	}
	titles, _ := documentChapterEvents(t, ldir)
	if len(titles) != 1 {
		t.Fatalf("got %d document_chapter events, want 1: %v", len(titles), titles)
	}
}

func TestRunIngest_StdinRequiresTitle(t *testing.T) {
	bindTestProject(t, "test-proj")
	resetIngestFlags(t)
	ingestText = true

	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("Chapter 1\nIntro.\n"))
	err := runIngest(cmd, []string{"-"})
	if err == nil {
		t.Fatal("expected an error for stdin input without --title")
	}

	ldir, err := ledgerDir()
	if err != nil {
		t.Fatal(err)
	}
	titles, _ := documentChapterEvents(t, ldir)
	if len(titles) != 0 {
		t.Fatalf("got %d document_chapter events, want 0 (no ledger write before the --title check)", len(titles))
	}
}

func TestRunIngest_NoExternalProcessReferences(t *testing.T) {
	forbidden := []string{"python3", "extract.py", "book-to-skill", "ingest_extractor"}
	roots := []string{
		".",
		filepath.Join("..", "..", "shared", "config"),
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, term := range forbidden {
				if strings.Contains(string(data), term) {
					t.Errorf("%s contains forbidden term %q (external-process ingestion path must be fully removed)", filepath.Join(root, e.Name()), term)
				}
			}
		}
	}
}
