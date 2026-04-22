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

	var drafts []Draft
	for i, chunk := range chunks {
		var texts []string
		var allPaths []string
		var allKeywords []RakeCandidate

		for j := chunk.StartIdx; j < chunk.EndIdx; j++ {
			texts = append(texts, turns[j].Text)
			allPaths = append(allPaths, parsedTurns[j].FilePaths...)
			allKeywords = append(allKeywords, RAKE(turns[j].Text, 5)...)
		}

		content := strings.Join(texts, "\n")

		// Build tags from multiple sources.
		tags := make(map[string]bool)
		for _, t := range ExtractFileTags(allPaths) {
			tags[t] = true
		}
		for _, t := range ExtractIdentifiers(content) {
			if len(t) > 2 {
				tags[t] = true
			}
		}
		ranked := bm25.RankCandidates(allKeywords)
		for _, t := range ranked {
			if len(t) > 2 {
				tags[t] = true
			}
		}

		var tagSlice []string
		for t := range tags {
			tagSlice = append(tagSlice, t)
		}
		sort.Strings(tagSlice)
		if len(tagSlice) > 15 {
			tagSlice = tagSlice[:15]
		}

		// Use the source file from the first turn in the chunk.
		sourceFile := ""
		if chunk.StartIdx < len(turns) {
			sourceFile = turns[chunk.StartIdx].SourceFile
		}

		drafts = append(drafts, Draft{
			ID:            fmt.Sprintf("%s-%03d", sessionID, i+1),
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
	return strings.Fields(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			return r
		}
		return ' '
	}, text))
}

func splitAtStopwords(words []string) [][]string {
	var result [][]string
	var current []string
	for _, w := range words {
		if stopwords[w] {
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
	return result
}

func sortCandidates(candidates []RakeCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
}
