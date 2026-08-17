// Package cli — book/document ingestion.
//
// `mom ingest <file>` parses a document natively (.txt .md .html .epub
// .docx via ingress/docparse, or anything else pre-converted to text with
// --text) into chapters, and appends one capture.document_chapter.observed
// event per chapter to the Ledger via the same Editor pipeline `mom watch`
// uses — so ingested books fold into the vault alongside transcript turns.
// No external tools required: the shipped binary is enough.
//
// NOT REGISTERED on rootCmd: this command is fully implemented and tested,
// but document ingestion lacks PDF support (the format most users actually
// have), so it's held back until v0.54. Do not re-add
// rootCmd.AddCommand(ingestCmd) without PDF support landing first.
package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/events/envelope"
	"github.com/momhq/mom/ingress/docparse"
	"github.com/momhq/mom/storage/ledger"
	"github.com/spf13/cobra"
)

var (
	ingestAuthor string
	ingestTitle  string
	ingestText   bool
)

var ingestCmd = &cobra.Command{
	Use:   "ingest <file>",
	Short: "Ingest a book/document into the project vault as capture events",
	Long: `Parses a document (.txt .md .html .epub .docx natively; anything else via
--text after converting it yourself, e.g. with pdftotext for PDF), splits it
into chapters, and appends one capture.document_chapter.observed event per
chapter to the Ledger — the same pipeline "mom watch" uses for transcript
turns. A later "mom vault fold" projects the ingested chapters into
reference/<book-slug>.md alongside the project's ordinary memory.`,
	Args: cobra.ExactArgs(1),
	RunE: runIngest,
}

func init() {
	ingestCmd.Flags().StringVar(&ingestAuthor, "author", "", "Document author (overrides parsed metadata; stamped on every chapter event)")
	ingestCmd.Flags().StringVar(&ingestTitle, "title", "", "Document title (overrides parsed metadata; required when reading from stdin)")
	ingestCmd.Flags().BoolVar(&ingestText, "text", false, "Treat the input as UTF-8 text/markdown regardless of extension; use \"-\" for stdin")
}

// documentEventCharBudget bounds the text of a single emitted
// capture.document_chapter.observed event. A heading-less document (or one
// huge chapter) becomes ONE event holding its whole text; the fold cannot
// bisect a window smaller than 2*minBisectEvents, so a single oversized event
// that fails synthesis wedges the watermark forever (see hierarchy.go
// foldL0Chunk). Keeping every event's text safely under
// documentChapterSnippetMax (the fold prompt's per-event snippet cap, 8000
// chars — services/projection/llm_synth.go) means the full text always fits
// the prompt untruncated, so there's no reason for that window to fail.
const documentEventCharBudget = 6000

// windowOversizedChapters splits any chapter whose text exceeds
// documentEventCharBudget into multiple bounded, offset-preserving windows.
// Each window keeps the parent chapter's Index (so "Through chapter N"
// still reads correctly downstream) and gets a "(part i/n)" suffix on its
// title — a stable, monotonic sub-window marker within the chapter.
func windowOversizedChapters(chapters []docparse.Chapter) []docparse.Chapter {
	out := make([]docparse.Chapter, 0, len(chapters))
	for _, ch := range chapters {
		windows := windowText(ch.Text, documentEventCharBudget)
		if len(windows) <= 1 {
			out = append(out, ch)
			continue
		}
		for i, w := range windows {
			title := ch.Title
			if title != "" {
				title = fmt.Sprintf("%s (part %d/%d)", title, i+1, len(windows))
			} else {
				title = fmt.Sprintf("part %d/%d", i+1, len(windows))
			}
			out = append(out, docparse.Chapter{Index: ch.Index, Title: title, Text: w})
		}
	}
	return out
}

// windowText splits text into windows of at most budget chars, breaking only
// at word boundaries. A single word longer than budget is kept whole in its
// own window rather than split mid-word — pathological, but real prose never
// hits it.
func windowText(text string, budget int) []string {
	text = strings.TrimSpace(text)
	if len(text) <= budget {
		return []string{text}
	}
	words := strings.Fields(text)
	var windows []string
	var b strings.Builder
	for _, w := range words {
		if b.Len() > 0 && b.Len()+1+len(w) > budget {
			windows = append(windows, b.String())
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	if b.Len() > 0 {
		windows = append(windows, b.String())
	}
	return windows
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
	inputArg := args[0]
	if inputArg == "-" && ingestTitle == "" {
		return fmt.Errorf("ingest: --title is required when reading from stdin (mom ingest --text - --title \"...\")")
	}

	projectID, _, err := resolveVaultTarget()
	if err != nil {
		return err
	}

	var doc docparse.Document
	if ingestText {
		if inputArg == "-" {
			doc, err = docparse.ExtractPlain(cmd.InOrStdin(), ingestTitle)
		} else {
			f, ferr := os.Open(inputArg)
			if ferr != nil {
				return fmt.Errorf("ingest: %w", ferr)
			}
			defer f.Close()
			title := ingestTitle
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(inputArg), filepath.Ext(inputArg))
			}
			doc, err = docparse.ExtractPlain(f, title)
		}
	} else {
		inputPath, aerr := filepath.Abs(inputArg)
		if aerr != nil {
			return aerr
		}
		if _, serr := os.Stat(inputPath); serr != nil {
			return fmt.Errorf("ingest: %w", serr)
		}
		doc, err = docparse.Extract(inputPath)
	}
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	docTitle := doc.Title
	if ingestTitle != "" {
		docTitle = ingestTitle
	}
	docAuthor := doc.Author
	if ingestAuthor != "" {
		docAuthor = ingestAuthor
	}
	chapters := windowOversizedChapters(doc.Chapters)

	sum := sha256.Sum256([]byte(doc.FullText()))
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

	out := cmd.OutOrStdout()
	already, err := documentAlreadyIngested(led, docID)
	if err != nil {
		return fmt.Errorf("ingest: scanning ledger for doc_id %s: %w", docID, err)
	}
	if already {
		fmt.Fprintf(out, "already ingested (doc_id=%s); nothing to do\n", docID)
		return nil
	}

	ed := editor.New(nil, nil).WithLedger(led)

	cwd, _ := os.Getwd()
	for _, ch := range chapters {
		canon := documentChapterCanonical{
			ProjectID:    projectID,
			DocID:        docID,
			DocTitle:     docTitle,
			DocAuthor:    docAuthor,
			ChapterIndex: ch.Index,
			ChapterTitle: ch.Title,
			Text:         ch.Text,
		}
		if err := ed.Publish(canon, editor.Source{Adapter: "ingest", Cwd: cwd}); err != nil {
			return fmt.Errorf("ingest: publishing chapter %d: %w", ch.Index, err)
		}
	}

	head, _ := led.HeadOffset()
	fmt.Fprintf(out, "ingested %q — %d chapter(s) appended (doc_id %s)\n", docTitle, len(chapters), docID)
	fmt.Fprintf(out, "ledger head: offset %d\n", head)
	fmt.Fprintln(out, "run `mom vault fold` to project it into the vault")
	return nil
}

// documentAlreadyIngested scans the ledger for any DocumentChapterObserved
// event carrying this doc_id. doc_id is a deterministic content hash, so a
// hit means this exact document was already ingested — re-running `mom
// ingest` on it (or retrying after a partial failure) must not double-append
// events, since already-appended chapters cannot be rolled back.
func documentAlreadyIngested(led *ledger.Ledger, docID string) (bool, error) {
	it := led.Iterate(0)
	defer it.Close()
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		if rec.Event.Type != envelope.DocumentChapterObserved {
			continue
		}
		if id, _ := rec.Event.Payload["doc_id"].(string); id == docID {
			return true, nil
		}
	}
	return false, it.Err()
}
