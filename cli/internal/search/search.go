package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Result is a single search result returned by Search.
type Result struct {
	ID            string   `json:"id"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	Score         float64  `json:"score"`
	SourceSession string   `json:"source_session,omitempty"`
	Created       string   `json:"created,omitempty"`
}

// SearchOptions controls a Search call.
type SearchOptions struct {
	Query         string
	MaxResults    int
	Tags          []string
	SessionID     string
	// ExcludeDrafts filters out docs with promotion_state == "draft".
	// Enabled by default in mom_recall to avoid raw drafter output (#147).
	ExcludeDrafts bool
}

// memDoc is the minimal shape of a memory JSON document.
type memDoc struct {
	ID             string         `json:"id"`
	Content        map[string]any `json:"content"`
	Tags           []string       `json:"tags"`
	Summary        string         `json:"summary"`
	Lifecycle      string         `json:"lifecycle"`
	SourceSession  string         `json:"source_session"`
	Created        string         `json:"created"`
	PromotionState string         `json:"promotion_state"`
}

// Search performs BM25 search over all memory JSON documents in memoryDir.
// An empty Query with no Tags returns all documents with score 1.0.
func Search(memoryDir string, opts SearchOptions) ([]Result, error) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if opts.MaxResults == 0 {
		opts.MaxResults = 5
	}

	var docs []memDoc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memoryDir, e.Name()))
		if err != nil {
			continue
		}
		var d memDoc
		if err := json.Unmarshal(data, &d); err != nil {
			continue
		}

		// Apply promotion_state filter (#147 — exclude raw drafts from recall).
		if opts.ExcludeDrafts && d.PromotionState == "draft" {
			continue
		}

		// Apply tag filter (AND logic).
		if len(opts.Tags) > 0 {
			tagSet := make(map[string]bool, len(d.Tags))
			for _, t := range d.Tags {
				tagSet[t] = true
			}
			match := true
			for _, t := range opts.Tags {
				if !tagSet[t] {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		// Apply session filter.
		if opts.SessionID != "" && d.SourceSession != opts.SessionID {
			continue
		}

		docs = append(docs, d)
	}

	if len(docs) == 0 {
		return nil, nil
	}

	// Build corpus strings for BM25 indexing.
	corpus := make([]string, len(docs))
	for i, d := range docs {
		text := d.Summary + " " + strings.Join(d.Tags, " ")
		for _, v := range d.Content {
			if s, ok := v.(string); ok {
				text += " " + s
			}
		}
		corpus[i] = text
	}

	idx := NewBM25Index(corpus)

	type scored struct {
		docIdx int
		score  float64
	}
	var results []scored

	if opts.Query == "" {
		// No query — return all with equal score.
		for i := range docs {
			results = append(results, scored{docIdx: i, score: 1.0})
		}
	} else {
		for i, text := range corpus {
			s := idx.Score(opts.Query, TokenizeBM25(text))
			if s > 0 {
				results = append(results, scored{docIdx: i, score: s})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	out := make([]Result, 0, len(results))
	for _, r := range results {
		d := docs[r.docIdx]
		contentStr := d.Summary
		if detail, ok := d.Content["detail"].(string); ok {
			contentStr = detail
		}
		out = append(out, Result{
			ID:            d.ID,
			Content:       contentStr,
			Tags:          d.Tags,
			Score:         r.score,
			SourceSession: d.SourceSession,
			Created:       d.Created,
		})
	}

	return out, nil
}
