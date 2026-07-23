// Package cli — book/document ingestion.
//
// `mom ingest <file>` runs the book-to-skill extractor to pull plain text out
// of a document (PDF, EPUB, DOCX, ...), splits it into chapters, and appends
// one capture.document_chapter.observed event per chapter to the Ledger via
// the same Editor pipeline `mom watch` uses — so ingested books fold into the
// vault alongside transcript turns.
package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/events/envelope"
	"github.com/momhq/mom/shared/config"
	"github.com/momhq/mom/storage/ledger"
	"github.com/spf13/cobra"
)

var ingestAuthor string

var ingestCmd = &cobra.Command{
	Use:   "ingest <file>",
	Short: "Ingest a book/document into the project vault as capture events",
	Long: `Extracts plain text from a document (PDF, EPUB, DOCX, HTML, Markdown, ...)
via the book-to-skill extractor, splits it into chapters, and appends one
capture.document_chapter.observed event per chapter to the Ledger — the same
pipeline "mom watch" uses for transcript turns. A later "mom vault fold"
projects the ingested chapters into reference/<book-slug>.md alongside the
project's ordinary memory.`,
	Args: cobra.ExactArgs(1),
	RunE: runIngest,
}

func init() {
	ingestCmd.Flags().StringVar(&ingestAuthor, "author", "", "Document author (stamped on every chapter event)")
}

// chapterHeadingRe splits full_text.txt into chapters. A heading line looks
// like "Chapter 5" or "Capítulo 5: ..."; when it matches zero times the whole
// document is treated as one chapter (Phase E resolved decision #1).
var chapterHeadingRe = regexp.MustCompile(`(?im)^\s*(chapter|cap[ií]tulo)\s+\d+`)

type ingestChapter struct {
	Index int
	Title string
	Text  string
}

// splitChapters splits fullText at chapterHeadingRe matches. Text preceding
// the first heading (front matter, ToC) is discarded from a multi-chapter
// document; a document with no heading matches becomes one chapter holding
// the whole text.
func splitChapters(fullText string) []ingestChapter {
	locs := chapterHeadingRe.FindAllStringIndex(fullText, -1)
	if len(locs) == 0 {
		return []ingestChapter{{Index: 1, Text: strings.TrimSpace(fullText)}}
	}
	chapters := make([]ingestChapter, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(fullText)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		segment := strings.TrimSpace(fullText[start:end])
		chapters = append(chapters, ingestChapter{Index: i + 1, Title: firstLine(segment), Text: segment})
	}
	return chapters
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// documentChapterCanonical implements editor.Canonicalizer for one ingested
// chapter — the producer-side shape ADR 0020 requires (see events/editor.go).
type documentChapterCanonical struct {
	ProjectID    string
	DocID        string
	DocTitle     string
	DocAuthor    string
	ChapterIndex int
	ChapterTitle string
	Text         string
}

func (d documentChapterCanonical) Canonical() (envelope.EventType, map[string]any) {
	payload := map[string]any{
		"project_id":    d.ProjectID,
		"doc_id":        d.DocID,
		"doc_title":     d.DocTitle,
		"chapter_index": d.ChapterIndex,
		"chapter_title": d.ChapterTitle,
		"text":          d.Text,
		"source_class":  "document",
	}
	if d.DocAuthor != "" {
		payload["doc_author"] = d.DocAuthor
	}
	return envelope.DocumentChapterObserved, payload
}

func runIngest(cmd *cobra.Command, args []string) error {
	inputPath, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	projectID, _, err := resolveVaultTarget()
	if err != nil {
		return err
	}

	extractor := resolveIngestExtractor()
	if _, err := os.Stat(extractor); err != nil {
		return fmt.Errorf("ingest: book-to-skill extractor not found at %s (set vault.ingest_extractor in ~/.mom/config.yaml to override): %w", extractor, err)
	}
	if _, err := exec.LookPath("python3"); err != nil {
		return fmt.Errorf("ingest: python3 not found on PATH — required to run the book-to-skill extractor")
	}
	if out, err := exec.Command("python3", extractor, "--check").CombinedOutput(); err != nil {
		return fmt.Errorf("ingest: extractor dependency check failed: %w\n%s", err, out)
	}

	workdir, err := os.MkdirTemp("", "mom-ingest-*")
	if err != nil {
		return fmt.Errorf("ingest: creating workdir: %w", err)
	}
	defer os.RemoveAll(workdir)

	extractCmd := exec.Command("python3", extractor, inputPath, "--mode", "text")
	extractCmd.Env = append(os.Environ(), "BOOK_SKILL_WORKDIR="+workdir)
	if out, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ingest: extraction failed: %w\n%s", err, out)
	}

	fullText, err := os.ReadFile(filepath.Join(workdir, "full_text.txt"))
	if err != nil {
		return fmt.Errorf("ingest: reading extracted text: %w", err)
	}

	docTitle := ingestDocTitle(workdir, inputPath)
	chapters := splitChapters(string(fullText))

	sum := sha256.Sum256(fullText)
	docID := fmt.Sprintf("%x", sum[:8])

	ldir, err := ledgerDir()
	if err != nil {
		return err
	}
	led, err := ledger.Open(ldir)
	if err != nil {
		return fmt.Errorf("ingest: opening ledger: %w", err)
	}
	defer led.Close()
	ed := editor.New(nil, nil).WithLedger(led)

	cwd, _ := os.Getwd()
	for _, ch := range chapters {
		canon := documentChapterCanonical{
			ProjectID:    projectID,
			DocID:        docID,
			DocTitle:     docTitle,
			DocAuthor:    ingestAuthor,
			ChapterIndex: ch.Index,
			ChapterTitle: ch.Title,
			Text:         ch.Text,
		}
		if err := ed.Publish(canon, editor.Source{Adapter: "ingest", Cwd: cwd}); err != nil {
			return fmt.Errorf("ingest: publishing chapter %d: %w", ch.Index, err)
		}
	}

	head, _ := led.HeadOffset()
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "ingested %q — %d chapter(s) appended (doc_id %s)\n", docTitle, len(chapters), docID)
	fmt.Fprintf(out, "ledger head: offset %d\n", head)
	fmt.Fprintln(out, "run `mom vault fold` to project it into the vault")
	return nil
}

// ingestMetadata mirrors the subset of book-to-skill's metadata.json this
// command reads.
type ingestMetadata struct {
	Filename string `json:"filename"`
}

// ingestDocTitle resolves the book/document title: the extractor's reported
// filename (metadata.json), falling back to the input file's own base name.
// The extractor never reports a document title (no format it parses exposes
// one reliably), so this is the best available signal.
func ingestDocTitle(workdir, inputPath string) string {
	if raw, err := os.ReadFile(filepath.Join(workdir, "metadata.json")); err == nil {
		var meta ingestMetadata
		if json.Unmarshal(raw, &meta) == nil && meta.Filename != "" && meta.Filename != "multi-source" {
			return stripExt(meta.Filename)
		}
	}
	return stripExt(filepath.Base(inputPath))
}

func stripExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// resolveIngestExtractor resolves the book-to-skill extract.py path: the
// vault.ingest_extractor config key, else the default install location.
func resolveIngestExtractor() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if cfg, err := config.Load(filepath.Join(home, ".mom")); err == nil && cfg.Vault.IngestExtractor != "" {
		return cfg.Vault.IngestExtractor
	}
	return filepath.Join(home, ".claude", "skills", "book-to-skill", "scripts", "extract.py")
}
