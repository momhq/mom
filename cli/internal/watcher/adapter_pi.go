package watcher

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/recorder"
)

// PiAdapter parses pi (https://github.com/mariozechner/pi) JSONL session files.
//
// Pi writes one JSON object per line to
//
//	~/.pi/agent/sessions/<project-slug>/<timestamp>_<sessionId>.jsonl
//
// The project slug uses the same "/" → "-" convention as Claude Code, so the
// existing projectSlug() scoping logic in watcher.go applies unchanged.
//
// Line schema (the entries we care about):
//
//	{
//	  "type":      "message",
//	  "id":        "<short-id>",
//	  "parentId":  "<short-id|null>",
//	  "timestamp": "2026-04-28T00:11:01.063Z",
//	  "message": {
//	    "role":      "user" | "assistant",
//	    "content":   string | [ {type:"text",text:"..."} | {type:"tool_use",...} | ... ],
//	    "timestamp": <unix-ms>
//	  }
//	}
//
// Other top-level "type" values exist (session, thinking_level_change,
// model_change, ...). They carry no conversational text and are dropped.
type PiAdapter struct{}

// NewPiAdapter returns a new PiAdapter.
func NewPiAdapter() *PiAdapter {
	return &PiAdapter{}
}

func (a *PiAdapter) Name() string { return "pi" }

// ProjectSlug implements ProjectScoper. Pi uses a different per-project
// directory convention than Claude/Codex: it strips the leading separator,
// replaces remaining path separators and colons with '-', and wraps the
// result with '--' on both sides.
//
// Example: /Users/foo/proj  →  --Users-foo-proj--
//
// Source-of-truth: pi-coding-agent dist/migrations.js, which builds the
// directory path as:
//
//	const safePath = `--${cwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--`;
//
// We mirror that rule exactly so the watcher's project-scoping check finds
// pi's actual session subdirectory and does not fall back to scanning all
// projects' sessions globally.
func (a *PiAdapter) ProjectSlug(projectDir string) string {
	p := projectDir
	// Strip leading path separator (Unix '/' or Windows '\').
	if len(p) > 0 && (p[0] == '/' || p[0] == '\\') {
		p = p[1:]
	}
	// Replace remaining separators and colons with '-'.
	p = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(p)
	return "--" + p + "--"
}

// ParseSession parses a pi session JSONL file into a logbook.SessionLog.
//
// Pi's transcript schema differs from Claude Code's in two ways that matter
// for metrics extraction:
//
//   1. Tool calls appear as content blocks with type "toolCall" (not
//      "tool_use"), and the arguments live under "arguments" (not "input").
//      Delegating to logbook.ParseTranscript would silently miss every pi
//      tool call.
//   2. Each assistant message carries rich per-turn metadata in-band:
//      message.{model, provider, stopReason, usage{input,output,cacheRead,
//      cacheWrite,totalTokens,cost{...total}}}. We aggregate it across the
//      session and surface it on the resulting SessionLog.
func (a *PiAdapter) ParseSession(transcriptPath, sessionID string) (*logbook.SessionLog, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	log := &logbook.SessionLog{
		SessionID: sessionID,
		ToolCalls: make(map[string]logbook.ToolGroup),
	}

	var usage logbook.UsageAggregate
	stopReasons := make(map[string]int)
	filesChanged := make(map[string]bool)
	var firstTS, lastTS string
	var haveUsage bool

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		var entry piSessionEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Timestamp != "" {
			if firstTS == "" {
				firstTS = entry.Timestamp
			}
			lastTS = entry.Timestamp
		}

		if entry.Type != "message" {
			continue
		}

		msg := entry.Message
		if msg.Role == "assistant" {
			log.Interactions++

			// First assistant message wins for model/provider — sessions can
			// switch model mid-flight (Ctrl+P), but we report the dominant one
			// downstream; for now keep the first-seen value.
			if log.Model == "" && msg.Model != "" {
				log.Model = msg.Model
			}
			if log.Provider == "" && msg.Provider != "" {
				log.Provider = msg.Provider
			}
			if msg.StopReason != "" {
				stopReasons[msg.StopReason]++
			}

			if msg.Usage != nil {
				haveUsage = true
				usage.InputTokens += msg.Usage.Input
				usage.OutputTokens += msg.Usage.Output
				usage.CacheReadTokens += msg.Usage.CacheRead
				usage.CacheWriteTokens += msg.Usage.CacheWrite
				usage.TotalTokens += msg.Usage.TotalTokens
				if msg.Usage.Cost != nil {
					usage.CostUSD += msg.Usage.Cost.Total
				}
			}
		}

		// Walk content blocks for tool calls.
		for _, b := range msg.Content {
			if b.Type != "toolCall" {
				continue
			}
			name := b.Name
			if name == "" {
				continue
			}

			normalized := logbook.NormalizeToolName(name)
			category := logbook.CategorizeToolCall(name)
			group := log.ToolCalls[category]
			group.Total++
			if group.Detail == nil {
				group.Detail = make(map[string]int)
			}
			group.Detail[name]++
			log.ToolCalls[category] = group

			// File-changed tracking: pi puts the path under "arguments".
			// Different tools use different key names; check the common ones.
			if isPiCodebaseWrite(normalized) {
				if fp := piExtractPath(b.Arguments); fp != "" {
					filesChanged[fp] = true
				}
			}

			if normalized == "create_memory_draft" {
				log.MemoriesCreated++
			}
		}
	}

	if firstTS == "" {
		firstTS = time.Now().UTC().Format(time.RFC3339)
	}
	if lastTS == "" {
		lastTS = firstTS
	}

	log.Started = firstTS
	log.Ended = lastTS
	log.FilesChanged = len(filesChanged)

	if haveUsage {
		if len(stopReasons) > 0 {
			usage.StopReasons = stopReasons
		}
		log.Usage = &usage
	} else if len(stopReasons) > 0 {
		// Stop reasons but no usage — still worth surfacing.
		log.Usage = &logbook.UsageAggregate{StopReasons: stopReasons}
	}

	return log, nil
}

// piSessionEntry mirrors the subset of a pi session line ParseSession reads.
type piSessionEntry struct {
	Type      string           `json:"type"`
	Timestamp string           `json:"timestamp"`
	Message   piSessionMessage `json:"message"`
}

type piSessionMessage struct {
	Role       string         `json:"role"`
	Content    []piContentRaw `json:"content"`
	Model      string         `json:"model"`
	Provider   string         `json:"provider"`
	StopReason string         `json:"stopReason"`
	Usage      *piUsage       `json:"usage"`
}

// piContentRaw is loosely typed because the same field name ("content")
// can be a string OR an array; for ParseSession we only inspect the array
// case (block list). The watcher's ParseLine path handles the string case
// separately via extractPiContent.
type piContentRaw struct {
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type piUsage struct {
	Input       int      `json:"input"`
	Output      int      `json:"output"`
	CacheRead   int      `json:"cacheRead"`
	CacheWrite  int      `json:"cacheWrite"`
	TotalTokens int      `json:"totalTokens"`
	Cost        *piCost  `json:"cost"`
}

type piCost struct {
	Total float64 `json:"total"`
}

// isPiCodebaseWrite mirrors logbook.isCodebaseWrite for the lowercase tool
// names pi emits (read, write, edit, bash, ...). We can't import the
// unexported logbook helper; this stays in sync with it.
func isPiCodebaseWrite(name string) bool {
	return name == "edit" || name == "Edit" || name == "write" || name == "Write"
}

// piExtractPath pulls a file path out of a tool-call arguments map. Pi tools
// use various keys for the target path; we check the common ones.
func piExtractPath(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, key := range []string{"file_path", "path", "filename", "file"} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// piTranscriptLine is the minimal subset of a pi session line we inspect.
type piTranscriptLine struct {
	Type      string    `json:"type"`
	Timestamp string    `json:"timestamp"`
	Message   piMessage `json:"message"`
}

type piMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []map[string]any
}

// ExtractTurn delegates to ParseLine for now and synthesises a thin
// Turn from the resulting RawEntry. Pi-specific tool_use / usage
// extraction will land in a follow-up slice; the legacy Pi
// SessionParser already covers session-end aggregates so the
// metadata projection has fallback data.
func (a *PiAdapter) ExtractTurn(line []byte, sessionID string) (Turn, bool) {
	entry, ok := a.ParseLine(line, sessionID)
	if !ok {
		return Turn{}, false
	}
	return Turn{
		SessionID: entry.SessionID,
		Timestamp: time.Now().UTC(),
		Role:      strings.TrimPrefix(entry.Event, "watch-"),
		Text:      entry.Text,
		Provider:  "pi",
	}, true
}

// ParseLine parses one JSONL line. Returns a RawEntry only for user/assistant
// message entries with non-empty text content.
func (a *PiAdapter) ParseLine(line []byte, sessionID string) (recorder.RawEntry, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return recorder.RawEntry{}, false
	}

	var tl piTranscriptLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return recorder.RawEntry{}, false
	}

	// Drop everything except conversational message lines.
	if tl.Type != "message" {
		return recorder.RawEntry{}, false
	}
	if tl.Message.Role != "user" && tl.Message.Role != "assistant" {
		return recorder.RawEntry{}, false
	}

	text := extractPiContent(tl.Message.Content)
	if text == "" {
		return recorder.RawEntry{}, false
	}

	ts := tl.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	// Pi messages don't carry a session id at the line level — use the
	// filename-derived one passed by the watcher.
	return recorder.RawEntry{
		Timestamp: ts,
		Event:     "watch-" + tl.Message.Role,
		Text:      text,
		SessionID: sessionID,
	}, true
}

// extractPiContent flattens pi's content field to plain text.
//
// pi content can be:
//   - a plain string (rare, but the schema allows it)
//   - an array of blocks: {type:"text",text:"..."}, {type:"tool_use",...},
//     {type:"tool_result",...}, {type:"thinking",...}, {type:"image",...}
//
// We keep only "text" blocks. Everything else (tool I/O, thinking, images)
// is intentionally dropped to match the Claude adapter's behavior and keep
// .mom/raw/ focused on conversational signal.
func extractPiContent(content any) string {
	if content == nil {
		return ""
	}

	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}

	items, ok := content.([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != "text" {
			continue
		}
		if text, _ := m["text"].(string); text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n")
}
