package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vmarinogg/leo-core/cli/internal/scope"
)

// resourceDef describes one MCP resource for resources/list.
type resourceDef struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

// resourceContent is a single content item in a resources/read response.
type resourceContent struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text"`
}

// allResources returns the static resource catalogue.
func allResources() []resourceDef {
	return []resourceDef{
		{
			URI:         "mom://identity",
			Name:        "MOM Identity",
			Description: "Identity document from the nearest .leo/ scope.",
			MIMEType:    "application/json",
		},
		{
			URI:         "mom://constraints",
			Name:        "MOM Constraints",
			Description: "Constraint summaries from the nearest .leo/ scope.",
			MIMEType:    "application/json",
		},
		{
			URI:         "mom://scopes",
			Name:        "MOM Scopes",
			Description: "Scope hierarchy discovered from the current working directory.",
			MIMEType:    "application/json",
		},
	}
}

// handleResourcesList returns the static resource catalogue.
func (s *Server) handleResourcesList() (any, *rpcError) {
	resources := allResources()
	out := make([]any, len(resources))
	for i, r := range resources {
		out[i] = r
	}
	return map[string]any{"resources": out}, nil
}

// handleResourcesRead reads the requested resource and returns its content.
func (s *Server) handleResourcesRead(params json.RawMessage) (any, *rpcError) {
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "invalid resources/read params: " + err.Error()}
	}

	switch req.URI {
	case "mom://identity":
		return s.readIdentity()
	case "mom://constraints":
		return s.readConstraints()
	case "mom://scopes":
		return s.readScopes()
	default:
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "unknown resource URI: " + req.URI}
	}
}

// readIdentity returns the identity.json from the nearest scope.
func (s *Server) readIdentity() (any, *rpcError) {
	// Look for identity.json in the leoDir first, then walk up.
	candidates := []string{filepath.Join(s.leoDir, "identity.json")}
	for _, sc := range scope.Walk(s.leoDir) {
		candidates = append(candidates, filepath.Join(sc.Path, "identity.json"))
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return map[string]any{
			"contents": []resourceContent{
				{URI: "mom://identity", MIMEType: "application/json", Text: string(data)},
			},
		}, nil
	}

	return map[string]any{
		"contents": []resourceContent{
			{URI: "mom://identity", MIMEType: "application/json", Text: "{}"},
		},
	}, nil
}

// readConstraints returns summaries of all constraint docs in the nearest scope.
func (s *Server) readConstraints() (any, *rpcError) {
	// Collect constraint JSON files from leoDir/constraints/ and walk-up scopes.
	type constraintSummary struct {
		ID      string `json:"id"`
		Summary string `json:"summary,omitempty"`
		Path    string `json:"path"`
	}

	var summaries []constraintSummary
	seen := make(map[string]bool)

	addFromDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if seen[path] {
				continue
			}
			seen[path] = true

			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				continue
			}
			id, _ := raw["id"].(string)
			if id == "" {
				id = strings.TrimSuffix(e.Name(), ".json")
			}
			summary, _ := raw["summary"].(string)
			summaries = append(summaries, constraintSummary{ID: id, Summary: summary, Path: path})
		}
	}

	addFromDir(filepath.Join(s.leoDir, "constraints"))
	for _, sc := range scope.Walk(s.leoDir) {
		addFromDir(filepath.Join(sc.Path, "constraints"))
	}

	text, _ := json.MarshalIndent(summaries, "", "  ")
	return map[string]any{
		"contents": []resourceContent{
			{URI: "mom://constraints", MIMEType: "application/json", Text: string(text)},
		},
	}, nil
}

// readScopes returns the scope hierarchy.
func (s *Server) readScopes() (any, *rpcError) {
	scopes := scope.Walk(s.leoDir)
	if len(scopes) == 0 {
		scopes = []scope.Scope{{Path: s.leoDir, Label: "repo"}}
	}

	type scopeEntry struct {
		Label       string `json:"label"`
		Path        string `json:"path"`
		MemoryCount int    `json:"memory_count"`
	}
	entries := make([]scopeEntry, len(scopes))
	for i, sc := range scopes {
		entries[i] = scopeEntry{
			Label:       sc.Label,
			Path:        sc.Path,
			MemoryCount: sc.MemoryCount(),
		}
	}

	text, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, &rpcError{Code: errCodeInternalError, Message: fmt.Sprintf("marshaling scopes: %v", err)}
	}
	return map[string]any{
		"contents": []resourceContent{
			{URI: "mom://scopes", MIMEType: "application/json", Text: string(text)},
		},
	}, nil
}
