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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/memory"
)

//go:embed static
var staticFS embed.FS

// ScopeEntry is a single .mom/ scope passed to New.
type ScopeEntry struct {
	Label string
	Path  string
}

// ScopeInfo is the API representation of a scope, including live counts.
// Key is the stable index string used for ?scope= filtering ("0", "1", ...).
// PathHint is the parent directory name, used to disambiguate duplicate labels.
type ScopeInfo struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	PathHint      string `json:"path_hint"`
	TotalSessions int    `json:"total_sessions"`
	TotalMemories int    `json:"total_memories"`
}

// SessionSummary is the list-view shape returned by GET /api/sessions.
type SessionSummary struct {
	SessionID    string  `json:"session_id"`
	ScopeLabel   string  `json:"scope_label"`
	Started      string  `json:"started"`
	Ended        string  `json:"ended"`
	DurationSecs float64 `json:"duration_secs"`
	Interactions int     `json:"interactions"`
	MemoryCount  int     `json:"memory_count"`
	CuratedCount int     `json:"curated_count"`
	ToolsTotal   int     `json:"tools_total"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
}

// MemoryItem is a memory document linked to a session.
type MemoryItem struct {
	ID             string    `json:"id"`
	Summary        string    `json:"summary"`
	Tags           []string  `json:"tags"`
	Scope          string    `json:"scope"`
	PromotionState string    `json:"promotion_state"`
	Created        time.Time `json:"created"`
	Landmark       bool      `json:"landmark"`
}

// SessionDetail extends SessionSummary with the full per-session breakdown.
type SessionDetail struct {
	SessionSummary
	ToolCalls map[string]logbook.ToolGroup `json:"tool_calls"`
	Memories  []MemoryItem                 `json:"memories"`
}

// MetaResponse is returned by GET /api/meta.
type MetaResponse struct {
	Scopes        []ScopeInfo `json:"scopes"`
	TotalSessions int         `json:"total_sessions"`
	TotalMemories int         `json:"total_memories"`
}

// scopeData carries scope identity. Memory and session data are read fresh
// on each request — see loadMemoryIndex and loadSessions.
type scopeData struct {
	entry ScopeEntry
	key   string // "0", "1", ... set by New()
}

// Server serves the lens dashboard.
type Server struct {
	scopes []*scopeData
	mux    *http.ServeMux
}

// New creates a Server for the given list of scopes (nearest-first).
func New(scopes []ScopeEntry) (*Server, error) {
	s := &Server{mux: http.NewServeMux()}

	for i, e := range scopes {
		s.scopes = append(s.scopes, &scopeData{entry: e, key: strconv.Itoa(i)})
	}

	s.mux.Handle("GET /api/meta", http.HandlerFunc(s.handleMeta))
	s.mux.Handle("GET /api/sessions/{id}", http.HandlerFunc(s.handleSession))
	s.mux.Handle("GET /api/sessions", http.HandlerFunc(s.handleSessions))

	// MOM_LENS_STATIC_DIR: serve files from disk for development iteration.
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

// loadMemoryIndex scans the scope's memory dir and returns a session_id →
// memories map plus the total memory count. Called per request — cheap for
// local dirs and avoids stale state when memories are written while lens runs.
func (sd *scopeData) loadMemoryIndex() (map[string][]MemoryItem, int) {
	index := make(map[string][]MemoryItem)
	total := 0
	memDir := filepath.Join(sd.entry.Path, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return index, 0
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		total++
		if doc.SessionID != "" {
			index[doc.SessionID] = append(index[doc.SessionID], MemoryItem{
				ID:             doc.ID,
				Summary:        doc.Summary,
				Tags:           doc.Tags,
				Scope:          doc.Scope,
				PromotionState: doc.PromotionState,
				Created:        doc.Created,
				Landmark:       doc.Landmark,
			})
		}
	}
	return index, total
}

// loadSessions returns sessions for a single scope, joined with the live
// memory index for that scope.
func (sd *scopeData) loadSessions() ([]SessionSummary, int, error) {
	memIndex, totalMemories := sd.loadMemoryIndex()

	logsDir := filepath.Join(sd.entry.Path, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, totalMemories, nil
		}
		return nil, totalMemories, err
	}

	var sessions []SessionSummary
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "session-") || filepath.Ext(name) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logsDir, name))
		if err != nil {
			continue
		}
		var sl logbook.SessionLog
		if err := json.Unmarshal(data, &sl); err != nil {
			continue
		}
		sessions = append(sessions, sd.toSummary(&sl, memIndex))
	}
	return sessions, totalMemories, nil
}

func (sd *scopeData) toSummary(sl *logbook.SessionLog, memIndex map[string][]MemoryItem) SessionSummary {
	dur := 0.0
	t1, err1 := time.Parse(time.RFC3339, sl.Started)
	t2, err2 := time.Parse(time.RFC3339, sl.Ended)
	if err1 == nil && err2 == nil && t2.After(t1) {
		dur = math.Round(t2.Sub(t1).Seconds())
	}

	toolsTotal := 0
	for _, g := range sl.ToolCalls {
		toolsTotal += g.Total
	}

	totalTokens := 0
	costUSD := 0.0
	if sl.Usage != nil {
		totalTokens = sl.Usage.TotalTokens
		costUSD = sl.Usage.CostUSD
	}

	memories := memIndex[sl.SessionID]
	curatedCount := 0
	for _, m := range memories {
		if m.PromotionState == "curated" || m.Landmark {
			curatedCount++
		}
	}

	return SessionSummary{
		SessionID:    sl.SessionID,
		ScopeLabel:   sd.entry.Label,
		Started:      sl.Started,
		Ended:        sl.Ended,
		DurationSecs: dur,
		Interactions: sl.Interactions,
		MemoryCount:  len(memories),
		CuratedCount: curatedCount,
		ToolsTotal:   toolsTotal,
		TotalTokens:  totalTokens,
		CostUSD:      costUSD,
		Provider:     sl.Provider,
		Model:        sl.Model,
	}
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	var infos []ScopeInfo
	seenIDs := make(map[string]bool)
	totalSessions, totalMemories := 0, 0
	for _, sd := range s.scopes {
		sessions, scopeMemories, _ := sd.loadSessions()
		infos = append(infos, ScopeInfo{
			Key:           sd.key,
			Label:         sd.entry.Label,
			PathHint:      filepath.Base(filepath.Dir(sd.entry.Path)),
			TotalSessions: len(sessions),
			TotalMemories: scopeMemories,
		})
		for _, s := range sessions {
			if !seenIDs[s.SessionID] {
				seenIDs[s.SessionID] = true
				totalSessions++
			}
		}
		totalMemories += scopeMemories
	}
	jsonResponse(w, MetaResponse{
		Scopes:        infos,
		TotalSessions: totalSessions,
		TotalMemories: totalMemories,
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	scopeFilter := r.URL.Query().Get("scope")

	var all []SessionSummary
	for _, sd := range s.scopes {
		if scopeFilter != "" && scopeFilter != "all" && sd.key != scopeFilter {
			continue
		}
		sessions, _, err := sd.loadSessions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		all = append(all, sessions...)
	}

	// Deduplicate by session ID: keep first occurrence (nearest scope wins,
	// since scopes are ordered nearest-first by scope.Walk).
	seen := make(map[string]bool, len(all))
	deduped := all[:0]
	for _, s := range all {
		if !seen[s.SessionID] {
			seen[s.SessionID] = true
			deduped = append(deduped, s)
		}
	}
	all = deduped

	sort.Slice(all, func(i, j int) bool {
		return all[i].Started > all[j].Started
	})

	if all == nil {
		all = []SessionSummary{}
	}
	jsonResponse(w, all)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	// Search all scopes for the session log (IDs are globally unique).
	for _, sd := range s.scopes {
		data, err := os.ReadFile(filepath.Join(sd.entry.Path, "logs", "session-"+id+".json"))
		if err != nil {
			continue
		}
		var sl logbook.SessionLog
		if err := json.Unmarshal(data, &sl); err != nil {
			http.Error(w, "malformed session log", http.StatusInternalServerError)
			return
		}
		memIndex, _ := sd.loadMemoryIndex()
		memories := memIndex[id]
		if memories == nil {
			memories = []MemoryItem{}
		}
		jsonResponse(w, SessionDetail{
			SessionSummary: sd.toSummary(&sl, memIndex),
			ToolCalls:      sl.ToolCalls,
			Memories:       memories,
		})
		return
	}

	http.Error(w, "session not found", http.StatusNotFound)
}

func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
