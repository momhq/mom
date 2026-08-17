// Package ingest serves the operational event ingress HTTP API (ADR 0025).
//
// It runs inside the `mom watch --global` daemon process, sharing the existing
// Editor + Ledger pipeline. Writes go exclusively through editor.Editor.Publish
// (never ledger.Append directly). Reads use the shared *ledger.Ledger handle,
// which is mutex-guarded in-process.
//
// Bind is loopback-only (copied from services/lens listener pattern).
// Default port 7475, with fallback to consecutive ports.
//
// Routes:
//
//	POST /api/ingest/events           — append one or many operational events
//	GET  /api/ingest/events           — iterate + filter ledger records
//	GET  /api/ingest/head             — current head offset
//	GET  /api/ingest/health           — health check
package ingest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/events/envelope"
	"github.com/momhq/mom/shared/buildinfo"
	"github.com/momhq/mom/storage/ledger"
)

// ingestSource is the provenance adapter name stamped by the Editor.
const ingestSource = "ingest"

// Server is the ingest HTTP server. Construct via New.
type Server struct {
	ed  *editor.Editor
	led *ledger.Ledger
	mux *http.ServeMux
}

// maxBodyBytes caps a single ingest request body. Operational events are
// small; this bounds memory against a hostile or runaway client.
const maxBodyBytes = 4 << 20 // 4 MiB

// New creates a Server using the shared Editor and Ledger from the watch
// daemon. Both may be nil, in which case the server starts but writes and
// reads return 503.
func New(ed *editor.Editor, led *ledger.Ledger) *Server {
	s := &Server{ed: ed, led: led, mux: http.NewServeMux()}
	s.mux.Handle("POST /api/ingest/events", http.HandlerFunc(s.handlePost))
	s.mux.Handle("GET /api/ingest/events", http.HandlerFunc(s.handleGet))
	s.mux.Handle("GET /api/ingest/head", http.HandlerFunc(s.handleHead))
	s.mux.Handle("GET /api/ingest/health", http.HandlerFunc(s.handleHealth))
	return s
}

// Handler returns the HTTP handler, wrapped so every request is guarded
// against cross-origin/DNS-rebinding abuse before reaching a route.
func (s *Server) Handler() http.Handler { return guardLocalOnly(s.mux) }

// guardLocalOnly rejects requests that a web page in the user's browser
// could use to reach this loopback daemon. The port is bound to loopback, but
// loopback is reachable from any site the user visits, so a bare bind is not
// enough:
//
//   - Host allowlist (localhost / 127.0.0.1 / [::1], any port) closes DNS
//     rebinding — a malicious domain that resolves to 127.0.0.1 arrives with
//     its own hostname in Host and is refused.
//   - Any cross-origin request carries an Origin header; since no browser
//     origin is ever legitimate for this internal API, a present Origin is a
//     refusal. (Native clients like momOS send none.)
//
// Without this, a visited page could POST attacker-controlled operational
// events that get folded verbatim into the user's LLM memory vault.
func guardLocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch host {
		case "localhost", "127.0.0.1", "::1", "":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "host not allowed", http.StatusForbidden)
		}
	})
}

// ---- request/response shapes ------------------------------------------------

// ingestItem is one event in a POST body: {type, payload}.
type ingestItem struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// postResponse is returned by a successful POST.
type postResponse struct {
	Appended int    `json:"appended"`
	Head     uint64 `json:"head"`
}

// getRecord is one record in the GET /api/ingest/events response.
type getRecord struct {
	Offset      uint64         `json:"offset"`
	AppendedAt  string         `json:"appended_at"`
	Type        string         `json:"type"`
	Payload     map[string]any `json:"payload"`
}

// getResponse is returned by GET /api/ingest/events.
type getResponse struct {
	Records []getRecord `json:"records"`
	Next    *uint64     `json:"next"`
	Head    uint64      `json:"head"`
}

// headResponse is returned by GET /api/ingest/head.
type headResponse struct {
	Head uint64 `json:"head"`
}

// healthResponse is returned by GET /api/ingest/health.
type healthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}

// ---- handlers ---------------------------------------------------------------

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	if s.ed == nil || s.led == nil {
		http.Error(w, "ingest not ready", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Accept both a single object and an array.
	var items []ingestItem
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &items); err != nil {
			http.Error(w, "invalid JSON array: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		var single ingestItem
		if err := json.Unmarshal(raw, &single); err != nil {
			http.Error(w, "invalid JSON object: "+err.Error(), http.StatusBadRequest)
			return
		}
		items = []ingestItem{single}
	}

	if len(items) == 0 {
		http.Error(w, "empty batch", http.StatusBadRequest)
		return
	}

	// Validate all items before publishing any.
	for i, item := range items {
		if !strings.HasPrefix(item.Type, "operational.") {
			http.Error(w, fmt.Sprintf("item[%d]: type %q is not in the operational.* family", i, item.Type), http.StatusBadRequest)
			return
		}
		pid, _ := item.Payload["project_id"].(string)
		if pid == "" {
			http.Error(w, fmt.Sprintf("item[%d]: payload must include project_id", i), http.StatusBadRequest)
			return
		}
	}

	// Publish each item through the Editor.
	appended := 0
	for _, item := range items {
		c := &rawCanonicalizer{eventType: envelope.EventType(item.Type), payload: item.Payload}
		src := editor.Source{Adapter: ingestSource}
		if err := s.ed.Publish(c, src); err != nil {
			http.Error(w, fmt.Sprintf("publish failed: %v", err), http.StatusInternalServerError)
			return
		}
		appended++
	}

	head := uint64(0)
	if h, ok := s.led.HeadOffset(); ok {
		head = h
	}
	jsonResponse(w, postResponse{Appended: appended, Head: head})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if s.led == nil {
		http.Error(w, "ingest not ready", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	from := uint64(0)
	if raw := q.Get("from"); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			http.Error(w, "from must be a non-negative integer", http.StatusBadRequest)
			return
		}
		from = v
	}

	projectFilter := q.Get("project")
	typePrefix := q.Get("type_prefix")
	if typePrefix == "" {
		typePrefix = "operational."
	}

	limit := 100
	if raw := q.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		if v > 1000 {
			v = 1000
		}
		limit = v
	}

	head := uint64(0)
	if h, ok := s.led.HeadOffset(); ok {
		head = h
	}

	it := s.led.Iterate(from)
	defer func() { _ = it.Close() }()

	var records []getRecord
	var nextOffset *uint64

	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		evType := string(rec.Event.Type)
		if !strings.HasPrefix(evType, typePrefix) {
			continue
		}
		if projectFilter != "" {
			pid, _ := rec.Event.Payload["project_id"].(string)
			if pid != projectFilter {
				continue
			}
		}
		if len(records) >= limit {
			n := rec.Offset
			nextOffset = &n
			break
		}
		records = append(records, getRecord{
			Offset:     rec.Offset,
			AppendedAt: rec.AppendedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
			Type:       evType,
			Payload:    rec.Event.Payload,
		})
	}
	if err := it.Err(); err != nil {
		http.Error(w, fmt.Sprintf("iterate ledger: %v", err), http.StatusInternalServerError)
		return
	}

	if records == nil {
		records = []getRecord{}
	}
	jsonResponse(w, getResponse{Records: records, Next: nextOffset, Head: head})
}

func (s *Server) handleHead(w http.ResponseWriter, _ *http.Request) {
	if s.led == nil {
		http.Error(w, "ingest not ready", http.StatusServiceUnavailable)
		return
	}
	head := uint64(0)
	if h, ok := s.led.HeadOffset(); ok {
		head = h
	}
	jsonResponse(w, headResponse{Head: head})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, healthResponse{OK: true, Version: buildinfo.Version})
}

// ---- helpers ----------------------------------------------------------------

// rawCanonicalizer wraps a pre-parsed (type, payload) pair so the Editor
// can handle it without type-switching on domain types.
type rawCanonicalizer struct {
	eventType envelope.EventType
	payload   map[string]any
}

func (c *rawCanonicalizer) Canonical() (envelope.EventType, map[string]any) {
	return c.eventType, c.payload
}

func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
