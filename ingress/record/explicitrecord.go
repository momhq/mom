package record

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/storage/librarian"
)

// Publisher is the canonicalization gateway record.Publish hands the
// explicit-write event to. Implemented by *editor.Editor in production.
// Defined as an interface so tests can substitute a recorder without
// standing up a real Ledger.
type Publisher interface {
	Publish(in editor.Canonicalizer, src editor.Source) error
}

// recordCanonical adapts a prepared MemoryRecord payload to the
// editor.Canonicalizer contract. SessionID lives on the envelope
// (extracted by Editor.Canonicalize from payload["session_id"]).
type recordCanonical struct {
	payload map[string]any
}

func (r recordCanonical) Canonical() (herald.EventType, map[string]any) {
	return herald.MemoryRecord, r.payload
}

var ErrMissingSessionID = errors.New("session_id is required; do not invent one")

// sessionEnvKeys is the ordered list of env vars consulted when no
// explicit session id is supplied. MOM_SESSION_ID is the neutral name
// every harness should export going forward; the harness-specific
// names remain for backwards compatibility with harnesses that have
// not yet adopted MOM_SESSION_ID. First non-empty value wins.
var sessionEnvKeys = []string{
	"MOM_SESSION_ID",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_SESSION_ID",
	"CODEX_THREAD_ID",
	"CODEX_SESSION_ID",
	// WINDSURF_TRAJECTORY_ID was retired alongside the Windsurf harness
	// in #342/#343 and removed from this list in v0.40 cleanup.
}

// Request is the shared explicit-write contract for `mom record` and
// `mom_record`. The caller may provide SessionID when it is a real harness
// session id; otherwise ResolveSessionID checks known harness environment
// variables.
type Request struct {
	SessionID string
	Summary   string
	Tags      []string
	Content   map[string]any
	Actor     string
	// ProjectId is the declared project identity (ADR 0016). Empty
	// when the caller could not resolve a binding (e.g. cwd is not in
	// any .mom-project.yaml). The resulting memory will have NULL
	// project_id in those cases.
	ProjectId string
}

type Result struct {
	SessionID string
	Summary   string
	Tags      []string
	Actor     string
}

// Publish builds the canonical explicit-write event and hands it to
// the Editor. Per architecture v3, Editor appends to the Ledger; Crier
// projects and republishes onto Herald, where Drafter persists. record
// no longer touches the bus directly — that path silently bypassed the
// Ledger and would not be replayable.
func Publish(pub Publisher, req Request) (Result, error) {
	if pub == nil {
		return Result{}, errors.New("editor publisher is required")
	}
	if len(req.Content) == 0 {
		return Result{}, errors.New("content cannot be empty (must contain at least one field)")
	}
	sessionID, err := ResolveSessionID(req.SessionID)
	if err != nil {
		return Result{}, err
	}
	tags, err := NormalizeTagsOrReject(req.Tags)
	if err != nil {
		return Result{}, err
	}
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "mcp"
	}

	payload := map[string]any{
		"session_id":               sessionID,
		"content":                  req.Content,
		"summary":                  req.Summary,
		"tags":                     tags,
		"provenance_actor":         actor,
		"provenance_source_type":   "manual-draft",
		"provenance_trigger_event": "record",
	}
	if req.ProjectId != "" {
		payload["project_id"] = req.ProjectId
	}
	if err := pub.Publish(recordCanonical{payload: payload}, editor.Source{Adapter: actor}); err != nil {
		return Result{}, fmt.Errorf("record: editor publish: %w", err)
	}

	return Result{SessionID: sessionID, Summary: req.Summary, Tags: tags, Actor: actor}, nil
}

// LooksLikeHarnessSessionID reports whether the given string has the shape
// of a real harness-issued session identifier (UUID, or a timestamped
// UUID-suffixed cursor filename). Used to reject invented or human-friendly
// strings before they reach the persistence layer.
func LooksLikeHarnessSessionID(sessionID string) bool {
	s := strings.TrimSpace(sessionID)
	if s == "" {
		return false
	}
	// UUID/UUIDv7-shaped IDs are what Claude and Pi expose. Pi watcher cursor
	// filenames may prefix the UUID with a timestamp and underscore, so validate
	// the suffix in that case.
	if idx := strings.LastIndex(s, "_"); idx >= 0 && idx+1 < len(s) {
		s = s[idx+1:]
	}
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !unicode.IsDigit(r) && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				return false
			}
		}
	}
	return true
}

func ResolveSessionID(explicit string) (string, error) {
	if s := strings.TrimSpace(explicit); s != "" {
		if !LooksLikeHarnessSessionID(s) {
			return "", fmt.Errorf("session_id %q does not look like a harness session ID; do not invent one", s)
		}
		return s, nil
	}
	for _, key := range sessionEnvKeys {
		if s := strings.TrimSpace(os.Getenv(key)); s != "" {
			return s, nil
		}
	}
	return "", ErrMissingSessionID
}

func NormalizeTagsOrReject(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	for i, t := range raw {
		n := librarian.NormalizeTagName(t)
		if n == "" {
			return nil, fmt.Errorf("tag %d (%q) normalises to empty; reject the request rather than persist a partial memory", i, t)
		}
		out = append(out, n)
	}
	return out, nil
}
