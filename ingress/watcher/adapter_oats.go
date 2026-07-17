package watcher

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OatsAdapter parses OATS (Open Agent Transcript Standard) JSONL
// transcript lines. Any OATS-conformant harness (momOS today, others
// later) writes one session per file under
//
//	~/.transcripts/<project-slug>/<timestamp>-<sid8>.jsonl
//
// Line 1 is the session header (type=="session") carrying the writing
// harness, model, and cwd. Lines 2+ are typed lines; in oats/1 the
// conversational ones are type=="message" with roles user/assistant/tool.
//
// Per OATS §2 (forward compatibility) lines with an unrecognized `type`
// are skipped. momOS's type=="event" extension lines (session.titled,
// delegation.spawned, ...) ARE recognized: ExtractEvent (EventExtractor)
// surfaces them as SessionEvents the watcher publishes as
// capture.event.observed.
//
// The adapter caches each session's header (keyed by the watcher-supplied
// session key, i.e. the filename stem) and stamps every subsequent Turn
// with the header's harness, model provider, and cwd — the source harness
// is attributed from the header's `harness` field, never hardcoded, so
// one adapter serves every OATS writer. The cwd stamp mirrors the Codex
// adapter: ~/.transcripts/ is shared across projects, so per-turn cwd is
// what lets the watcher resolve the correct project_id.
type OatsAdapter struct {
	mu           sync.Mutex
	headerBySess map[string]oatsSessionHeader
}

// NewOatsAdapter returns a new OatsAdapter.
func NewOatsAdapter() *OatsAdapter {
	return &OatsAdapter{headerBySess: map[string]oatsSessionHeader{}}
}

func (a *OatsAdapter) Name() string { return "oats" }

// ProjectSlug implements ProjectScoper. OATS buckets transcripts by a
// project slug derived from the git-root (or cwd) basename: lowercase,
// alphanumeric and '-' only, max 64 chars. Example:
// /Users/foo/projects/My-App → my-app.
//
// This differs from the Claude/Codex convention (full path with "/"→"-"),
// so without this override the watcher would never find the scoped
// subdirectory and would fall back to scanning all of ~/.transcripts/.
func (a *OatsAdapter) ProjectSlug(projectDir string) string {
	slug := strings.ToLower(filepath.Base(projectDir))
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// oatsSessionHeader is the subset of the OATS line-1 session header the
// adapter needs (harness attribution, model identity, cwd).
type oatsSessionHeader struct {
	Harness string `json:"harness"`
	Cwd     string `json:"cwd"`
	Model   struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
	} `json:"model"`
}

// oatsLine is the minimal subset of an OATS message line we inspect.
// The session header is parsed separately (oatsSessionHeader): its
// `model` field is an object where the message line's is a string, so
// one shared struct cannot decode both shapes.
type oatsLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"session_id"`
	Role      string          `json:"role"`
	Model     string          `json:"model"`
	Content   json.RawMessage `json:"content"`
	Usage     *oatsUsage      `json:"usage"`
	// Tool-result lines (role=="tool") only:
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

type oatsUsage struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// ExtractTurn implements Adapter. Returns the rich per-turn shape the
// Editor canonicalizes into a `turn.observed` event.
//
// Line handling:
//   - type=="session"  → cache the header for attribution, no Turn
//   - type=="message", role user/assistant → Turn
//   - type=="message", role "tool" → role:"tool" Turn carrying the
//     ToolResult (name, call_id, content, is_error)
//   - type=="event" → no Turn; surfaced separately via ExtractEvent
//   - any other type → skipped per OATS §2
func (a *OatsAdapter) ExtractTurn(line []byte, sessionID string) (Turn, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return Turn{}, false
	}
	// Decode the line type first: header and message lines have
	// incompatible field shapes (header `model` is an object, message
	// `model` is a string).
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return Turn{}, false
	}

	if head.Type == "session" {
		a.rememberHeader(sessionID, line)
		return Turn{}, false
	}
	if head.Type != "message" {
		// Unknown line type (momOS events, future extensions) — OATS §2:
		// readers MUST skip lines whose type they do not recognize.
		return Turn{}, false
	}

	var tl oatsLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return Turn{}, false
	}
	if tl.Role != "user" && tl.Role != "assistant" && tl.Role != "tool" {
		return Turn{}, false
	}

	header, hasHeader := a.recallHeader(sessionID)

	turn := Turn{
		SessionID: tl.SessionID,
		Role:      tl.Role,
		Model:     tl.Model,
		Provider:  header.Model.Provider,
		Harness:   header.Harness,
		Cwd:       header.Cwd,
	}
	if turn.SessionID == "" {
		turn.SessionID = sessionID
	}
	if turn.Model == "" {
		turn.Model = header.Model.ID
	}
	if !hasHeader {
		// Header not seen (e.g. the watcher resumed mid-file from a
		// cursor). Attribute generically rather than inventing a writer.
		turn.Harness = "oats"
	}

	// Timestamp: prefer the line's, fall back to now (Claude/Pi parity).
	if tl.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, tl.Timestamp); err == nil {
			turn.Timestamp = t
		} else if t, err := time.Parse(time.RFC3339, tl.Timestamp); err == nil {
			turn.Timestamp = t
		}
	}
	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now().UTC()
	}

	if tl.Role == "tool" {
		// Tool-result line: the result text rides on the ToolResult (with
		// error state), not on Turn.Text — the fold treats these as
		// non-prose and keeps them out of the vault; the Ledger keeps them.
		text, _ := walkOatsContent(tl.Content)
		turn.ToolResults = []ToolResult{{
			Name:    tl.Name,
			CallID:  tl.CallID,
			Content: truncateToolResult(text),
			IsError: tl.IsError,
		}}
		return turn, true
	}

	turn.Text, turn.ToolCalls = walkOatsContent(tl.Content)

	if tl.Usage != nil {
		u := &Usage{
			InputTokens:      tl.Usage.InputTokens,
			OutputTokens:     tl.Usage.OutputTokens,
			CacheReadTokens:  tl.Usage.CacheReadTokens,
			CacheWriteTokens: tl.Usage.CacheWriteTokens,
			CostUSD:          tl.Usage.CostUSD,
		}
		u.TotalTokens = u.InputTokens + u.OutputTokens
		turn.Usage = u
	}

	// Drop turns with no text and no tool calls — no signal for the vault.
	if turn.Text == "" && len(turn.ToolCalls) == 0 {
		return Turn{}, false
	}

	return turn, true
}

// oatsEventLine is the subset of a momOS type=="event" extension line
// the adapter extracts. momOS writes the event name under `event` and
// its details under `payload`.
type oatsEventLine struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	SessionID string         `json:"session_id"`
	Event     string         `json:"event"`
	Payload   map[string]any `json:"payload"`
}

// ExtractEvent implements EventExtractor. It surfaces OATS extension
// event lines (type=="event") — delegation.spawned, session.titled,
// session.archived, context.compacted, memory.folded, plan/todo
// updates — as SessionEvents the watcher publishes as
// capture.event.observed. All event names are captured generically;
// the payload rides verbatim.
func (a *OatsAdapter) ExtractEvent(line []byte, sessionID string) (SessionEvent, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return SessionEvent{}, false
	}
	var el oatsEventLine
	if err := json.Unmarshal(line, &el); err != nil {
		return SessionEvent{}, false
	}
	if el.Type != "event" || el.Event == "" {
		return SessionEvent{}, false
	}

	header, _ := a.recallHeader(sessionID)

	ev := SessionEvent{
		SessionID: el.SessionID,
		Name:      el.Event,
		Payload:   el.Payload,
		Harness:   header.Harness,
		Cwd:       header.Cwd,
	}
	if ev.SessionID == "" {
		ev.SessionID = sessionID
	}
	if ev.Harness == "" {
		ev.Harness = "oats"
	}
	if el.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, el.Timestamp); err == nil {
			ev.Timestamp = t
		} else if t, err := time.Parse(time.RFC3339, el.Timestamp); err == nil {
			ev.Timestamp = t
		}
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev, true
}

// PrimeSession implements SessionPrimer. When the watcher resumes a
// transcript from a non-zero cursor (process restart, cold header
// cache), the session header on line 1 sits before the cursor and would
// never be re-read — attribution (harness/provider/cwd) would degrade
// to the generic fallback. PrimeSession re-reads line 1 directly and
// caches it. No-op when the header is already cached.
func (a *OatsAdapter) PrimeSession(path string, sessionID string) {
	if _, ok := a.recallHeader(sessionID); ok {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64*1024)
	first, err := reader.ReadBytes('\n')
	if err != nil && len(first) == 0 {
		return
	}
	first = trimLine(first)
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(first, &head); err != nil || head.Type != "session" {
		return
	}
	a.rememberHeader(sessionID, first)
}

// rememberHeader parses and caches the session header line for the given
// session key so subsequent Turns can be attributed to the writing harness.
func (a *OatsAdapter) rememberHeader(sessionID string, line []byte) {
	var h oatsSessionHeader
	if err := json.Unmarshal(line, &h); err != nil {
		return
	}
	a.mu.Lock()
	a.headerBySess[sessionID] = h
	a.mu.Unlock()
}

func (a *OatsAdapter) recallHeader(sessionID string) (oatsSessionHeader, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	h, ok := a.headerBySess[sessionID]
	return h, ok
}

// walkOatsContent traverses an OATS content block array and returns:
//   - the concatenated text from text-typed blocks
//   - the tool calls extracted from tool_call-typed blocks (with
//     pre-computed Category)
//
// Other block types (image, file, future extensions) are ignored —
// readers must ignore what they do not recognize.
func walkOatsContent(raw json.RawMessage) (string, []ToolCall) {
	if len(raw) == 0 {
		return "", nil
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", nil
	}
	var (
		textParts []string
		tcs       []ToolCall
	)
	for _, m := range items {
		t, _ := m["type"].(string)
		switch t {
		case "text":
			if text, _ := m["text"].(string); text != "" {
				textParts = append(textParts, text)
			}
		case "tool_call":
			name, _ := m["name"].(string)
			if name == "" {
				continue
			}
			input, _ := m["arguments"].(map[string]any)
			category, safeName := CategorizeObservedToolCall(name, input)
			tcs = append(tcs, ToolCall{
				Name:     name,
				SafeName: safeName,
				Input:    input,
				Category: category,
			})
		}
	}
	return strings.Join(textParts, "\n"), tcs
}
