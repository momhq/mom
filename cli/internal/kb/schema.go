// Package kb provides types and validation for KB documents.
package kb

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"
)

var validID = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var validTypes = map[string]bool{
	"rule": true, "skill": true, "identity": true, "decision": true,
	"pattern": true, "fact": true, "feedback": true, "reference": true, "metric": true,
}

var validLifecycles = map[string]bool{
	"permanent": true, "learning": true, "state": true,
}

var validScopes = map[string]bool{
	"core": true, "project": true,
}

// Doc represents a KB document.
type Doc struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Boot      bool           `json:"boot,omitempty"`
	Lifecycle string         `json:"lifecycle"`
	Scope     string         `json:"scope"`
	Tags      []string       `json:"tags"`
	Created   time.Time      `json:"created"`
	CreatedBy string         `json:"created_by"`
	Updated   time.Time      `json:"updated"`
	UpdatedBy string         `json:"updated_by"`
	Content   map[string]any `json:"content"`
}

// Validate checks the document against the KB schema rules.
func (d *Doc) Validate() error {
	if !validID.MatchString(d.ID) {
		return fmt.Errorf("invalid id %q: must be kebab-case", d.ID)
	}
	if !validTypes[d.Type] {
		return fmt.Errorf("invalid type %q", d.Type)
	}
	if !validLifecycles[d.Lifecycle] {
		return fmt.Errorf("invalid lifecycle %q", d.Lifecycle)
	}
	if !validScopes[d.Scope] {
		return fmt.Errorf("invalid scope %q", d.Scope)
	}
	if len(d.Tags) == 0 {
		return fmt.Errorf("tags must not be empty")
	}
	for _, tag := range d.Tags {
		if !validID.MatchString(tag) {
			return fmt.Errorf("invalid tag %q: must be kebab-case", tag)
		}
	}
	if d.CreatedBy == "" {
		return fmt.Errorf("created_by must not be empty")
	}
	if d.UpdatedBy == "" {
		return fmt.Errorf("updated_by must not be empty")
	}
	if d.Content == nil {
		return fmt.Errorf("content must not be nil")
	}
	return nil
}

// LoadDoc reads and parses a JSON document from disk.
func LoadDoc(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading doc: %w", err)
	}

	var doc Doc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing doc: %w", err)
	}

	return &doc, nil
}

// SaveDoc writes a document as formatted JSON to disk.
func SaveDoc(path string, doc *Doc) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling doc: %w", err)
	}

	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing doc: %w", err)
	}

	return nil
}
