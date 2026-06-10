// Package lens serves the mom sessions dashboard as a local HTTP server.
package lens

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/storage/ledger"
)

//go:embed static
var staticFS embed.FS

const centralScopeLabel = "central"

type ToolGroup struct {
	Total  int            `json:"total"`
	Detail map[string]int `json:"detail"`
}

// SessionSummary is the list-view shape returned by GET /api/sessions.
// Built purely from the Ledger: turn-observed events drive interactions,
// tool calls, tokens, and cost; memory-recorded events drive MemoryCount.
type SessionSummary struct {
	SessionID    string  `json:"session_id"`
	ScopeLabel   string  `json:"scope_label"`
	Started      string  `json:"started"`
	Ended        string  `json:"ended"`
	DurationSecs float64 `json:"duration_secs"`
	Interactions int     `json:"interactions"`
	MemoryCount  int     `json:"memory_count"`
	ToolsTotal   int     `json:"tools_total"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
}

// MemoryItem is a memory-recorded event linked to a session. The Ledger
// carries no promotion state, landmarks, or tags (those lived in the retired
// SQLite vault), so only the content + timestamp are available.
type MemoryItem struct {
	ID      string    `json:"id"`
	Summary string    `json:"summary"`
	Created time.Time `json:"created"`
}

// SessionDetail extends SessionSummary with the full per-session breakdown.
type SessionDetail struct {
	SessionSummary
	ToolCalls map[string]ToolGroup `json:"tool_calls"`
	Memories  []MemoryItem         `json:"memories"`
}

// MetaResponse is returned by GET /api/meta.
type MetaResponse struct {
	TotalSessions int `json:"total_sessions"`
	TotalMemories int `json:"total_memories"`
}

// Server serves the lens dashboard. It reads the append-only Ledger
// (the source of truth) directly — no SQLite.
type Server struct {
	ledgerDir string
	mux       *http.ServeMux
}

// New creates a Server over the central Ledger directory.
func New(ledgerDir string) (*Server, error) {
	if ledgerDir == "" {
		return nil, fmt.Errorf("ledger dir is empty")
	}
	s := &Server{ledgerDir: ledgerDir, mux: http.NewServeMux()}

	s.mux.Handle("GET /api/meta", http.HandlerFunc(s.handleMeta))
	s.mux.Handle("GET /api/sessions/{id}", http.HandlerFunc(s.handleSession))
	s.mux.Handle("GET /api/sessions", http.HandlerFunc(s.handleSessions))

	if devDir := os.Getenv("MOM_LENS_STATIC_DIR"); devDir != "" {
		s.mux.Handle("/", http.FileServer(http.Dir(devDir)))
	} else {
		sub, err := fs.Sub(staticFS, "static")
		if err != nil {
			return nil, fmt.Errorf("sub FS: %w", err)
		}
		s.mux.Handle("/", http.FileServer(http.FS(sub)))
	}

	return s, nil
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

type sessionData struct {
	SessionSummary
	ToolCalls map[string]ToolGroup
	Memories  []MemoryItem
}

type sessionBuild struct {
	id              string
	started         time.Time
	ended           time.Time
	interactions    int
	tools           map[string]int
	toolDetails     map[string]map[string]int
	totalTokens     int
	costUSD         float64
	provider        string
	model           string
	hasTurnObserved bool
}

// loadSessionData scans the whole Ledger once, grouping turn-observed and
// memory-recorded events by session. Everything shown is derived from the
// Ledger: tool calls, tokens, cost, and interactions from turn-observed
// payloads; memory counts from memory-recorded events.
func (s *Server) loadSessionData() (map[string]sessionData, int, error) {
	l, err := ledger.Open(s.ledgerDir)
	if err != nil {
		return nil, 0, fmt.Errorf("open ledger: %w", err)
	}
	defer func() { _ = l.Close() }()

	builds := map[string]*sessionBuild{}
	get := func(id string) *sessionBuild {
		b := builds[id]
		if b == nil {
			b = &sessionBuild{id: id, tools: map[string]int{}, toolDetails: map[string]map[string]int{}}
			builds[id] = b
		}
		return b
	}
	memIndex := map[string][]MemoryItem{}
	totalMemories := 0

	it := l.Iterate(0)
	defer func() { _ = it.Close() }()
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		ev := rec.Event
		sid := ev.SessionID
		if sid == "" {
			continue
		}
		// herald.Event.Timestamp is zero on historical records; fall
		// back to the Ledger's durable AppendedAt.
		created := ev.Timestamp
		if created.IsZero() {
			created = rec.AppendedAt
		}
		switch ev.Type {
		case herald.TurnObserved:
			b := get(sid)
			b.includeTime(created)
			applyTurnObserved(b, ev.Payload)
		case herald.MemoryRecord:
			b := get(sid)
			b.includeTime(created)
			summary := payloadStr(ev.Payload, "summary")
			if summary == "" {
				summary = payloadStr(ev.Payload, "content")
			}
			memIndex[sid] = append(memIndex[sid], MemoryItem{
				ID:      payloadStr(ev.Payload, "id"),
				Summary: summary,
				Created: created,
			})
			totalMemories++
		}
	}
	if err := it.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate ledger: %w", err)
	}

	out := map[string]sessionData{}
	for sid, b := range builds {
		memories := memIndex[sid]
		if memories == nil {
			memories = []MemoryItem{}
		}
		toolCalls, toolsTotal := toolGroups(b.tools, b.toolDetails)
		out[sid] = sessionData{
			SessionSummary: SessionSummary{
				SessionID:    sid,
				ScopeLabel:   centralScopeLabel,
				Started:      formatLensTime(b.started),
				Ended:        formatLensTime(b.ended),
				DurationSecs: durationSecs(b.started, b.ended),
				Interactions: b.interactions,
				MemoryCount:  len(memories),
				ToolsTotal:   toolsTotal,
				TotalTokens:  b.totalTokens,
				CostUSD:      b.costUSD,
				Provider:     b.provider,
				Model:        b.model,
			},
			ToolCalls: toolCalls,
			Memories:  memories,
		}
	}
	return out, totalMemories, nil
}

// payloadStr reads a string field from an event payload, returning "" if
// absent or not a string.
func payloadStr(payload map[string]any, key string) string {
	s, _ := payload[key].(string)
	return s
}

func (b *sessionBuild) includeTime(t time.Time) {
	if t.IsZero() {
		return
	}
	if b.started.IsZero() || t.Before(b.started) {
		b.started = t
	}
	if b.ended.IsZero() || t.After(b.ended) {
		b.ended = t
	}
}

func applyTurnObserved(b *sessionBuild, payload map[string]any) {
	if !b.hasTurnObserved {
		b.interactions = 0
		b.tools = map[string]int{}
		b.toolDetails = map[string]map[string]int{}
		b.totalTokens = 0
		b.costUSD = 0
	}
	b.hasTurnObserved = true
	if s, _ := payload["role"].(string); s == "assistant" {
		b.interactions++
	}
	if model, _ := payload["model"].(string); model != "" {
		b.model = model
	}
	if provider, _ := payload["provider"].(string); provider != "" {
		b.provider = provider
	}
	// tool_calls is the per-call array the watcher records on every turn:
	// [{name, category, safe_name, input}, ...]. Aggregate by category and,
	// within each category, by tool name. Prefer safe_name (privacy-projected)
	// over name, and never surface the raw `input`.
	for _, raw := range anySlice(payload["tool_calls"]) {
		tc, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cat := payloadStr(tc, "category")
		if cat == "" {
			cat = "uncategorized"
		}
		name := payloadStr(tc, "safe_name")
		if name == "" {
			name = payloadStr(tc, "name")
		}
		b.tools[cat]++
		if name != "" {
			if b.toolDetails[cat] == nil {
				b.toolDetails[cat] = map[string]int{}
			}
			b.toolDetails[cat][name]++
		}
	}
	if usage, ok := payload["usage"].(map[string]any); ok {
		b.totalTokens += intValue(usage["total_tokens"])
		b.costUSD += floatValue(usage["cost_usd"])
	}
}

// anySlice returns the value as a []any, or nil. JSON arrays decode to []any.
func anySlice(v any) []any {
	xs, _ := v.([]any)
	return xs
}

func intValue(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := strconv.Atoi(n.String())
		return i
	default:
		return 0
	}
}

func floatValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := strconv.ParseFloat(n.String(), 64)
		return f
	default:
		return 0
	}
}

func toolGroups(tools map[string]int, details map[string]map[string]int) (map[string]ToolGroup, int) {
	out := map[string]ToolGroup{}
	total := 0
	for cat, n := range tools {
		if n <= 0 {
			continue
		}
		detail := details[cat]
		if detail == nil {
			detail = map[string]int{}
		}
		out[cat] = ToolGroup{Total: n, Detail: detail}
		total += n
	}
	return out, total
}

func durationSecs(started, ended time.Time) float64 {
	if started.IsZero() || ended.IsZero() || !ended.After(started) {
		return 0
	}
	return math.Round(ended.Sub(started).Seconds())
}

func formatLensTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func sortedSessions(m map[string]sessionData) []SessionSummary {
	out := make([]SessionSummary, 0, len(m))
	for _, s := range m {
		out = append(out, s.SessionSummary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Started > out[j].Started
	})
	return out
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	sessions, totalMemories, err := s.loadSessionData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, MetaResponse{TotalSessions: len(sessions), TotalMemories: totalMemories})
}

func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	sessions, _, err := s.loadSessionData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, sortedSessions(sessions))
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}
	sessions, _, err := s.loadSessionData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	d, ok := sessions[id]
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, SessionDetail(d))
}

func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
