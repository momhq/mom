package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/momhq/mom/cli/internal/store"
)

// toolMomRecord handles the mom_record MCP tool. Writes a memory to
// the central vault via MemoryStore. Bypasses capture filters
// (explicit user/agent write path per ADR 0014). Stamps the
// server-side provenance fields trigger_event="record" and
// source_type="manual-draft"; all other substance fields are
// caller-provided and required.
func (s *Server) toolMomRecord(args map[string]any) (toolCallResult, error) {
	if s.vault == nil {
		return toolCallResult{}, fmt.Errorf("mom_record: vault not configured on server")
	}

	summary := stringArg(args, "summary")
	if summary == "" {
		return toolCallResult{}, fmt.Errorf("mom_record: summary is required")
	}
	content, _ := args["content"].(map[string]any)
	if content == nil {
		return toolCallResult{}, fmt.Errorf("mom_record: content is required")
	}
	sessionID := stringArg(args, "session_id")
	if sessionID == "" {
		return toolCallResult{}, fmt.Errorf("mom_record: session_id is required")
	}
	actor := stringArg(args, "actor")
	if actor == "" {
		return toolCallResult{}, fmt.Errorf("mom_record: actor is required")
	}
	createdBy := stringArg(args, "created_by")
	if createdBy == "" {
		return toolCallResult{}, fmt.Errorf("mom_record: created_by is required")
	}
	memType := stringArg(args, "type")

	// Pre-validate tags: normalize each (T2 model from ADR 0010) and
	// reject the whole request if any tag reduces to an empty string.
	// Doing this before any DB write avoids orphan memory + entity rows
	// when a tag like "!!!" or "   " is passed.
	rawTags := stringSliceArg(args, "tags")
	tags := make([]string, 0, len(rawTags))
	for _, raw := range rawTags {
		norm := store.NormalizeTagName(raw)
		if norm == "" {
			return toolCallResult{}, fmt.Errorf("mom_record: tag %q normalises to empty; tags must contain at least one alphanumeric character", raw)
		}
		tags = append(tags, norm)
	}

	ms := store.NewMemoryStore(s.vault)
	mem, err := ms.Insert(store.Memory{
		Type:                   memType,
		Summary:                summary,
		Content:                content,
		SessionID:              sessionID,
		ProvenanceActor:        actor,
		ProvenanceSourceType:   "manual-draft",
		ProvenanceTriggerEvent: "record",
	})
	if err != nil {
		return toolCallResult{}, fmt.Errorf("mom_record: %w", err)
	}

	gs := store.NewGraphStore(s.vault)

	// created_by edge: upsert the user entity (idempotent on
	// (type, display_name)) and link the new memory to it.
	entityID, err := gs.UpsertEntity("user", createdBy)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("mom_record: upsert created_by entity %q: %w", createdBy, err)
	}
	if err := gs.LinkEntity(mem.ID, entityID, "created_by"); err != nil {
		return toolCallResult{}, fmt.Errorf("mom_record: link created_by entity: %w", err)
	}

	for _, tag := range tags {
		tagID, err := gs.UpsertTag(tag)
		if err != nil {
			return toolCallResult{}, fmt.Errorf("mom_record: upsert tag %q: %w", tag, err)
		}
		if err := gs.LinkTag(mem.ID, tagID); err != nil {
			return toolCallResult{}, fmt.Errorf("mom_record: link tag %q: %w", tag, err)
		}
	}

	resultDoc := map[string]any{
		"id":              mem.ID,
		"promotion_state": mem.PromotionState,
		"created_at":      mem.CreatedAt.Format(time.RFC3339Nano),
		"message":         fmt.Sprintf("Memory created with id=%s", mem.ID),
	}
	text, _ := json.MarshalIndent(resultDoc, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}
