package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/memory"
	"github.com/momhq/mom/cli/internal/recall"
	"github.com/momhq/mom/cli/internal/scope"
)

// toolDef describes one MCP tool for the tools/list response.
type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolResult is the content item returned in tools/call responses.
type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// toolCallResult wraps the content list returned by a tool call.
type toolCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// allTools returns the static tool catalogue.
func allTools() []toolDef {
	return []toolDef{
		{
			Name:        "get_memory",
			Description: "Retrieve a single memory document by its ID.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Memory document ID"},
				},
			},
		},
		{
			Name:        "list_scopes",
			Description: "List all discovered .mom/ scopes from the current working directory walk-up.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "create_memory_draft",
			Description: "Create a draft memory document in the nearest .mom/memory/ directory.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"summary", "tags", "content"},
				"properties": map[string]any{
					"summary": map[string]any{"type": "string", "description": "One-line summary"},
					"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Kebab-case tags"},
					"content": map[string]any{"type": "object", "description": "Freeform content map"},
				},
			},
		},
		{
			Name:        "list_landmarks",
			Description: "List landmark memories sorted by centrality_score descending.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scope": map[string]any{"type": "string", "description": "Restrict to scope label"},
					"limit": map[string]any{"type": "integer", "description": "Maximum results (default 20)"},
				},
			},
		},
		{
			Name:        "mom_status",
			Description: "Returns MOM's full operating protocol — identity, boundaries, constraints, modes, and memory overview. Call this at the start of every session.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "mom_record",
			Description: "Explicit-write path: intentionally save a memory mid-session. Bypasses Drafter's content filters (the user override) and stamps trigger_event='record', source_type='manual-draft'. Required: session_id, content. Optional: summary, tags, actor.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"session_id", "content"},
				"properties": map[string]any{
					"session_id": map[string]any{"type": "string", "description": "Session ID this memory belongs to"},
					"summary":    map[string]any{"type": "string", "description": "One-line summary"},
					"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tag names (will be normalised; empty after normalisation rejects the request)"},
					"content":    map[string]any{"type": "object", "description": "Memory content (must include $.text for FTS)"},
					"actor":      map[string]any{"type": "string", "description": "Calling agent (claude-code, codex, …); defaults to 'mcp'"},
				},
			},
		},
		{
			Name:        "mom_recall",
			Description: "Search your memory for relevant context. Uses progressive scope escalation (repo→org→user) with AND→OR query relaxation and curated-first, draft-fallback quality tiers.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query":       map[string]any{"type": "string", "description": "Search query (keywords or natural language)"},
					"max_results": map[string]any{"type": "integer", "description": "Maximum results (default 5)"},
					"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter by tags (AND logic)"},
					"session_id":  map[string]any{"type": "string", "description": "Filter by source session"},
					"scope":       map[string]any{"type": "string", "description": "Pin search to a single scope label (repo/org/user); omit to use full escalation chain"},
				},
			},
		},
	}
}

// handleToolsList returns the static tool catalogue.
func (s *Server) handleToolsList() (any, *rpcError) {
	tools := allTools()
	// Convert to []any for JSON serialisation.
	out := make([]any, len(tools))
	for i, t := range tools {
		out[i] = t
	}
	return map[string]any{"tools": out}, nil
}

// handleToolsCall dispatches a tools/call request.
func (s *Server) handleToolsCall(params json.RawMessage) (any, *rpcError) {
	var req struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "invalid tools/call params: " + err.Error()}
	}

	var (
		result toolCallResult
		err    error
	)

	switch req.Name {
	case "get_memory":
		result, err = s.toolGetMemory(req.Arguments)
	case "list_scopes":
		result, err = s.toolListScopes()
	case "create_memory_draft":
		result, err = s.toolCreateMemoryDraft(req.Arguments)
	case "list_landmarks":
		result, err = s.toolListLandmarks(req.Arguments)
	case "mom_status":
		result, err = s.toolMomStatus()
	case "mom_record":
		result, err = s.toolMomRecord(req.Arguments)
	case "mom_recall":
		result, err = s.toolMomRecall(req.Arguments)
	default:
		return nil, &rpcError{Code: errCodeMethodNotFound, Message: "unknown tool: " + req.Name}
	}

	if err != nil {
		return toolCallResult{
			IsError: true,
			Content: []toolContent{{Type: "text", Text: err.Error()}},
		}, nil
	}
	return result, nil
}

// --- Tool implementations ---

// toolGetMemory retrieves a single memory doc by ID.
func (s *Server) toolGetMemory(args map[string]any) (toolCallResult, error) {
	id := stringArg(args, "id")
	if id == "" {
		return toolCallResult{}, fmt.Errorf("id is required")
	}

	scopes := scope.Walk(s.momDir)
	if len(scopes) == 0 {
		scopes = []scope.Scope{{Path: s.momDir, Label: "repo"}}
	}

	for _, sc := range scopes {
		path := filepath.Join(sc.Path, "memory", id+".json")
		doc, err := memory.LoadDoc(path)
		if err != nil {
			continue
		}
		// Emit telemetry.
		em := herald.New(s.momDir, true)
		em.EmitConsumptionEvent(herald.ConsumptionEvent{
			MemoryID: doc.ID,
			TS:       time.Now().UTC().Format(time.RFC3339),
			ByAgent:  "mcp",
			Context:  "get_memory",
		})

		text, _ := json.MarshalIndent(doc, "", "  ")
		return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
	}

	return toolCallResult{}, fmt.Errorf("memory %q not found in any scope", id)
}

// toolListScopes lists discovered .mom/ scopes.
func (s *Server) toolListScopes() (toolCallResult, error) {
	scopes := scope.Walk(s.momDir)
	if len(scopes) == 0 {
		scopes = []scope.Scope{{Path: s.momDir, Label: "repo"}}
	}

	type scopeItem struct {
		Label       string `json:"label"`
		Path        string `json:"path"`
		MemoryCount int    `json:"memory_count"`
	}
	items := make([]scopeItem, len(scopes))
	for i, sc := range scopes {
		items[i] = scopeItem{
			Label:       sc.Label,
			Path:        sc.Path,
			MemoryCount: sc.MemoryCount(),
		}
	}
	text, _ := json.MarshalIndent(items, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// toolCreateMemoryDraft creates a new draft memory document.
func (s *Server) toolCreateMemoryDraft(args map[string]any) (toolCallResult, error) {
	summary := stringArg(args, "summary")
	tags := stringSliceArg(args, "tags")
	content, _ := args["content"].(map[string]any)

	if summary == "" || len(tags) == 0 {
		return toolCallResult{}, fmt.Errorf("summary and tags are required")
	}
	if content == nil {
		content = map[string]any{}
	}

	// Sanitize tags: convert underscores to hyphens for kebab-case compliance.
	for i, t := range tags {
		tags[i] = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(t)), "_", "-")
	}

	// Use nearest writable scope or fall back to leoDir.
	targetDir := s.momDir
	if sc, ok := scope.NearestWritable(s.momDir); ok {
		targetDir = sc.Path
	}

	// Generate a slug ID from summary.
	id := slugify(summary)
	now := time.Now().UTC()

	memDir := filepath.Join(targetDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return toolCallResult{}, fmt.Errorf("creating memory dir: %w", err)
	}

	// Write through IndexedAdapter for SQLite index sync.
	storageDoc := &storage.Doc{
		ID:             id,
		Scope:          "project",
		Tags:           tags,
		Created:        now,
		CreatedBy:      "mcp",
		PromotionState: "curated",
		Classification: "INTERNAL",
		Compartments:   map[string][]string{},
		Provenance: &memory.Provenance{
			Runtime:      "mcp",
			TriggerEvent: "create_memory_draft",
		},
		Content: content,
	}
	if err := s.idx.Write(storageDoc); err != nil {
		return toolCallResult{}, fmt.Errorf("saving draft: %w", err)
	}

	path := filepath.Join(memDir, id+".json")
	result := map[string]any{
		"id":              id,
		"promotion_state": "curated",
		"path":            path,
		"message":         fmt.Sprintf("Memory created at %s", path),
	}
	text, _ := json.MarshalIndent(result, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// toolListLandmarks lists landmark memories sorted by centrality_score.
func (s *Server) toolListLandmarks(args map[string]any) (toolCallResult, error) {
	scopeLabel := stringArg(args, "scope")
	limit := intArg(args, "limit", 20)

	scopes := scope.Walk(s.momDir)
	if len(scopes) == 0 {
		scopes = []scope.Scope{{Path: s.momDir, Label: "repo"}}
	}

	var scopePaths []string
	for _, sc := range scopes {
		if scopeLabel == "" || sc.Label == scopeLabel {
			scopePaths = append(scopePaths, sc.Path)
		}
	}

	results, err := s.idx.ListLandmarks(scopePaths, limit)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("list_landmarks failed: %w", err)
	}

	if len(results) == 0 {
		return toolCallResult{Content: []toolContent{{Type: "text", Text: "No landmarks found."}}}, nil
	}

	type landmarkItem struct {
		ID              string   `json:"id"`
		Summary         string   `json:"summary"`
		Tags            []string `json:"tags"`
		CentralityScore float64  `json:"centrality_score"`
	}
	items := make([]landmarkItem, len(results))
	for i, r := range results {
		cs := 0.0
		if r.CentralityScore != nil {
			cs = *r.CentralityScore
		}
		items[i] = landmarkItem{
			ID:              r.ID,
			Summary:         r.Summary,
			Tags:            r.Tags,
			CentralityScore: cs,
		}
	}

	text, _ := json.MarshalIndent(items, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// toolMomRecall searches memories using the progressive escalation engine.
// When scope is provided, the engine chain is pinned to that single scope.
func (s *Server) toolMomRecall(args map[string]any) (toolCallResult, error) {
	query := stringArg(args, "query")
	maxResults := intArg(args, "max_results", 5)
	tags := stringSliceArg(args, "tags")
	sessionID := stringArg(args, "session_id")
	scopeLabel := stringArg(args, "scope")

	engine := s.engine
	// Pin to a single scope when the caller requests it.
	if scopeLabel != "" {
		scopes := scope.Walk(s.momDir)
		if len(scopes) == 0 {
			scopes = []scope.Scope{{Path: s.momDir, Label: "repo"}}
		}
		var chain []recall.Searcher
		for _, sc := range scopes {
			if sc.Label == scopeLabel {
				chain = append(chain, recall.NewScopeSearcher(s.idx, sc.Path))
			}
		}
		if len(chain) > 0 {
			engine = recall.NewEngine(chain)
		}
	}

	results, err := engine.Search(recall.Options{
		Query:      query,
		MaxResults: maxResults,
		Tags:       tags,
		SessionID:  sessionID,
	})
	if err != nil {
		return toolCallResult{}, fmt.Errorf("mom_recall search failed: %w", err)
	}

	// Emit telemetry for consumed memories.
	if s.momDir != "" {
		em := herald.New(s.momDir, true)
		for _, r := range results {
			em.EmitConsumptionEvent(herald.ConsumptionEvent{
				MemoryID: r.ID,
				TS:       time.Now().UTC().Format(time.RFC3339),
				ByAgent:  "mcp",
				Context:  "mom_recall",
			})
		}
	}

	if len(results) == 0 {
		return toolCallResult{Content: []toolContent{{Type: "text", Text: "No memories matched."}}}, nil
	}

	text, _ := json.MarshalIndent(results, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}



// --- Argument helpers ---

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func stringSliceArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func intArg(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return defaultVal
}

// slugify converts a string to a kebab-case ID suitable for file names.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prev := '-'
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prev = r
		default:
			if prev != '-' {
				b.WriteRune('-')
			}
			prev = '-'
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = fmt.Sprintf("draft-%d", time.Now().UnixMilli())
	}
	return result
}
