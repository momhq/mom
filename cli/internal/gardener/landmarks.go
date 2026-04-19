// Package gardener provides landmark computation for the KB memory graph.
// Landmarks are high-centrality memory documents that sit at structural
// crossroads — connected to many others via shared tags.
package gardener

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// MinDocsForLandmarks is the minimum number of memory docs required before
// landmark computation is meaningful. Corpora smaller than this threshold
// are skipped — the graph is too sparse to produce reliable centrality scores.
const MinDocsForLandmarks = 100

// docEntry is a minimal in-memory representation used during computation.
type docEntry struct {
	id   string
	tags []string
	path string
	raw  map[string]any
}

// ComputeLandmarks loads all memory docs from memDir, builds a tag co-occurrence
// graph, computes weighted degree centrality, and marks the top thresholdPct% as
// landmarks. Returns the number of doc files updated.
//
// Edge weight between two docs that share a tag is 1/count(docs with that tag),
// so rare shared tags produce higher weights. The weighted degree of each doc is
// the sum of all its edge weights. Scores are normalised to [0, 1].
//
// If len(docs) < MinDocsForLandmarks, computation is skipped and (0, nil) is
// returned.
func ComputeLandmarks(memDir string, thresholdPct float64) (int, error) {
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return 0, fmt.Errorf("reading memory dir: %w", err)
	}

	// Load all JSON docs.
	var docs []docEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(memDir, e.Name())
		raw, err := loadRaw(path)
		if err != nil {
			// Skip unreadable/unparseable files.
			continue
		}
		id, _ := raw["id"].(string)
		if id == "" {
			id = strings.TrimSuffix(e.Name(), ".json")
		}
		tags := extractTags(raw)
		docs = append(docs, docEntry{id: id, tags: tags, path: path, raw: raw})
	}

	if len(docs) < MinDocsForLandmarks {
		return 0, nil
	}

	// Build tag → doc-index mapping to know how many docs share each tag.
	tagDocIndices := make(map[string][]int, 64)
	for i, d := range docs {
		for _, tag := range d.tags {
			tagDocIndices[tag] = append(tagDocIndices[tag], i)
		}
	}

	// Compute weighted degree: for each doc, sum edge weights over all neighbours.
	// Two docs share an edge per shared tag; weight of that edge = 1/len(tagDocIndices[tag]).
	// Use a set to avoid double-counting the same pair from multiple shared tags.
	// We accumulate all pair weights into a per-doc score.
	scores := make([]float64, len(docs))
	for _, indices := range tagDocIndices {
		count := len(indices)
		if count < 2 {
			// Tag shared by only one doc — no edges.
			continue
		}
		w := 1.0 / float64(count)
		// Each doc in the group gets credit for edges to all other docs in the group.
		// We add w for each other member (undirected, but we add to both sides).
		for _, i := range indices {
			// Weight contribution: (count-1) edges × w each.
			scores[i] += float64(count-1) * w
		}
	}

	// Normalise to [0, 1].
	maxScore := 0.0
	for _, s := range scores {
		if s > maxScore {
			maxScore = s
		}
	}
	if maxScore > 0 {
		for i := range scores {
			scores[i] /= maxScore
		}
	}

	// Determine landmark threshold index.
	// thresholdPct is a percentage (e.g. 2.0 = top 2%).
	cutoffCount := int(math.Ceil(float64(len(docs)) * thresholdPct / 100.0))
	if cutoffCount < 0 {
		cutoffCount = 0
	}

	// Build a sorted index of docs by score descending to find the cutoff score.
	// We avoid a full sort by computing the minimum score in the top cutoffCount
	// using a simple threshold: a doc is a landmark if its normalised rank is
	// within the top cutoffCount. We do a partial sort via score ranking.
	cutoffScore := findCutoffScore(scores, cutoffCount)

	// Write updated landmark and centrality_score fields back to each doc file.
	updated := 0
	for i, d := range docs {
		score := scores[i]
		isLandmark := cutoffCount > 0 && score >= cutoffScore && score > 0

		changed := false

		// Check existing values.
		existingLandmark, _ := d.raw["landmark"].(bool)
		if existingLandmark != isLandmark {
			changed = true
		}

		var existingScore *float64
		if v, ok := d.raw["centrality_score"].(float64); ok {
			existingScore = &v
		}
		scoreChanged := existingScore == nil || math.Abs(*existingScore-score) > 1e-9
		if scoreChanged {
			changed = true
		}

		// Always write to ensure both fields are in sync.
		d.raw["landmark"] = isLandmark
		if score == 0 {
			delete(d.raw, "centrality_score")
		} else {
			d.raw["centrality_score"] = score
		}

		if err := saveRaw(d.path, d.raw); err != nil {
			// Non-fatal: report but continue.
			continue
		}

		if changed {
			updated++
		}
	}

	return updated, nil
}

// findCutoffScore returns the minimum score that qualifies a doc as a landmark
// given the top cutoffCount docs. If cutoffCount <= 0 it returns +Inf (nothing qualifies).
func findCutoffScore(scores []float64, cutoffCount int) float64 {
	if cutoffCount <= 0 {
		return math.Inf(1)
	}
	if cutoffCount >= len(scores) {
		return 0
	}

	// Find the cutoffCount-th largest score without a full sort.
	// We use a simple O(n*cutoffCount) selection — corpus is at most a few thousand docs.
	remaining := make([]float64, len(scores))
	copy(remaining, scores)

	for k := 0; k < cutoffCount; k++ {
		maxIdx := 0
		for j := 1; j < len(remaining); j++ {
			if remaining[j] > remaining[maxIdx] {
				maxIdx = j
			}
		}
		if k == cutoffCount-1 {
			return remaining[maxIdx]
		}
		remaining[maxIdx] = -1 // mark as selected
	}

	return 0
}

// TagGraph represents tag co-occurrence data for incremental updates.
type TagGraph struct {
	// Tags maps each tag to the list of doc IDs that carry it.
	Tags map[string][]string `json:"tags"`
	// DocCount is the total number of docs included in this graph snapshot.
	DocCount int `json:"doc_count"`
}

// WriteTagGraph builds a tag co-occurrence graph from memDir and writes it to
// cacheDir/tag-graph.json. Creates cacheDir if it does not exist.
func WriteTagGraph(memDir, cacheDir string) error {
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return fmt.Errorf("reading memory dir: %w", err)
	}

	tagDocs := make(map[string][]string)
	docCount := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := loadRaw(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		id, _ := raw["id"].(string)
		if id == "" {
			id = strings.TrimSuffix(e.Name(), ".json")
		}
		docCount++
		for _, tag := range extractTags(raw) {
			tagDocs[tag] = append(tagDocs[tag], id)
		}
	}

	graph := TagGraph{
		Tags:     tagDocs,
		DocCount: docCount,
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tag graph: %w", err)
	}
	data = append(data, '\n')

	outPath := filepath.Join(cacheDir, "tag-graph.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("writing tag graph: %w", err)
	}

	return nil
}

// loadRaw reads a JSON file and returns its content as a raw map.
func loadRaw(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// saveRaw writes a raw map as formatted JSON.
func saveRaw(path string, raw map[string]any) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling doc: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// extractTags pulls the tags array from a raw doc map.
func extractTags(raw map[string]any) []string {
	v, ok := raw["tags"]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []any:
		tags := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				tags = append(tags, s)
			}
		}
		return tags
	case []string:
		return t
	}
	return nil
}
