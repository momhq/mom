package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vmarinogg/leo-core/cli/internal/memory"
	"github.com/vmarinogg/leo-core/cli/internal/scope"
	"github.com/vmarinogg/leo-core/cli/internal/transponder"
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
			Name:        "search_memories",
			Description: "Search MOM memories by query text, tags, confidence, or classification. Returns ranked results.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":          map[string]any{"type": "string", "description": "Free-text search query"},
					"scope":          map[string]any{"type": "string", "description": "Restrict to scope label (repo/org/user)"},
					"tags":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter by tags (all must match)"},
					"confidence":     map[string]any{"type": "string", "description": "Filter by confidence (EXTRACTED/INFERRED/AMBIGUOUS)"},
					"classification": map[string]any{"type": "string", "description": "Filter by classification (PUBLIC/INTERNAL/CONFIDENTIAL)"},
					"limit":          map[string]any{"type": "integer", "description": "Maximum results (default 10)"},
				},
			},
		},
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
				"required": []string{"type", "summary", "tags", "content"},
				"properties": map[string]any{
					"type":    map[string]any{"type": "string", "description": "Doc type (fact/decision/pattern/learning/…)"},
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
	case "search_memories":
		result, err = s.toolSearchMemories(req.Arguments)
	case "get_memory":
		result, err = s.toolGetMemory(req.Arguments)
	case "list_scopes":
		result, err = s.toolListScopes()
	case "create_memory_draft":
		result, err = s.toolCreateMemoryDraft(req.Arguments)
	case "list_landmarks":
		result, err = s.toolListLandmarks(req.Arguments)
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

// toolSearchMemories searches memories across scopes using the same scoring
// logic as the recall command.
func (s *Server) toolSearchMemories(args map[string]any) (toolCallResult, error) {
	query := strings.ToLower(stringArg(args, "query"))
	scopeLabel := stringArg(args, "scope")
	tags := stringSliceArg(args, "tags")
	confidence := stringArg(args, "confidence")
	classification := stringArg(args, "classification")
	limit := intArg(args, "limit", 10)

	scopes := scope.Walk(s.momDir)
	// Also include the momDir itself as a scope if Walk doesn't find it.
	if len(scopes) == 0 {
		scopes = []scope.Scope{{Path: s.momDir, Label: "repo"}}
	}

	// Filter by scope label if specified.
	targetScopes := scopes
	if scopeLabel != "" {
		targetScopes = nil
		for _, sc := range scopes {
			if sc.Label == scopeLabel {
				targetScopes = append(targetScopes, sc)
				break
			}
		}
	}

	filterTagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		filterTagSet[strings.TrimSpace(t)] = true
	}

	type scored struct {
		doc   *memory.Doc
		score float64
	}
	var results []scored

	for _, sc := range targetScopes {
		memDir := filepath.Join(sc.Path, "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
			if err != nil {
				continue
			}
			// Apply confidence filter.
			if confidence != "" && !strings.EqualFold(doc.Confidence, confidence) {
				continue
			}
			// Apply classification filter.
			if classification != "" && !strings.EqualFold(doc.Classification, classification) {
				continue
			}
			score := scoreMemory(doc, query, filterTagSet)
			if score <= 0 {
				continue
			}
			results = append(results, scored{doc: doc, score: score})

			// Emit telemetry.
			if s.momDir != "" {
				em := transponder.New(s.momDir, true)
				em.EmitConsumptionEvent(transponder.ConsumptionEvent{
					MemoryID: doc.ID,
					TS:       time.Now().UTC().Format(time.RFC3339),
					ByAgent:  "mcp",
					Context:  "search_memories",
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].doc.ID < results[j].doc.ID
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	if len(results) == 0 {
		return toolCallResult{Content: []toolContent{{Type: "text", Text: "No memories matched."}}}, nil
	}

	type resultItem struct {
		ID       string  `json:"id"`
		Score    float64 `json:"score"`
		Type     string  `json:"type"`
		Summary  string  `json:"summary"`
		Tags     []string `json:"tags"`
		Landmark bool    `json:"landmark,omitempty"`
	}
	items := make([]resultItem, len(results))
	for i, r := range results {
		summary := r.doc.Summary
		if summary == "" {
			if s2, ok := r.doc.Content["summary"].(string); ok {
				summary = s2
			}
		}
		items[i] = resultItem{
			ID:       r.doc.ID,
			Score:    r.score,
			Type:     r.doc.Type,
			Summary:  summary,
			Tags:     r.doc.Tags,
			Landmark: r.doc.Landmark,
		}
	}
	text, _ := json.MarshalIndent(items, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

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
		em := transponder.New(s.momDir, true)
		em.EmitConsumptionEvent(transponder.ConsumptionEvent{
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
	docType := stringArg(args, "type")
	summary := stringArg(args, "summary")
	tags := stringSliceArg(args, "tags")
	content, _ := args["content"].(map[string]any)

	if docType == "" || summary == "" || len(tags) == 0 {
		return toolCallResult{}, fmt.Errorf("type, summary, and tags are required")
	}
	if content == nil {
		content = map[string]any{}
	}

	// Use nearest writable scope or fall back to leoDir.
	targetDir := s.momDir
	if sc, ok := scope.NearestWritable(s.momDir); ok {
		targetDir = sc.Path
	}

	// Generate a slug ID from summary.
	id := slugify(summary)
	now := time.Now().UTC()

	doc := &memory.Doc{
		ID:             id,
		Type:           docType,
		Lifecycle:      "learning",
		Scope:          "project",
		Tags:           tags,
		Summary:        summary,
		Created:        now,
		CreatedBy:      "mcp",
		Updated:        now,
		UpdatedBy:      "mcp",
		Confidence:     "INFERRED",
		PromotionState: "draft",
		Classification: "INTERNAL",
		Compartments:   map[string][]string{},
		Provenance: &memory.Provenance{
			Runtime:      "mcp",
			TriggerEvent: "create_memory_draft",
		},
		Content: content,
	}

	memDir := filepath.Join(targetDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return toolCallResult{}, fmt.Errorf("creating memory dir: %w", err)
	}

	path := filepath.Join(memDir, id+".json")
	if err := memory.SaveDoc(path, doc); err != nil {
		return toolCallResult{}, fmt.Errorf("saving draft: %w", err)
	}

	result := map[string]any{
		"id":              id,
		"promotion_state": "draft",
		"path":            path,
		"message":         fmt.Sprintf("Draft memory created at %s", path),
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

	targetScopes := scopes
	if scopeLabel != "" {
		targetScopes = nil
		for _, sc := range scopes {
			if sc.Label == scopeLabel {
				targetScopes = append(targetScopes, sc)
				break
			}
		}
	}

	type landmarkItem struct {
		ID              string   `json:"id"`
		Type            string   `json:"type"`
		Summary         string   `json:"summary"`
		Tags            []string `json:"tags"`
		CentralityScore float64  `json:"centrality_score"`
	}
	var items []landmarkItem

	for _, sc := range targetScopes {
		memDir := filepath.Join(sc.Path, "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
			if err != nil {
				continue
			}
			if !doc.Landmark {
				continue
			}
			score := 0.0
			if doc.CentralityScore != nil {
				score = *doc.CentralityScore
			}
			summary := doc.Summary
			if summary == "" {
				if s2, ok := doc.Content["summary"].(string); ok {
					summary = s2
				}
			}
			items = append(items, landmarkItem{
				ID:              doc.ID,
				Type:            doc.Type,
				Summary:         summary,
				Tags:            doc.Tags,
				CentralityScore: score,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CentralityScore > items[j].CentralityScore
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	if len(items) == 0 {
		return toolCallResult{Content: []toolContent{{Type: "text", Text: "No landmarks found."}}}, nil
	}

	text, _ := json.MarshalIndent(items, "", "  ")
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// --- Scoring (mirrors recall.go) ---

const landmarkBoost = 0.3

func scoreMemory(doc *memory.Doc, query string, filterTags map[string]bool) float64 {
	// If tag filter specified, doc must match ALL filter tags.
	if len(filterTags) > 0 {
		docTagSet := make(map[string]bool, len(doc.Tags))
		for _, t := range doc.Tags {
			docTagSet[t] = true
		}
		for tag := range filterTags {
			if !docTagSet[tag] {
				return 0
			}
		}
	}

	var score float64

	if query != "" {
		for _, tag := range doc.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				score += 1.0
			}
		}
		summary := strings.ToLower(doc.Summary)
		if summary == "" {
			if s, ok := doc.Content["summary"].(string); ok {
				summary = strings.ToLower(s)
			}
		}
		if strings.Contains(summary, query) {
			score += 1.5
		}
		for _, v := range doc.Content {
			if s, ok := v.(string); ok {
				if strings.Contains(strings.ToLower(s), query) {
					score += 0.5
					break
				}
			}
		}
	}

	if query == "" && len(filterTags) == 0 {
		score = 0.1
	}
	if doc.Landmark {
		score += landmarkBoost
	}
	if query == "" && len(filterTags) > 0 && score == landmarkBoost {
		score = 1.0 + landmarkBoost
	} else if query == "" && len(filterTags) > 0 {
		score = 1.0
	}
	if score == 0 && query != "" {
		return 0
	}

	return score
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
