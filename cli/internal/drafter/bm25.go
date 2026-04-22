package drafter

import (
	"math"
	"strings"

	"github.com/momhq/mom/cli/internal/search"
)

// BM25Index wraps search.BM25Index with drafter-specific RankCandidates method.
type BM25Index struct {
	inner *search.BM25Index
	// docs mirrors inner's tokenized corpus for RankCandidates iteration.
	docs [][]string
}

// NewBM25Index builds an index from existing tag vocabulary.
func NewBM25Index(vocab []string) *BM25Index {
	inner := search.NewBM25Index(vocab)
	docs := make([][]string, len(vocab))
	for i, v := range vocab {
		docs[i] = search.TokenizeBM25(v)
	}
	return &BM25Index{inner: inner, docs: docs}
}

// Score returns a BM25 score for a query against a document.
func (idx *BM25Index) Score(query string, docTokens []string) float64 {
	return idx.inner.Score(query, docTokens)
}

// RankCandidates ranks RAKE candidates against existing vocabulary.
func (idx *BM25Index) RankCandidates(candidates []RakeCandidate) []string {
	type scored struct {
		tag   string
		score float64
	}
	var results []scored
	for _, c := range candidates {
		tokens := search.TokenizeBM25(c.Phrase)
		s := 0.0
		for _, doc := range idx.docs {
			s = math.Max(s, idx.inner.Score(c.Phrase, doc))
		}
		results = append(results, scored{tag: strings.Join(tokens, "-"), score: s + c.Score})
	}
	// Sort by score descending.
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	var tags []string
	for _, r := range results {
		tags = append(tags, r.tag)
	}
	return tags
}

// tokenizeBM25 is a package-local alias for the shared tokenizer.
func tokenizeBM25(s string) []string {
	return search.TokenizeBM25(s)
}
