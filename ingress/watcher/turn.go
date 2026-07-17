package watcher

import (
	"time"

	"github.com/momhq/mom/events/envelope"
)

// Turn is the per-turn structured payload emitted by the watcher's
// adapters. It carries the full turn — raw text, tool calls (name,
// category, input), usage, model, provider.
//
// The Editor canonicalizes the Turn into a `turn.observed` event and
// appends it to the Ledger. The projection fold and the lens read paths
// decide what to surface; raw text and tool inputs are not shown by the
// vault. See PRD 0003 + ADR 0014 for the privacy contract.
type Turn struct {
	SessionID   string
	Timestamp   time.Time
	Role        string // "user" | "assistant" | "tool"
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
	Usage       *Usage

	// Three orthogonal identity fields, each answering a different
	// question. Any may be empty when the source transcript does not
	// surface the data.
	Model    string // "the model": e.g. "claude-sonnet-4-6", "gpt-4o"
	Provider string // "provided by whom": model vendor — "anthropic", "openai", …
	Harness  string // "used in which client": "claude-code", "codex", "pi"

	// ProjectId carries the resolved project identity (ADR 0016).
	// Empty means "unknown" — the resolver found no .mom-project.yaml.
	ProjectId string

	// Cwd is the working directory the harness reported for this turn,
	// when the transcript carries it (Codex includes per-turn cwd in
	// `turn_context` envelopes). The watcher prefers this for project
	// resolution over the watcher's configured ProjectDir — critical when
	// multiple projects share a global transcript directory (Codex).
	Cwd string
}

// ToolCall is one tool invocation observed in an assistant turn.
// `Input` carries the raw tool arguments (file paths, shell commands,
// etc.). `Category` and SafeName are pre-computed by the adapter so read
// paths (lens) can show privacy-safe per-tool analytics without inspecting
// raw inputs.
type ToolCall struct {
	Name     string
	SafeName string
	Input    map[string]any
	Category string // "mom_memory" | "mom_cli" | "codebase_read" | "codebase_write" | "system"
}

// ToolResult is the outcome of one tool invocation, observed on a
// role:"tool" transcript line (OATS) or a tool_result content block
// (Claude Code). `Content` carries the result text, truncated to
// maxToolResultChars by the adapters — tool outputs (full file reads,
// command output) can be megabytes and would bloat the Ledger without
// adding vault signal. `IsError` preserves the error state so consumers
// can distinguish failed from successful tool runs.
type ToolResult struct {
	Name    string // tool name, when the harness surfaces it on the result
	CallID  string // back-reference to the originating tool call id
	Content string
	IsError bool
}

// maxToolResultChars caps the result text an adapter carries on a
// ToolResult. Results beyond the cap are truncated with a marker.
const maxToolResultChars = 4096

// truncateToolResult enforces maxToolResultChars on result content.
func truncateToolResult(s string) string {
	if len(s) <= maxToolResultChars {
		return s
	}
	return s[:maxToolResultChars] + "\n…[truncated]"
}

// Usage carries token-accounting numbers for a single turn. Optional —
// not every harness surfaces it (e.g. user turns rarely have usage).
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalTokens      int
	CostUSD          float64
	StopReason       string
}

// ToPayload renders a Turn into the map[string]any payload of the
// turn.observed event appended to the Ledger. The projection fold and the
// lens read these keys; the map convention is documented here so readers
// don't reinvent extraction.
//
// Keys: "role", "text", "tool_calls" ([]map with name/input/category),
// "tool_results" ([]map with name/call_id/content/is_error), "usage"
// (map of token counts), "model", "provider", "harness", "project_id".
func (t Turn) ToPayload() map[string]any {
	out := map[string]any{
		"role": t.Role,
	}
	if t.Text != "" {
		out["text"] = t.Text
	}
	if len(t.ToolCalls) > 0 {
		tcs := make([]map[string]any, 0, len(t.ToolCalls))
		for _, tc := range t.ToolCalls {
			m := map[string]any{
				"name":     tc.Name,
				"category": tc.Category,
			}
			if tc.SafeName != "" {
				m["safe_name"] = tc.SafeName
			}
			if tc.Input != nil {
				m["input"] = tc.Input
			}
			tcs = append(tcs, m)
		}
		out["tool_calls"] = tcs
	}
	if len(t.ToolResults) > 0 {
		trs := make([]map[string]any, 0, len(t.ToolResults))
		for _, tr := range t.ToolResults {
			m := map[string]any{
				"is_error": tr.IsError,
			}
			if tr.Name != "" {
				m["name"] = tr.Name
			}
			if tr.CallID != "" {
				m["call_id"] = tr.CallID
			}
			if tr.Content != "" {
				m["content"] = tr.Content
			}
			trs = append(trs, m)
		}
		out["tool_results"] = trs
	}
	if t.Usage != nil {
		out["usage"] = map[string]any{
			"input_tokens":       t.Usage.InputTokens,
			"output_tokens":      t.Usage.OutputTokens,
			"cache_read_tokens":  t.Usage.CacheReadTokens,
			"cache_write_tokens": t.Usage.CacheWriteTokens,
			"total_tokens":       t.Usage.TotalTokens,
			"cost_usd":           t.Usage.CostUSD,
			"stop_reason":        t.Usage.StopReason,
		}
	}
	if t.Model != "" {
		out["model"] = t.Model
	}
	if t.Provider != "" {
		out["provider"] = t.Provider
	}
	if t.Harness != "" {
		out["harness"] = t.Harness
	}
	if t.ProjectId != "" {
		out["project_id"] = t.ProjectId
	}
	return out
}

// Canonical implements editor.Canonicalizer. It exposes Turn as a
// canonical envelope.TurnObserved event whose payload is the ToPayload()
// shape. The Editor (ADR 0020) layers provenance + project_id + schema
// validation on top before appending to the Ledger.
func (t Turn) Canonical() (envelope.EventType, map[string]any) {
	payload := t.ToPayload()
	if t.SessionID != "" {
		payload["session_id"] = t.SessionID
	}
	return envelope.TurnObserved, payload
}
