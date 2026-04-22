package mcp_test

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momhq/mom/cli/internal/mcp"
)

// helpers

func newTestLeoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	leoDir := filepath.Join(dir, ".mom")
	if err := os.MkdirAll(filepath.Join(leoDir, "memory"), 0755); err != nil {
		t.Fatal(err)
	}
	// Write a minimal config.yaml so scope.loadScopeLabel works.
	if err := os.WriteFile(filepath.Join(leoDir, "config.yaml"), []byte("scope: repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write identity.json
	identity := map[string]any{
		"id":         "identity",
		"type":       "identity",
		"lifecycle":  "permanent",
		"scope":      "project",
		"tags":       []string{"identity"},
		"created":    time.Now().Format(time.RFC3339),
		"created_by": "test",
		"updated":    time.Now().Format(time.RFC3339),
		"updated_by": "test",
		"content": map[string]any{
			"name": "Test Project",
		},
	}
	writeJSON(t, filepath.Join(leoDir, "identity.json"), identity)
	return leoDir
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeMemoryDoc(t *testing.T, leoDir string, doc map[string]any) {
	t.Helper()
	id, _ := doc["id"].(string)
	path := filepath.Join(leoDir, "memory", id+".json")
	writeJSON(t, path, doc)
}

// sendRequest writes one JSON-RPC request to the writer.
func sendRequest(t *testing.T, w io.Writer, method string, id any, params any) {
	t.Helper()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
}

// readResponse reads one JSON-RPC response from the reader.
func readResponse(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &resp); err != nil {
		t.Fatalf("parsing response %q: %v", line, err)
	}
	return resp
}

// runServer starts the MCP server in a goroutine, sends requests via inW,
// reads responses from outR, and returns cleanup func.
func runServer(t *testing.T, leoDir string) (inW io.WriteCloser, outR *bufio.Reader, done chan struct{}) {
	t.Helper()
	inR, inW := io.Pipe()
	outR2, outW := io.Pipe()

	srv := mcp.New(leoDir)
	done = make(chan struct{})
	go func() {
		defer close(done)
		srv.Serve(inR, outW)
		outW.Close()
	}()
	outR = bufio.NewReader(outR2)
	return inW, outR, done
}

// --- Tests ---

func TestInitializeHandshake(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]any{"name": "test-client", "version": "0.1"},
	})

	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp["result"])
	}
	if result["protocolVersion"] == nil {
		t.Error("protocolVersion missing from initialize result")
	}
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("capabilities missing or wrong type")
	}
	if caps["tools"] == nil {
		t.Error("tools capability missing")
	}
	if caps["resources"] == nil {
		t.Error("resources capability missing")
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("serverInfo missing")
	}
	if serverInfo["name"] == nil {
		t.Error("serverInfo.name missing")
	}
}

func TestToolsList(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	// Initialize first.
	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/list", 2, nil)
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools not an array: %v", result["tools"])
	}
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("tool not a map")
		}
		name, _ := tool["name"].(string)
		names[name] = true
	}
	expected := []string{"search_memories", "get_memory", "list_scopes", "create_memory_draft", "list_landmarks", "mom_record_turn", "mom_recall"}
	for _, n := range expected {
		if !names[n] {
			t.Errorf("tool %q missing from tools/list", n)
		}
	}
}

func TestToolsCallSearchMemories(t *testing.T) {
	leoDir := newTestLeoDir(t)
	writeMemoryDoc(t, leoDir, map[string]any{
		"id":         "auth-pattern",
		"type":       "pattern",
		"lifecycle":  "permanent",
		"scope":      "project",
		"tags":       []string{"auth", "security"},
		"summary":    "Authentication pattern for JWT",
		"created":    time.Now().Format(time.RFC3339),
		"created_by": "test",
		"updated":    time.Now().Format(time.RFC3339),
		"updated_by": "test",
		"content":    map[string]any{"detail": "Use JWT with RS256"},
	})

	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name": "search_memories",
		"arguments": map[string]any{
			"query": "auth",
		},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("content missing or empty")
	}
}

func TestToolsCallGetMemory(t *testing.T) {
	leoDir := newTestLeoDir(t)
	writeMemoryDoc(t, leoDir, map[string]any{
		"id":         "test-fact",
		"type":       "fact",
		"lifecycle":  "permanent",
		"scope":      "project",
		"tags":       []string{"test"},
		"summary":    "Test fact",
		"created":    time.Now().Format(time.RFC3339),
		"created_by": "test",
		"updated":    time.Now().Format(time.RFC3339),
		"updated_by": "test",
		"content":    map[string]any{"detail": "some detail"},
	})

	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "get_memory",
		"arguments": map[string]any{"id": "test-fact"},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("content missing or empty")
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first content item not a map")
	}
	text, _ := first["text"].(string)
	if !strings.Contains(text, "test-fact") {
		t.Errorf("expected response to contain doc id, got: %s", text)
	}
}

func TestToolsCallListScopes(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "list_scopes",
		"arguments": map[string]any{},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("content missing or empty")
	}
}

func TestToolsCallCreateMemoryDraft(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name": "create_memory_draft",
		"arguments": map[string]any{
			"type":    "fact",
			"summary": "A new test fact",
			"tags":    []any{"test", "mcp"},
			"content": map[string]any{"detail": "created via MCP"},
		},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("content missing or empty")
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first content item not a map")
	}
	text, _ := first["text"].(string)
	if !strings.Contains(text, "draft") {
		t.Errorf("expected response to mention draft, got: %s", text)
	}

	// Verify the file was actually created.
	entries, _ := os.ReadDir(filepath.Join(leoDir, "memory"))
	if len(entries) == 0 {
		t.Error("expected draft file to be written to memory dir")
	}
}

func TestToolsCallListLandmarks(t *testing.T) {
	leoDir := newTestLeoDir(t)
	score := 0.9
	writeMemoryDoc(t, leoDir, map[string]any{
		"id":              "landmark-doc",
		"type":            "pattern",
		"lifecycle":       "permanent",
		"scope":           "project",
		"tags":            []string{"arch"},
		"summary":         "Key architectural pattern",
		"landmark":        true,
		"centrality_score": score,
		"created":         time.Now().Format(time.RFC3339),
		"created_by":      "test",
		"updated":         time.Now().Format(time.RFC3339),
		"updated_by":      "test",
		"content":         map[string]any{"detail": "x"},
	})

	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "list_landmarks",
		"arguments": map[string]any{},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("content missing or empty")
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first content item not a map")
	}
	text, _ := first["text"].(string)
	if !strings.Contains(text, "landmark-doc") {
		t.Errorf("expected landmark doc in response, got: %s", text)
	}
}

func TestResourcesList(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "resources/list", 2, nil)
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	resources, ok := result["resources"].([]any)
	if !ok {
		t.Fatalf("resources not an array: %v", result["resources"])
	}
	if len(resources) != 3 {
		t.Errorf("expected 3 resources, got %d", len(resources))
	}

	uris := make(map[string]bool)
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("resource not a map")
		}
		uri, _ := res["uri"].(string)
		uris[uri] = true
	}
	for _, expected := range []string{"mom://identity", "mom://constraints", "mom://scopes"} {
		if !uris[expected] {
			t.Errorf("resource %q missing from resources/list", expected)
		}
	}
}

func TestResourcesReadIdentity(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "resources/read", 2, map[string]any{"uri": "mom://identity"})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	contents, ok := result["contents"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatal("contents missing or empty")
	}
}

func TestResourcesReadConstraints(t *testing.T) {
	leoDir := newTestLeoDir(t)
	// Add a constraint doc.
	if err := os.MkdirAll(filepath.Join(leoDir, "constraints"), 0755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(leoDir, "constraints", "anti-hallucination.json"), map[string]any{
		"id":         "anti-hallucination",
		"type":       "constraint",
		"lifecycle":  "permanent",
		"scope":      "core",
		"tags":       []string{"constraint"},
		"summary":    "Never hallucinate",
		"created":    time.Now().Format(time.RFC3339),
		"created_by": "test",
		"updated":    time.Now().Format(time.RFC3339),
		"updated_by": "test",
		"content":    map[string]any{"rule": "no hallucination"},
	})

	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "resources/read", 2, map[string]any{"uri": "mom://constraints"})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	contents, ok := result["contents"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatal("contents missing or empty")
	}
}

func TestResourcesReadScopes(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "resources/read", 2, map[string]any{"uri": "mom://scopes"})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %v", resp)
	}
	contents, ok := result["contents"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatal("contents missing or empty")
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "unknown/method", 2, nil)
	resp := readResponse(t, outR)

	if resp["error"] == nil {
		t.Error("expected error for unknown method, got nil")
	}
}

func TestNotificationIgnored(t *testing.T) {
	leoDir := newTestLeoDir(t)
	inW, outR, _ := runServer(t, leoDir)
	defer inW.Close()

	// Send initialize first.
	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	// Send a notification (no id field) — server must NOT respond.
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	data, _ := json.Marshal(notif)
	data = append(data, '\n')
	inW.Write(data)

	// Now send a valid request and verify we get exactly one response (not two).
	sendRequest(t, inW, "tools/list", 2, nil)
	resp := readResponse(t, outR)
	if id, ok := resp["id"].(float64); !ok || id != 2 {
		t.Errorf("expected response id=2, got %v", resp["id"])
	}
}
