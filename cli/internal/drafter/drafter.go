// Package drafter processes raw JSONL recordings into structured draft memory documents.
// It applies RAKE keyword extraction, BM25 ranking against existing vocabulary,
// and chunk boundary detection to group related turns.
package drafter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jdkato/prose/v2"
	"golang.org/x/text/unicode/norm"
)

// Draft is a structured memory draft produced from raw recordings.
type Draft struct {
	ID            string   `json:"id"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	SourceSession string   `json:"source_session"`
	SourceFile    string   `json:"source_file"`
	SourceLines   [2]int   `json:"source_lines"`
	Created       string   `json:"created"`
}

// Drafter orchestrates the extraction pipeline.
type Drafter struct {
	rawDir    string
	memoryDir string
	vocabFn   func() []string
}

// New creates a Drafter that reads from rawDir and writes to memoryDir.
// vocabFn returns the existing tag vocabulary for BM25 ranking.
func New(rawDir, memoryDir string, vocabFn func() []string) *Drafter {
	return &Drafter{rawDir: rawDir, memoryDir: memoryDir, vocabFn: vocabFn}
}

// Process reads raw JSONL since the given time and produces drafts.
func (d *Drafter) Process(since time.Time) ([]Draft, error) {
	entries, err := os.ReadDir(d.rawDir)
	if err != nil {
		return nil, fmt.Errorf("reading raw dir: %w", err)
	}

	var allTurns []rawTurn
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		filePath := filepath.Join(d.rawDir, e.Name())
		turns, _ := readRawFile(filePath, since)
		for i := range turns {
			turns[i].SourceFile = filePath
		}
		allTurns = append(allTurns, turns...)
	}

	// Sanitize turns: extract conversational text, drop tool_use/metadata.
	allTurns = sanitizeTurns(allTurns)

	if len(allTurns) == 0 {
		return nil, nil
	}

	// Group by session.
	bySession := make(map[string][]rawTurn)
	for _, t := range allTurns {
		bySession[t.SessionID] = append(bySession[t.SessionID], t)
	}

	vocab := d.vocabFn()
	bm25 := NewBM25Index(vocab)

	var drafts []Draft
	for sessionID, turns := range bySession {
		sessionDrafts := d.processSession(sessionID, turns, bm25)
		drafts = append(drafts, sessionDrafts...)
	}

	return drafts, nil
}

func (d *Drafter) processSession(sessionID string, turns []rawTurn, bm25 *BM25Index) []Draft {
	// Convert to Turn structs for boundary detection.
	parsedTurns := make([]Turn, len(turns))
	for i, t := range turns {
		keywords := RAKE(t.Text, 10)
		var kw []string
		for _, c := range keywords {
			kw = append(kw, c.Phrase)
		}
		parsedTurns[i] = Turn{
			Text:      t.Text,
			FilePaths: extractPaths(t.Text),
			Keywords:  kw,
		}
	}

	chunks := DetectBoundaries(parsedTurns, 0.6)

	// Find the next available draft number for this session to avoid overwriting.
	nextNum := d.nextDraftNumber(sessionID)

	var drafts []Draft
	for i, chunk := range chunks {
		var texts []string
		var tagTexts []string
		var allPaths []string
		var allKeywords []RakeCandidate

		for j := chunk.StartIdx; j < chunk.EndIdx; j++ {
			texts = append(texts, turns[j].Text)
			allPaths = append(allPaths, parsedTurns[j].FilePaths...)
			// Level 2: stricter sanitization for tag extraction.
			cleaned := sanitizeForTags(turns[j].Text)
			tagTexts = append(tagTexts, cleaned)
			allKeywords = append(allKeywords, RAKE(cleaned, 5)...)
		}

		content := strings.Join(texts, "\n")
		tagContent := strings.Join(tagTexts, "\n")

		// Build tags from multiple sources using tag-sanitized text.
		tags := make(map[string]bool)
		for _, t := range ExtractFileTags(allPaths) {
			tags[t] = true
		}
		for _, t := range ExtractIdentifiers(tagContent) {
			if len(t) > 2 && len(t) <= 40 {
				tags[t] = true
			}
		}
		ranked := bm25.RankCandidates(allKeywords)
		for _, t := range ranked {
			if len(t) > 2 && len(t) <= 40 {
				tags[t] = true
			}
		}

		// Split compound tags on "-" and deduplicate into single words.
		wordSet := make(map[string]bool)
		for t := range tags {
			parts := strings.Split(t, "-")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if len(p) > 2 && isCleanTag(p) {
					wordSet[p] = true
				}
			}
		}

		// POS-tag filter: keep only nouns (NN, NNP, NNS, NNPS) and
		// foreign words (FW) — drop verbs, adverbs, prepositions, etc.
		var tagSlice []string
		tagText := strings.Join(mapKeys(wordSet), " ")
		doc, posErr := prose.NewDocument(tagText)
		if posErr == nil {
			for _, tok := range doc.Tokens() {
				tag := tok.Tag
				if strings.HasPrefix(tag, "NN") || tag == "FW" {
					w := strings.ToLower(tok.Text)
					if len(w) > 2 {
						tagSlice = append(tagSlice, w)
					}
				}
			}
		} else {
			// Fallback: use all words if POS fails.
			for w := range wordSet {
				tagSlice = append(tagSlice, w)
			}
		}
		sort.Strings(tagSlice)
		// Deduplicate after lowercasing.
		tagSlice = dedup(tagSlice)
		if len(tagSlice) > 15 {
			tagSlice = tagSlice[:15]
		}

		// Use the source file from the first turn in the chunk.
		sourceFile := ""
		if chunk.StartIdx < len(turns) {
			sourceFile = turns[chunk.StartIdx].SourceFile
		}

		drafts = append(drafts, Draft{
			ID:            fmt.Sprintf("%s-%03d", sessionID, nextNum+i),
			Content:       content,
			Tags:          tagSlice,
			SourceSession: sessionID,
			SourceFile:    sourceFile,
			SourceLines:   [2]int{chunk.StartIdx, chunk.EndIdx},
			Created:       time.Now().UTC().Format(time.RFC3339),
		})
	}

	return drafts
}

// nextDraftNumber scans the memory directory for existing drafts of a session
// and returns the next available number (e.g., if -001, -002 exist, returns 3).
func (d *Drafter) nextDraftNumber(sessionID string) int {
	entries, err := os.ReadDir(d.memoryDir)
	if err != nil {
		return 1
	}
	prefix := sessionID + "-"
	max := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		// Extract number from "{session}-NNN.json"
		numPart := strings.TrimPrefix(name, prefix)
		numPart = strings.TrimSuffix(numPart, ".json")
		var n int
		if _, err := fmt.Sscanf(numPart, "%d", &n); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

type rawTurn struct {
	Timestamp  string `json:"timestamp"`
	Event      string `json:"event"`
	Text       string `json:"text"`
	SessionID  string `json:"session_id"`
	SourceFile string `json:"-"` // path of the .jsonl file this turn came from
}

func readRawFile(path string, since time.Time) ([]rawTurn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var turns []rawTurn
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var t rawTurn
		if err := json.Unmarshal(scanner.Bytes(), &t); err != nil {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, t.Timestamp); err == nil && ts.After(since) {
			turns = append(turns, t)
		}
	}
	return turns, nil
}

func extractPaths(text string) []string {
	var paths []string
	for _, word := range strings.Fields(text) {
		if strings.Contains(word, "/") && strings.Contains(word, ".") {
			clean := strings.Trim(word, "\"'`(),;:")
			paths = append(paths, clean)
		}
	}
	return paths
}

// tokenize splits text into lowercase words, collapsing punctuation to spaces.
func tokenize(text string) []string {
	// Normalize unicode: decompose accents, then strip combining marks.
	text = stripAccents(text)
	return strings.Fields(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			return r
		}
		return ' '
	}, text))
}

// stripAccents removes diacritical marks (ã→a, é→e, ç→c, etc.).
func stripAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) { // Mn = Mark, Nonspacing (combining accents)
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitAtStopwords(words []string) [][]string {
	var result [][]string
	var current []string
	for _, w := range words {
		if isStopword(w) {
			if len(current) > 0 {
				result = append(result, current)
				current = nil
			}
		} else {
			current = append(current, w)
		}
	}
	if len(current) > 0 {
		result = append(result, current)
	}
	// Enforce max phrase length — split long phrases into chunks.
	var trimmed [][]string
	for _, phrase := range result {
		for len(phrase) > maxPhraseWords {
			trimmed = append(trimmed, phrase[:maxPhraseWords])
			phrase = phrase[maxPhraseWords:]
		}
		if len(phrase) > 0 {
			trimmed = append(trimmed, phrase)
		}
	}
	return trimmed
}

func sortCandidates(candidates []RakeCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
}

// isCleanTag returns false for markdown fragments, URL pieces, and other
// non-word noise that shouldn't become tags. Only rejects obvious junk —
// intentionally permissive to avoid dropping valid technical terms.
func isCleanTag(s string) bool {
	// Contains markdown/formatting chars: ` * # [ ] ( ) < >
	for _, r := range s {
		switch r {
		case '`', '*', '#', '[', ']', '(', ')', '<', '>', '{', '}':
			return false
		}
	}
	// URL fragments
	if strings.Contains(s, "https") || strings.Contains(s, "http") {
		return false
	}
	// Dotfiles / file extensions like ".aider.tags.cache"
	if strings.HasPrefix(s, ".") {
		return false
	}
	// Must contain at least one letter (rejects pure punctuation leftovers)
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func dedup(sorted []string) []string {
	if len(sorted) == 0 {
		return sorted
	}
	out := []string{sorted[0]}
	for _, s := range sorted[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
