package storage

import (
	"os"
	"path/filepath"

	"github.com/vmarinogg/leo-core/cli/internal/scope"
)

// InheritedDoc wraps a Doc with metadata about which scope it came from
// and whether it is inherited (read-only from the child's perspective).
type InheritedDoc struct {
	*Doc
	// ScopePath is the .leo/ directory this doc was read from.
	ScopePath string
	// ScopeLabel is the scope label declared in that .leo/'s config.yaml.
	ScopeLabel string
	// Inherited is true for docs that come from an ancestor scope (not the
	// nearest/writable scope). Inherited docs are read-only from the child's
	// perspective — writes always go to the nearest scope.
	Inherited bool
}

// ReadAllScopes walks up from cwd and reads every memory doc from every
// ancestor .leo/ directory. Results are merged nearest-first: when the same
// document ID appears in multiple scopes, the child's copy wins.
//
// Symlinks are not followed (inherited from scope.Walk).
func ReadAllScopes(cwd string) ([]*InheritedDoc, error) {
	scopes := scope.Walk(cwd)

	// Collect docs per scope, nearest first. Track seen IDs so child wins.
	seen := make(map[string]bool)
	var results []*InheritedDoc

	for i, s := range scopes {
		inherited := i > 0 // nearest scope is writable; all others are inherited
		memDir := filepath.Join(s.Path, "memory")

		entries, err := os.ReadDir(memDir)
		if err != nil {
			// Memory dir may not exist (e.g. freshly initialised scope) — skip.
			continue
		}

		adapter := NewJSONAdapter(s.Path)

		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			id := e.Name()[:len(e.Name())-len(".json")]

			// Child copy wins — skip if we already have this ID.
			if seen[id] {
				continue
			}

			doc, err := adapter.Read(id)
			if err != nil {
				continue
			}

			seen[id] = true
			results = append(results, &InheritedDoc{
				Doc:        doc,
				ScopePath:  s.Path,
				ScopeLabel: s.Label,
				Inherited:  inherited,
			})
		}
	}

	return results, nil
}
