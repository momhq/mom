// Package recall implements memory search over the v0.30 central
// vault. The Engine combines FTS5 full-text matching with bm25 column
// weights (ADR 0007), AND→OR query relaxation (ADR 0008), and
// curated→draft tier escalation (ADR 0006 quality dimension; the
// scope-chain dimension is dropped per ADR 0009 single-vault model).
package recall

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/vault"
)

// recallEscalationThreshold is the minimum number of results required
// before the engine stops escalating from AND→OR or curated→draft.
const recallEscalationThreshold = 3

// Engine runs memory search against a single central vault. The
// constructor takes the vault directly; there is no scope chain in
// v0.30.
type Engine struct {
	v *vault.Vault
}

// NewEngine returns an Engine backed by the given vault.
func NewEngine(v *vault.Vault) *Engine {
	return &Engine{v: v}
}

// Options controls one Engine.Search call. Zero values are safe.
type Options struct {
	Query      string
	MaxResults int
	Tags       []string // AND logic across tags
	SessionID  string
}

// Result is the summary view of a memory match. Callers fetch full
// content via MemoryStore.Get when they want to render a specific
// result — this keeps recall token-light.
type Result struct {
	ID              string
	Type            string
	Summary         string
	PromotionState  string
	Landmark        bool
	CentralityScore *float64
	SessionID       string
	CreatedAt       time.Time
	Score           float64 // bm25 score (lower is better; SQLite returns negative)
}

// Search runs the FTS5 query with AND→OR relaxation and
// curated→draft tier escalation, applying optional tag and session
// filters. Results are ordered by bm25 score ascending (best first).
func (e *Engine) Search(opts Options) ([]Result, error) {
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	tokens := strings.Fields(opts.Query)
	if len(tokens) == 0 {
		return nil, nil
	}

	seen := map[string]Result{}

	// Pass 1 — curated only.
	if err := e.runPass(tokens, true, opts, maxResults, seen); err != nil {
		return nil, err
	}
	if len(seen) >= recallEscalationThreshold {
		return rank(seen, maxResults), nil
	}

	// Pass 2 — drafts included.
	if err := e.runPass(tokens, false, opts, maxResults, seen); err != nil {
		return nil, err
	}

	return rank(seen, maxResults), nil
}

// runPass tries the AND form first, then the OR form, accumulating
// results into seen. Stops as soon as the threshold is reached.
func (e *Engine) runPass(tokens []string, curatedOnly bool, opts Options, limit int, seen map[string]Result) error {
	for _, form := range []func([]string) string{andQuery, orQuery} {
		if len(seen) >= recallEscalationThreshold {
			return nil
		}
		ftsQuery := form(tokens)
		results, err := e.runFTS(ftsQuery, curatedOnly, opts, limit)
		if err != nil {
			return err
		}
		for _, r := range results {
			if existing, ok := seen[r.ID]; !ok || r.Score < existing.Score {
				seen[r.ID] = r
			}
		}
	}
	return nil
}

// runFTS executes one FTS5 query with the given filters. bm25 weights
// (0, 2, 10) match the FTS5 column order (id UNINDEXED, summary,
// content_text) per ADR 0007 adjusted for the v0.30 schema.
func (e *Engine) runFTS(ftsQuery string, curatedOnly bool, opts Options, limit int) ([]Result, error) {
	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT m.id, m.type, m.summary, m.promotion_state,
			m.landmark, m.centrality_score, m.session_id, m.created_at,
			bm25(memories_fts, 0.0, 2.0, 10.0) AS score
		FROM memories_fts
		JOIN memories m ON m.id = memories_fts.id
		WHERE memories_fts MATCH ?`)
	args = append(args, ftsQuery)

	if curatedOnly {
		sb.WriteString(` AND m.promotion_state = 'curated'`)
	}
	if opts.SessionID != "" {
		sb.WriteString(` AND m.session_id = ?`)
		args = append(args, opts.SessionID)
	}
	for _, tag := range opts.Tags {
		sb.WriteString(` AND m.id IN (
			SELECT mt.memory_id FROM memory_tags mt
			JOIN tags t ON t.id = mt.tag_id WHERE t.name = ?
		)`)
		args = append(args, tag)
	}
	sb.WriteString(` ORDER BY score ASC LIMIT ?`)
	args = append(args, limit*4) // gather extras for tier merging

	var results []Result
	err := e.v.Query(sb.String(), args, func(rows *sql.Rows) error {
		for rows.Next() {
			var r Result
			var createdAt string
			if err := rows.Scan(
				&r.ID, &r.Type, &r.Summary, &r.PromotionState,
				&r.Landmark, &r.CentralityScore, &r.SessionID, &createdAt,
				&r.Score,
			); err != nil {
				return err
			}
			t, err := time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return fmt.Errorf("parse created_at: %w", err)
			}
			r.CreatedAt = t
			results = append(results, r)
		}
		return nil
	})
	return results, err
}

// andQuery joins quoted tokens with the FTS5 `AND` operator —
// every token must appear in the matched row (precision-first form).
func andQuery(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = `"` + ftsEscape(t) + `"`
	}
	return strings.Join(parts, " AND ")
}

// orQuery joins quoted tokens with FTS5 `OR` — any token matches
// (recall-first form). Equivalent to space-joined tokens (FTS5
// default is OR), but explicit for readability.
func orQuery(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = `"` + ftsEscape(t) + `"`
	}
	return strings.Join(parts, " OR ")
}

// ftsEscape doubles any inner double-quotes; FTS5 string literals use
// `""` to escape `"`. No other escaping is needed for the MATCH form.
func ftsEscape(s string) string {
	return strings.ReplaceAll(s, `"`, `""`)
}

// rank returns the top results from seen, sorted by score ascending,
// truncated to limit.
func rank(seen map[string]Result, limit int) []Result {
	out := make([]Result, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	// Insertion sort by score ascending (better first). Small N.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Score < out[j-1].Score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
