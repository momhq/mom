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
	"constraint": true, "skill": true, "identity": true, "decision": true,
	"fact": true, "feedback": true, "reference": true, "session-log": true,
	"pattern": true, "learning": true,
}

var validLifecycles = map[string]bool{
	"permanent": true, "learning": true, "state": true,
}

var validScopes = map[string]bool{
	"core": true, "project": true,
}

var validConfidence = map[string]bool{
	"EXTRACTED": true, "INFERRED": true, "AMBIGUOUS": true,
}

var validPromotionState = map[string]bool{
	"draft": true, "curated": true, "validated": true, "deprecated": true,
}

var validClassification = map[string]bool{
	"PUBLIC": true, "INTERNAL": true, "CONFIDENTIAL": true,
}

// Provenance captures the origin of a memory document.
type Provenance struct {
	Runtime       string `json:"runtime,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	TriggerEvent  string `json:"trigger_event,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	RawExhaustRef string `json:"raw_exhaust_ref,omitempty"`
}

// Doc represents a KB document.
type Doc struct {
	ID              string              `json:"id"`
	Type            string              `json:"type"`
	Boot            bool                `json:"boot,omitempty"`
	Summary         string              `json:"summary,omitempty"`
	Lifecycle       string              `json:"lifecycle"`
	Scope           string              `json:"scope"`
	Tags            []string            `json:"tags"`
	Created         time.Time           `json:"created"`
	CreatedBy       string              `json:"created_by"`
	Updated         time.Time           `json:"updated"`
	UpdatedBy       string              `json:"updated_by"`
	SessionID       string              `json:"session_id,omitempty"`
	Confidence      string              `json:"confidence,omitempty"`
	PromotionState  string              `json:"promotion_state,omitempty"`
	Classification  string              `json:"classification,omitempty"`
	Compartments    map[string][]string `json:"compartments,omitempty"`
	Provenance      *Provenance         `json:"provenance,omitempty"`
	Landmark        bool                `json:"landmark,omitempty"`
	CentralityScore *float64            `json:"centrality_score,omitempty"`
	Content         map[string]any      `json:"content"`
}

// ApplyDefaults fills in safe defaults for any optional fields that are absent.
// This enables legacy memory files (without the new fields) to load cleanly.
func (d *Doc) ApplyDefaults() {
	if d.Confidence == "" {
		d.Confidence = "INFERRED"
	}
	if d.PromotionState == "" {
		d.PromotionState = "draft"
	}
	if d.Classification == "" {
		d.Classification = "INTERNAL"
	}
	if d.Compartments == nil {
		d.Compartments = map[string][]string{}
	}
	if d.Provenance == nil {
		d.Provenance = &Provenance{}
	}
	// Landmark defaults to false (Go zero value) — no action needed.
	// CentralityScore defaults to nil (*float64) — no action needed.
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

	// Validate optional enum fields when present.
	if d.Confidence != "" && !validConfidence[d.Confidence] {
		return fmt.Errorf("invalid confidence %q: must be EXTRACTED, INFERRED, or AMBIGUOUS", d.Confidence)
	}
	if d.PromotionState != "" && !validPromotionState[d.PromotionState] {
		return fmt.Errorf("invalid promotion_state %q: must be draft, curated, validated, or deprecated", d.PromotionState)
	}
	if d.Classification != "" && !validClassification[d.Classification] {
		return fmt.Errorf("invalid classification %q: must be PUBLIC, INTERNAL, or CONFIDENTIAL", d.Classification)
	}
	if d.CentralityScore != nil && (*d.CentralityScore < 0 || *d.CentralityScore > 1) {
		return fmt.Errorf("centrality_score %v is out of range: must be 0.0–1.0", *d.CentralityScore)
	}

	return nil
}

// LoadDoc reads and parses a JSON document from disk, applying safe defaults
// for any new optional fields missing from legacy files.
func LoadDoc(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading doc: %w", err)
	}

	var doc Doc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing doc: %w", err)
	}

	doc.ApplyDefaults()

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
