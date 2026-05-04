package mcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// toolMomGet retrieves a single memory by ID from the central vault
// via MemoryStore. Returns ErrNotFound mapped to a clear error message
// when the ID does not exist.
func (s *Server) toolMomGet(args map[string]any) (toolCallResult, error) {
	if s.vault == nil {
		return toolCallResult{}, fmt.Errorf("mom_get: vault not configured on server")
	}
	id := stringArg(args, "id")
	if id == "" {
		return toolCallResult{}, fmt.Errorf("mom_get: id is required")
	}

	mem, err := s.memoryStore.Get(id)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("mom_get: %w", err)
	}

	doc := map[string]any{
		"id":                       mem.ID,
		"type":                     mem.Type,
		"summary":                  mem.Summary,
		"content":                  mem.Content,
		"created_at":               mem.CreatedAt.Format(time.RFC3339Nano),
		"session_id":               mem.SessionID,
		"provenance_actor":         mem.ProvenanceActor,
		"provenance_source_type":   mem.ProvenanceSourceType,
		"provenance_trigger_event": mem.ProvenanceTriggerEvent,
		"promotion_state":          mem.PromotionState,
		"landmark":                 mem.Landmark,
		"centrality_score":         mem.CentralityScore,
	}
	text, _ := json.MarshalIndent(doc, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// toolMomLandmarks returns landmark memories sorted by
// centrality_score descending. Reads directly from the vault — this
// is a focused query that does not need recall's escalation logic.
func (s *Server) toolMomLandmarks(args map[string]any) (toolCallResult, error) {
	if s.vault == nil {
		return toolCallResult{}, fmt.Errorf("mom_landmarks: vault not configured on server")
	}
	limit := intArg(args, "limit", 20)
	if limit <= 0 {
		limit = 20
	}

	type landmarkItem struct {
		ID              string   `json:"id"`
		Type            string   `json:"type"`
		Summary         string   `json:"summary"`
		PromotionState  string   `json:"promotion_state"`
		CentralityScore *float64 `json:"centrality_score"`
		CreatedAt       string   `json:"created_at"`
	}
	var items []landmarkItem

	err := s.vault.Query(
		`SELECT id, type, summary, promotion_state, centrality_score, created_at
		 FROM memories
		 WHERE landmark = 1
		 ORDER BY COALESCE(centrality_score, 0) DESC, created_at DESC
		 LIMIT ?`,
		[]any{limit},
		func(rows *sql.Rows) error {
			for rows.Next() {
				var item landmarkItem
				if err := rows.Scan(
					&item.ID, &item.Type, &item.Summary,
					&item.PromotionState, &item.CentralityScore, &item.CreatedAt,
				); err != nil {
					return err
				}
				items = append(items, item)
			}
			return nil
		},
	)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("mom_landmarks: %w", err)
	}

	if items == nil {
		items = []landmarkItem{}
	}
	text, _ := json.MarshalIndent(items, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}
