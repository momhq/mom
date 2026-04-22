package drafter

import "strings"

// Stopwords for RAKE (common English, ~200 words).
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "shall": true, "can": true,
	"this": true, "that": true, "these": true, "those": true,
	"i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true,
	"me": true, "him": true, "her": true, "us": true, "them": true,
	"my": true, "your": true, "his": true, "its": true, "our": true, "their": true,
	"what": true, "which": true, "who": true, "whom": true, "where": true, "when": true,
	"how": true, "why": true, "if": true, "then": true, "else": true,
	"and": true, "or": true, "but": true, "not": true, "so": true, "yet": true,
	"for": true, "with": true, "from": true, "to": true, "in": true, "on": true,
	"at": true, "by": true, "of": true, "about": true, "into": true, "through": true,
	"as": true, "up": true, "out": true, "off": true, "over": true, "under": true,
	"also": true, "just": true, "very": true, "much": true, "more": true, "most": true,
	"some": true, "any": true, "no": true, "all": true, "each": true, "every": true,
	"both": true, "few": true, "many": true, "such": true, "own": true, "same": true,
	"other": true, "than": true, "too": true, "only": true, "here": true, "there": true,
	"now": true, "while": true, "after": true,
	"before": true, "during": true, "since": true, "until": true,
}

// RakeCandidate is a keyword candidate with a score.
type RakeCandidate struct {
	Phrase string
	Score  float64
}

// RAKE extracts keyword candidates from text using the RAKE algorithm.
// Returns top N candidates sorted by score descending.
func RAKE(text string, topN int) []RakeCandidate {
	// Lowercase
	text = strings.ToLower(text)

	// Split at stopwords and punctuation to get candidate phrases
	words := tokenize(text)
	phrases := splitAtStopwords(words)

	// Calculate word frequency and degree
	wordFreq := make(map[string]int)
	wordDeg := make(map[string]int)
	for _, phrase := range phrases {
		for _, word := range phrase {
			wordFreq[word]++
			wordDeg[word] += len(phrase)
		}
	}

	// Score each phrase
	scored := make(map[string]float64)
	for _, phrase := range phrases {
		key := strings.Join(phrase, " ")
		if key == "" {
			continue
		}
		var score float64
		for _, word := range phrase {
			if wordFreq[word] > 0 {
				score += float64(wordDeg[word]) / float64(wordFreq[word])
			}
		}
		scored[key] = score
	}

	// Sort by score
	var candidates []RakeCandidate
	for phrase, score := range scored {
		candidates = append(candidates, RakeCandidate{Phrase: phrase, Score: score})
	}
	sortCandidates(candidates)

	if topN > 0 && len(candidates) > topN {
		candidates = candidates[:topN]
	}
	return candidates
}
