package projection

import (
	"fmt"
	"strings"

	"github.com/momhq/mom/events/envelope"
	"github.com/momhq/mom/storage/ledger"
)

// Reader projects Ledger Records for one project into []FoldEvent. It is
// a read-only consumer: it Opens the Ledger, iterates from fromOffset+1,
// keeps only this project's turn/memory events (dropping tool-noise
// turns), and reports the head offset reached.
type Reader struct {
	ledgerDir string
	projectID string
}

// NewReader builds a Reader bound to a ledger directory and a project id.
func NewReader(ledgerDir, projectID string) *Reader {
	return &Reader{ledgerDir: ledgerDir, projectID: projectID}
}

// ReadResult carries the folded events and the head offset reached. Head
// is the offset of the last Record observed (the new watermark); it is
// fromOffset when no new records exist.
type ReadResult struct {
	Events []FoldEvent
	Head   uint64
}

// Read iterates the Ledger from fromOffset+1, filtering to this project's
// turn-observed and memory-recorded events. fromOffset is the last folded
// offset (0 means "from the very start" — see PendingFrom for the +1
// resume semantics; passing 0 here folds everything).
func (r *Reader) Read(fromOffset uint64) (ReadResult, error) {
	l, err := ledger.Open(r.ledgerDir)
	if err != nil {
		return ReadResult{}, fmt.Errorf("open ledger: %w", err)
	}
	defer func() { _ = l.Close() }()

	// Resume strictly AFTER the last folded offset. On a fresh rebuild
	// (fromOffset == 0) we still want offset 0, so start at 0 in that case.
	start := fromOffset + 1
	if fromOffset == 0 {
		start = 0
	}

	head := fromOffset
	var events []FoldEvent

	it := l.Iterate(start)
	defer func() { _ = it.Close() }()
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		if rec.Offset > head {
			head = rec.Offset
		}
		fe, keep := r.convert(rec)
		if keep {
			events = append(events, fe)
		}
	}
	if err := it.Err(); err != nil {
		return ReadResult{}, fmt.Errorf("iterate ledger: %w", err)
	}
	return ReadResult{Events: events, Head: head}, nil
}

// PendingCount returns how many of this project's foldable events sit in
// the Ledger after fromOffset, plus the current head offset. Used by
// `mom vault status`.
func (r *Reader) PendingCount(fromOffset uint64) (int, uint64, error) {
	res, err := r.Read(fromOffset)
	if err != nil {
		return 0, 0, err
	}
	return len(res.Events), res.Head, nil
}

// convert maps a Ledger Record to a FoldEvent, returning keep=false for
// records that belong to another project or are tool-noise turns.
func (r *Reader) convert(rec ledger.Record) (FoldEvent, bool) {
	ev := rec.Event
	if projectIDOf(ev) != r.projectID {
		return FoldEvent{}, false
	}

	// envelope.Event.Timestamp is zero on historical records; the Ledger's
	// AppendedAt is the durable wall-clock time. Prefer Timestamp, fall
	// back to AppendedAt.
	created := ev.Timestamp
	if created.IsZero() {
		created = rec.AppendedAt
	}

	switch ev.Type {
	case envelope.TurnObserved:
		role := payloadString(ev.Payload, "role")
		text := payloadString(ev.Payload, "text")
		if isToolNoise(text) {
			return FoldEvent{}, false
		}
		return FoldEvent{
			Offset:    rec.Offset,
			Type:      string(ev.Type),
			SessionID: ev.SessionID,
			CreatedAt: created,
			Role:      role,
			Text:      text,
		}, true
	case envelope.MemoryRecord:
		content := payloadString(ev.Payload, "content")
		summary := payloadString(ev.Payload, "summary")
		return FoldEvent{
			Offset:    rec.Offset,
			Type:      string(ev.Type),
			SessionID: ev.SessionID,
			CreatedAt: created,
			Text:      content,
			Summary:   summary,
			Tags:      payloadStrings(ev.Payload, "tags"),
		}, true

	// Operational events — ADR 0025.
	// operational.message.posted: surface prose kinds in the fold; drop
	// tool_result and system (harness machinery, not company record prose).
	case envelope.OperationalMessagePosted:
		kind := payloadString(ev.Payload, "kind")
		switch kind {
		case "chat", "plan", "handoff", "artifact", "decision":
			senderName := payloadString(ev.Payload, "sender_name")
			senderType := payloadString(ev.Payload, "sender_type")
			body := payloadString(ev.Payload, "body")
			return FoldEvent{
				Offset:    rec.Offset,
				Type:      "os.message",
				SessionID: ev.SessionID,
				CreatedAt: created,
				Role:      senderName + " (" + senderType + ")",
				Text:      "[" + kind + "] " + body,
			}, true
		default:
			// tool_result, system, and any future kinds are dropped.
			return FoldEvent{}, false
		}

	// operational.approval.resolved: surface approval outcomes in the fold.
	case envelope.OperationalApprovalResolved:
		status := payloadString(ev.Payload, "status")
		note := payloadString(ev.Payload, "note")
		text := "approval " + status
		if note != "" {
			text += ": " + note
		}
		return FoldEvent{
			Offset:    rec.Offset,
			Type:      "os.approval",
			SessionID: ev.SessionID,
			CreatedAt: created,
			Text:      text,
		}, true

	default:
		// All other operational.* events (run.started, run.finished,
		// approval.requested, task.created, task.updated,
		// workspace.registered) remain in the Ledger but are not folded.
		return FoldEvent{}, false
	}
}

// projectIDOf reads the project_id payload key, tolerating either a
// top-level SessionID-style field absence.
func projectIDOf(ev envelope.Event) string {
	return payloadString(ev.Payload, "project_id")
}

// isToolNoise reports whether a turn carries no human/agent prose worth
// folding: empty text, or text that is only a tool-call marker.
func isToolNoise(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	// Tool markers look like "[tool_use: Bash]" / "[tool_result]" or a
	// bare bracketed token with no surrounding prose.
	if strings.HasPrefix(t, "[tool_use") || strings.HasPrefix(t, "[tool_result") {
		return true
	}
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") && !strings.ContainsAny(t, " \n") {
		return true
	}
	// Harness-injected machinery, not human/agent prose: task notifications,
	// slash-command framing, system reminders, and local-command stdout.
	for _, marker := range []string{
		"<task-notification",
		"<command-message",
		"<command-name",
		"<local-command-stdout",
		"<local-command-caveat",
		"<system-reminder",
		"<environment_context",
		"Base directory for this skill:",
		"<bash-",
		"Caveat: The messages below",
	} {
		if strings.HasPrefix(t, marker) {
			return true
		}
	}
	return false
}

func payloadString(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func payloadStrings(p map[string]any, key string) []string {
	if p == nil {
		return nil
	}
	v, ok := p[key]
	if !ok {
		return nil
	}
	switch xs := v.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
