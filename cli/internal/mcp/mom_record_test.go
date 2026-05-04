package mcp_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/momhq/mom/cli/internal/mcp"
	"github.com/momhq/mom/cli/internal/store"
	"github.com/momhq/mom/cli/internal/vault"
)

// jsonUnmarshalString is a thin helper for tests that parse the JSON
// payload embedded in a tool result's text content.
func jsonUnmarshalString(s string, dst any) error {
	return json.Unmarshal([]byte(s), dst)
}

// runServerWithVault is the runServer variant that wires a Vault into
// the Server before Serve starts, so v0.30 tools that depend on the
// vault (mom_record and friends) work.
func runServerWithVault(t *testing.T, leoDir string, v *vault.Vault) (io.WriteCloser, *bufio.Reader, chan struct{}) {
	t.Helper()
	inR, inW := io.Pipe()
	outR2, outW := io.Pipe()

	srv := mcp.New(leoDir)
	srv.SetVault(v)

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Serve(inR, outW)
		_ = outW.Close()
	}()
	return inW, bufio.NewReader(outR2), done
}

// newTestVault opens a fresh Vault in a temp dir and returns it
// alongside its filesystem path.
func newTestVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mom.db")
	v, err := vault.Open(dbPath)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v, dbPath
}

// T1 (tracer bullet): mom_record with all required args writes a
// memory readable via MemoryStore.Get. The returned ID is a UUID v4.
// Verifies the full path: MCP request → handler → MemoryStore →
// Vault → SQLite, and back via Get.
func TestToolsCallMomRecord_RoundTrip(t *testing.T) {
	leoDir := newTestLeoDir(t)
	v, _ := newTestVault(t)

	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name": "mom_record",
		"arguments": map[string]any{
			"summary":    "test summary",
			"content":    map[string]any{"text": "test body"},
			"session_id": "session-1",
			"actor":      "claude-code",
			"created_by": "Vinicius",
		},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp["result"])
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("tool returned isError: %v", result)
	}

	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("content missing: %v", result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)

	// Parse the JSON returned by the tool to extract the new ID.
	var resultDoc map[string]any
	if err := jsonUnmarshalString(text, &resultDoc); err != nil {
		t.Fatalf("parse tool response %q: %v", text, err)
	}
	id, _ := resultDoc["id"].(string)
	if id == "" {
		t.Fatalf("response missing id: %v", resultDoc)
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("expected UUID id, got %q: %v", id, err)
	}

	// Read back via MemoryStore.
	ms := store.NewMemoryStore(v)
	got, err := ms.Get(id)
	if err != nil {
		t.Fatalf("Get(%s): %v", id, err)
	}
	if got.Summary != "test summary" {
		t.Errorf("Summary: got %q, want %q", got.Summary, "test summary")
	}
	if txt, _ := got.Content["text"].(string); txt != "test body" {
		t.Errorf("Content.text: got %v", got.Content["text"])
	}
}

// callMomRecord is a small fixture that runs an MCP server with the
// given vault, sends one mom_record request with the given args, and
// returns the parsed result document.
func callMomRecord(t *testing.T, v *vault.Vault, args map[string]any) map[string]any {
	t.Helper()
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "mom_record",
		"arguments": args,
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content in response: %v", result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)

	var doc map[string]any
	if err := jsonUnmarshalString(text, &doc); err != nil {
		t.Fatalf("parse tool response %q: %v", text, err)
	}
	return doc
}

// T2: mom_record stamps server-side provenance — trigger_event="record"
// and source_type="manual-draft" — regardless of what the caller
// passes. The caller-supplied actor flows through.
func TestToolsCallMomRecord_StampsServerSideProvenance(t *testing.T) {
	v, _ := newTestVault(t)

	doc := callMomRecord(t, v, map[string]any{
		"summary":    "stamping test",
		"content":    map[string]any{"text": "x"},
		"session_id": "session-xyz",
		"actor":      "codex",
		"created_by": "Vinicius",
	})

	id, _ := doc["id"].(string)
	if id == "" {
		t.Fatalf("missing id in response: %v", doc)
	}

	ms := store.NewMemoryStore(v)
	mem, err := ms.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if mem.ProvenanceTriggerEvent != "record" {
		t.Errorf("trigger_event: got %q, want %q", mem.ProvenanceTriggerEvent, "record")
	}
	if mem.ProvenanceSourceType != "manual-draft" {
		t.Errorf("source_type: got %q, want %q", mem.ProvenanceSourceType, "manual-draft")
	}
	if mem.ProvenanceActor != "codex" {
		t.Errorf("actor: got %q, want %q", mem.ProvenanceActor, "codex")
	}
	if mem.SessionID != "session-xyz" {
		t.Errorf("session_id: got %q, want %q", mem.SessionID, "session-xyz")
	}
}

// T3: When mom_record receives tags, the handler upserts each tag and
// links the new memory to it. MemoriesByTag returns the new memory ID
// for each tag.
func TestToolsCallMomRecord_TagsAreLinked(t *testing.T) {
	v, _ := newTestVault(t)

	doc := callMomRecord(t, v, map[string]any{
		"summary":    "tagged memory",
		"content":    map[string]any{"text": "x"},
		"session_id": "s1",
		"actor":      "claude-code",
		"created_by": "Vinicius",
		"tags":       []any{"recall", "graph"},
	})
	id, _ := doc["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", doc)
	}

	gs := store.NewGraphStore(v)
	for _, tag := range []string{"recall", "graph"} {
		ids, err := gs.MemoriesByTag(tag)
		if err != nil {
			t.Fatalf("MemoriesByTag(%q): %v", tag, err)
		}
		found := false
		for _, mid := range ids {
			if mid == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("MemoriesByTag(%q) does not include %s; got %v", tag, id, ids)
		}
	}
}

// T4: mom_record returns an error when any required arg is missing.
// Substance fields cannot be silently defaulted to empty — there is
// no way to fill them in later (substance is immutable per ADR 0011).
func TestToolsCallMomRecord_RejectsMissingRequiredArgs(t *testing.T) {
	complete := map[string]any{
		"summary":    "x",
		"content":    map[string]any{"text": "y"},
		"session_id": "s1",
		"actor":      "claude-code",
		"created_by": "Vinicius",
	}

	cases := []string{"summary", "content", "session_id", "actor", "created_by"}
	for _, missing := range cases {
		t.Run("missing_"+missing, func(t *testing.T) {
			v, _ := newTestVault(t)
			leoDir := newTestLeoDir(t)
			inW, outR, _ := runServerWithVault(t, leoDir, v)
			defer inW.Close()

			sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
			readResponse(t, outR)

			args := make(map[string]any, len(complete))
			for k, val := range complete {
				if k != missing {
					args[k] = val
				}
			}
			sendRequest(t, inW, "tools/call", 2, map[string]any{
				"name":      "mom_record",
				"arguments": args,
			})
			resp := readResponse(t, outR)

			result, _ := resp["result"].(map[string]any)
			isErr, _ := result["isError"].(bool)
			if !isErr {
				t.Errorf("expected isError=true when %q is missing, got result=%v", missing, result)
			}
		})
	}
}

// T5: mom_record upserts a user entity for created_by and links the
// new memory to it with relationship="created_by". Second call with
// the same created_by reuses the same entity (idempotent at the
// entity layer).
func TestToolsCallMomRecord_CreatedByLinksUserEntity(t *testing.T) {
	v, _ := newTestVault(t)

	doc1 := callMomRecord(t, v, map[string]any{
		"summary":    "first memory",
		"content":    map[string]any{"text": "x"},
		"session_id": "s1",
		"actor":      "claude-code",
		"created_by": "Vinicius",
	})
	id1, _ := doc1["id"].(string)

	doc2 := callMomRecord(t, v, map[string]any{
		"summary":    "second memory",
		"content":    map[string]any{"text": "y"},
		"session_id": "s2",
		"actor":      "claude-code",
		"created_by": "Vinicius",
	})
	id2, _ := doc2["id"].(string)

	// One entity row for ("user", "Vinicius") — idempotent across calls.
	var entityCount int
	if err := v.Query(
		`SELECT COUNT(*) FROM entities WHERE type='user' AND display_name='Vinicius'`,
		nil,
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&entityCount)
			}
			return nil
		},
	); err != nil {
		t.Fatalf("count entities: %v", err)
	}
	if entityCount != 1 {
		t.Errorf("expected 1 user/Vinicius entity row, got %d", entityCount)
	}

	// Both memories have a created_by edge.
	for _, id := range []string{id1, id2} {
		var rel string
		err := v.Query(
			`SELECT me.relationship FROM memory_entities me
			 JOIN entities e ON e.id = me.entity_id
			 WHERE me.memory_id = ? AND e.type='user' AND e.display_name='Vinicius'`,
			[]any{id},
			func(rows *sql.Rows) error {
				if rows.Next() {
					return rows.Scan(&rel)
				}
				return nil
			},
		)
		if err != nil {
			t.Fatalf("query edge for %s: %v", id, err)
		}
		if rel != "created_by" {
			t.Errorf("memory %s relationship: got %q, want %q", id, rel, "created_by")
		}
	}
}

// T18a: mom_record normalizes tags before linking. Caller-supplied
// variations of the same intent ("My Tag", "my-tag", "MY_TAG") all
// resolve to the same canonical tag row.
func TestToolsCallMomRecord_NormalizesTags(t *testing.T) {
	v, _ := newTestVault(t)

	doc := callMomRecord(t, v, map[string]any{
		"summary":    "normalize test",
		"content":    map[string]any{"text": "x"},
		"session_id": "s1",
		"actor":      "claude-code",
		"created_by": "Vinicius",
		"tags":       []any{"My Tag", "v0.30", "Foo_Bar"},
	})
	id, _ := doc["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", doc)
	}

	gs := store.NewGraphStore(v)
	for _, want := range []string{"my-tag", "v0-30", "foo-bar"} {
		ids, err := gs.MemoriesByTag(want)
		if err != nil {
			t.Fatalf("MemoriesByTag(%q): %v", want, err)
		}
		found := false
		for _, mid := range ids {
			if mid == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tag %q to link to %s, got %v", want, id, ids)
		}
	}
}

// T18b: When a tag normalizes to empty (e.g. "!!!" or "   "),
// mom_record rejects the entire request before persisting anything.
// No orphan memory or partial graph state.
func TestToolsCallMomRecord_RejectsEmptyTagWithoutOrphans(t *testing.T) {
	v, _ := newTestVault(t)
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name": "mom_record",
		"arguments": map[string]any{
			"summary":    "should not persist",
			"content":    map[string]any{"text": "x"},
			"session_id": "s1",
			"actor":      "claude-code",
			"created_by": "Vinicius",
			"tags":       []any{"valid", "!!!"}, // "!!!" normalizes to ""
		},
	})
	resp := readResponse(t, outR)
	result, _ := resp["result"].(map[string]any)
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Errorf("expected isError=true for empty-after-normalization tag, got %v", result)
	}

	// No memory was persisted — fail-fast before Insert.
	var memCount int
	if err := v.Query(`SELECT COUNT(*) FROM memories`, nil, func(rows *sql.Rows) error {
		if rows.Next() {
			return rows.Scan(&memCount)
		}
		return nil
	}); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if memCount != 0 {
		t.Errorf("expected no memory persisted on rejected request, got %d", memCount)
	}

	// And no entities (created_by would have been upserted otherwise).
	var entCount int
	if err := v.Query(`SELECT COUNT(*) FROM entities`, nil, func(rows *sql.Rows) error {
		if rows.Next() {
			return rows.Scan(&entCount)
		}
		return nil
	}); err != nil {
		t.Fatalf("count entities: %v", err)
	}
	if entCount != 0 {
		t.Errorf("expected no entity persisted on rejected request, got %d", entCount)
	}
}
