package drafter

import (
	"math"
	"strings"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25Index indexes a vocabulary for ranking.
type BM25Index struct {
	docs   [][]string     // tokenized documents (existing tags)
	df     map[string]int // document frequency
	avgLen float64
}

// NewBM25Index builds an index from existing tag vocabulary.
func NewBM25Index(vocab []string) *BM25Index {
	idx := &BM25Index{df: make(map[string]int)}
	for _, tag := range vocab {
		tokens := tokenizeBM25(tag)
		idx.docs = append(idx.docs, tokens)
		seen := make(map[string]bool)
		for _, t := range tokens {
			if !seen[t] {
				seen[t] = true
				idx.df[t]++
			}
		}
	}
	total := 0
	for _, d := range idx.docs {
		total += len(d)
	}
	if len(idx.docs) > 0 {
		idx.avgLen = float64(total) / float64(len(idx.docs))
	}
	return idx
}

// Score returns a BM25 score for a query against a document.
func (idx *BM25Index) Score(query string, docTokens []string) float64 {
	queryTokens := tokenizeBM25(query)
	n := float64(len(idx.docs))
	dl := float64(len(docTokens))

	tf := make(map[string]int)
	for _, t := range docTokens {
		tf[t]++
	}

	var score float64
	for _, qt := range queryTokens {
		docFreq := float64(idx.df[qt])
		if docFreq == 0 {
			continue
		}
		idf := math.Log((n - docFreq + 0.5) / (docFreq + 0.5))
		termFreq := float64(tf[qt])
		tfNorm := (termFreq * (bm25K1 + 1)) / (termFreq + bm25K1*(1-bm25B+bm25B*dl/idx.avgLen))
		score += idf * tfNorm
	}
	return score
}

// RankCandidates ranks RAKE candidates against existing vocabulary.
func (idx *BM25Index) RankCandidates(candidates []RakeCandidate) []string {
	type scored struct {
		tag   string
		score float64
	}
	var results []scored
	for _, c := range candidates {
		tokens := tokenizeBM25(c.Phrase)
		s := 0.0
		for _, doc := range idx.docs {
			s = math.Max(s, idx.Score(c.Phrase, doc))
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

func tokenizeBM25(s string) []string {
	s = strings.ToLower(s)
	var tokens []string
	for _, word := range strings.Fields(s) {
		clean := strings.Trim(word, ".,;:!?()[]{}\"'`")
		if clean != "" && !stopwords[clean] {
			tokens = append(tokens, clean)
		}
	}
	return tokens
}
