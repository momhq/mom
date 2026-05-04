package mcp_test

import (
	"testing"

	"github.com/momhq/mom/cli/internal/store"
)

// T6: mom_recall via MCP — agent sends query, server returns ranked
// summary results including the new memory's id and summary. Full
// content NOT included (agents fetch via mom_get).
func TestToolsCallMomRecall_v030(t *testing.T) {
	v, _ := newTestVault(t)
	leoDir := newTestLeoDir(t)

	// Pre-populate via MemoryStore (simulating prior mom_record calls).
	ms := store.NewMemoryStore(v)
	mem, err := ms.Insert(store.Memory{
		Type:                   "semantic",
		Summary:                "deploy procedure for staging",
		Content:                map[string]any{"text": "the deploy step uses a canary check before promoting"},
		SessionID:              "s1",
		ProvenanceActor:        "claude-code",
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
		PromotionState:         "curated",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()

	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name": "mom_recall",
		"arguments": map[string]any{
			"query":       "canary",
			"max_results": 5,
		},
	})
	resp := readResponse(t, outR)

	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("tool returned isError: %v", result)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content: %v", result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)

	var hits []map[string]any
	if err := jsonUnmarshalString(text, &hits); err != nil {
		t.Fatalf("parse mom_recall results %q: %v", text, err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit for 'canary', got 0")
	}

	found := false
	for _, hit := range hits {
		if id, _ := hit["ID"].(string); id == mem.ID {
			found = true
			// Verify summary fields present, content NOT included
			// (agents fetch via mom_get).
			if _, hasContent := hit["Content"]; hasContent {
				t.Errorf("mom_recall result should not include Content; got %v", hit)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected mem %s in mom_recall hits, got %v", mem.ID, hits)
	}
}

// T7: mom_get via MCP — retrieves memory by ID through MemoryStore,
// returns full substance including content.
func TestToolsCallMomGet_v030(t *testing.T) {
	v, _ := newTestVault(t)
	leoDir := newTestLeoDir(t)

	ms := store.NewMemoryStore(v)
	mem, err := ms.Insert(store.Memory{
		Summary:                "test memory",
		Content:                map[string]any{"text": "the body"},
		SessionID:              "s1",
		ProvenanceActor:        "claude-code",
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()
	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "mom_get",
		"arguments": map[string]any{"id": mem.ID},
	})
	resp := readResponse(t, outR)

	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("tool returned isError: %v", result)
	}
	content, _ := result["content"].([]any)
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)

	var doc map[string]any
	if err := jsonUnmarshalString(text, &doc); err != nil {
		t.Fatalf("parse mom_get response %q: %v", text, err)
	}
	if doc["id"] != mem.ID {
		t.Errorf("id: got %v, want %q", doc["id"], mem.ID)
	}
	if doc["summary"] != "test memory" {
		t.Errorf("summary: got %v", doc["summary"])
	}
	gotContent, _ := doc["content"].(map[string]any)
	if txt, _ := gotContent["text"].(string); txt != "the body" {
		t.Errorf("content.text: got %v", gotContent["text"])
	}
}

// T7b: mom_get returns a clear error for an unknown ID.
func TestToolsCallMomGet_NotFound(t *testing.T) {
	v, _ := newTestVault(t)
	leoDir := newTestLeoDir(t)

	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()
	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "mom_get",
		"arguments": map[string]any{"id": "00000000-0000-0000-0000-000000000000"},
	})
	resp := readResponse(t, outR)

	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Errorf("expected isError=true for unknown id")
	}
}

// T8: mom_landmarks via MCP — returns landmark memories sorted by
// centrality_score descending.
func TestToolsCallMomLandmarks_v030(t *testing.T) {
	v, _ := newTestVault(t)
	leoDir := newTestLeoDir(t)

	ms := store.NewMemoryStore(v)
	for i, score := range []float64{0.5, 0.9, 0.7} {
		mem, err := ms.Insert(store.Memory{
			Summary:                "landmark candidate",
			Content:                map[string]any{"text": "x"},
			SessionID:              "s1",
			ProvenanceActor:        "test",
			ProvenanceSourceType:   "manual-draft",
			ProvenanceTriggerEvent: "record",
		})
		if err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
		if err := ms.SetLandmark(mem.ID, true); err != nil {
			t.Fatalf("SetLandmark: %v", err)
		}
		s := score
		if err := ms.SetCentralityScore(mem.ID, &s); err != nil {
			t.Fatalf("SetCentralityScore: %v", err)
		}
	}
	// One non-landmark memory that should NOT appear.
	if _, err := ms.Insert(store.Memory{
		Summary:                "non-landmark",
		Content:                map[string]any{"text": "x"},
		SessionID:              "s1",
		ProvenanceActor:        "test",
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
	}); err != nil {
		t.Fatal(err)
	}

	inW, outR, _ := runServerWithVault(t, leoDir, v)
	defer inW.Close()
	sendRequest(t, inW, "initialize", 1, map[string]any{"protocolVersion": "2024-11-05"})
	readResponse(t, outR)

	sendRequest(t, inW, "tools/call", 2, map[string]any{
		"name":      "mom_landmarks",
		"arguments": map[string]any{"limit": 10},
	})
	resp := readResponse(t, outR)
	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("tool returned isError: %v", result)
	}
	content, _ := result["content"].([]any)
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)

	var items []map[string]any
	if err := jsonUnmarshalString(text, &items); err != nil {
		t.Fatalf("parse mom_landmarks response %q: %v", text, err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 landmarks, got %d: %v", len(items), items)
	}
	// Should be sorted by centrality_score desc: 0.9, 0.7, 0.5
	wantScores := []float64{0.9, 0.7, 0.5}
	for i, item := range items {
		got, _ := item["centrality_score"].(float64)
		if got != wantScores[i] {
			t.Errorf("position %d: centrality_score got %v, want %v", i, got, wantScores[i])
		}
	}
}
