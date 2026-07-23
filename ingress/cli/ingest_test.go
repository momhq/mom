package cli

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momhq/mom/shared/project"
	"github.com/momhq/mom/storage/ledger"
	"github.com/spf13/cobra"
)

func TestSplitChapters(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		count int
		want  []string // expected chapter titles (first line of each chapter)
	}{
		{
			name:  "no headings falls back to one chapter",
			text:  "Just some prose.\nMore prose.\n",
			count: 1,
		},
		{
			name:  "two arabic chapters",
			text:  "Front matter.\n\nChapter 1\nIntro text.\n\nChapter 2\nMore text.\n",
			count: 2,
			want:  []string{"Chapter 1", "Chapter 2"},
		},
		{
			name:  "capitulo variant",
			text:  "Capítulo 1: Início\nTexto.\n\nCapítulo 2: Meio\nMais texto.\n",
			count: 2,
			want:  []string{"Capítulo 1: Início", "Capítulo 2: Meio"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chapters := splitChapters(tc.text)
			if len(chapters) != tc.count {
				t.Fatalf("got %d chapters, want %d: %+v", len(chapters), tc.count, chapters)
			}
			for i, want := range tc.want {
				if chapters[i].Title != want {
					t.Errorf("chapter %d title = %q, want %q", i, chapters[i].Title, want)
				}
				if chapters[i].Index != i+1 {
					t.Errorf("chapter %d index = %d, want %d", i, chapters[i].Index, i+1)
				}
			}
		})
	}
}

func TestSplitChapters_WindowsOversizedHeadinglessDocument(t *testing.T) {
	// A heading-less document becomes ONE chapter (see the "no headings"
	// case above). Build one far larger than documentEventCharBudget so it
	// must be windowed into multiple bounded, ordered events — otherwise the
	// fold can never bisect the resulting 1-event chunk (hierarchy.go
	// foldL0Chunk) and the watermark wedges forever.
	words := make([]string, 0, 3000)
	for i := 0; i < 3000; i++ {
		words = append(words, "word")
	}
	text := strings.Join(words, " ") // ~15000 chars, no "Chapter N" heading anywhere

	chapters := splitChapters(text)
	if len(chapters) < 2 {
		t.Fatalf("got %d window(s), want > 1 for a %d-char heading-less document", len(chapters), len(text))
	}

	var rebuilt []string
	for i, ch := range chapters {
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

// stubExtractorScript writes a python3 script that mimics book-to-skill's
// extract.py surface: `--check` exits 0, and `<file> --mode text` writes a
// deterministic full_text.txt + metadata.json into $BOOK_SKILL_WORKDIR.
func stubExtractorScript(t *testing.T, fullText string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "extract.py")
	encoded := base64.StdEncoding.EncodeToString([]byte(fullText))
	script := `import sys, os, json, base64
from pathlib import Path

if "--check" in sys.argv[1:]:
    print("ok")
    sys.exit(0)

workdir = Path(os.environ["BOOK_SKILL_WORKDIR"])
workdir.mkdir(parents=True, exist_ok=True)
text = base64.b64decode("` + encoded + `").decode("utf-8")
(workdir / "full_text.txt").write_text(text)
(workdir / "metadata.json").write_text(json.dumps({"filename": "test-book.pdf"}))
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
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

func TestRunIngest_EndToEnd(t *testing.T) {
	bindTestProject(t, "test-proj")

	extractor := stubExtractorScript(t, "Chapter 1\nIntro text.\n\nChapter 2\nMore text.\n")
	writeTestMomConfig(t, extractor)

	docFile := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(docFile, []byte("fake pdf bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	ingestAuthor = "Jane Author"
	t.Cleanup(func() { ingestAuthor = "" })

	if err := runIngest(cmd, []string{docFile}); err != nil {
		t.Fatalf("runIngest: %v", err)
	}

	ldir, err := ledgerDir()
	if err != nil {
		t.Fatal(err)
	}
	led, err := ledger.Open(ldir)
	if err != nil {
		t.Fatalf("opening ledger: %v", err)
	}
	defer led.Close()

	it := led.Iterate(0)
	defer it.Close()
	var records []string
	var titles []string
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		if string(rec.Event.Type) != "capture.document_chapter.observed" {
			continue
		}
		records = append(records, rec.Event.SessionID)
		title, _ := rec.Event.Payload["chapter_title"].(string)
		titles = append(titles, title)
		if got, _ := rec.Event.Payload["project_id"].(string); got != "test-proj" {
			t.Errorf("project_id = %q, want test-proj", got)
		}
		if got, _ := rec.Event.Payload["doc_author"].(string); got != "Jane Author" {
			t.Errorf("doc_author = %q, want Jane Author", got)
		}
		if got, _ := rec.Event.Payload["source_class"].(string); got != "document" {
			t.Errorf("source_class = %q, want document", got)
		}
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 {
		t.Fatalf("got %d document_chapter events, want 2: %v", len(titles), titles)
	}
	if titles[0] != "Chapter 1" || titles[1] != "Chapter 2" {
		t.Errorf("chapter titles = %v, want [Chapter 1, Chapter 2]", titles)
	}
}

func TestRunIngest_SecondIngestOfSameContentAppendsNothing(t *testing.T) {
	bindTestProject(t, "test-proj")

	extractor := stubExtractorScript(t, "Chapter 1\nIntro text.\n\nChapter 2\nMore text.\n")
	writeTestMomConfig(t, extractor)

	docFile := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(docFile, []byte("fake pdf bytes"), 0o644); err != nil {
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

// writeTestMomConfig points vault.ingest_extractor at the given path via
// $HOME/.mom/config.yaml (config.Load's expected location).
func writeTestMomConfig(t *testing.T, extractorPath string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	momDir := filepath.Join(home, ".mom")
	if err := os.MkdirAll(momDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "version: \"1\"\nvault:\n  ingest_extractor: " + extractorPath + "\n"
	if err := os.WriteFile(filepath.Join(momDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunIngest_MissingExtractor(t *testing.T) {
	bindTestProject(t, "test-proj")
	writeTestMomConfig(t, filepath.Join(t.TempDir(), "does-not-exist.py"))

	docFile := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(docFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	if err := runIngest(cmd, []string{docFile}); err == nil {
		t.Fatal("expected a clean error for a missing extractor, got nil")
	}
}

func TestRunIngest_MissingPython3(t *testing.T) {
	bindTestProject(t, "test-proj")
	extractor := stubExtractorScript(t, "text")
	writeTestMomConfig(t, extractor)

	// Empty PATH — exec.LookPath("python3") must fail.
	emptyPathDir := t.TempDir()
	t.Setenv("PATH", emptyPathDir)

	docFile := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(docFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	if err := runIngest(cmd, []string{docFile}); err == nil {
		t.Fatal("expected a clean error when python3 is unavailable, got nil")
	}
}
