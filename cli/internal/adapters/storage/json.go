package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vmarinogg/leo-core/cli/internal/kb"
)

// JSONAdapter implements the Adapter interface using flat JSON files
// in .mom/memory/ with an index at .mom/index.json.
type JSONAdapter struct {
	docsDir   string
	indexPath string
}

// NewJSONAdapter creates a JSONAdapter for the given .mom/ directory.
func NewJSONAdapter(leoDir string) *JSONAdapter {
	return &JSONAdapter{
		docsDir:   filepath.Join(leoDir, "memory"),
		indexPath: filepath.Join(leoDir, "index.json"),
	}
}

func (a *JSONAdapter) Read(id string) (*Doc, error) {
	path := filepath.Join(a.docsDir, id+".json")
	kbDoc, err := kb.LoadDoc(path)
	if err != nil {
		return nil, fmt.Errorf("reading doc %q: %w", id, err)
	}
	return kbDocToStorage(kbDoc), nil
}

func (a *JSONAdapter) Write(doc *Doc) error {
	kbDoc := storageDocToKB(doc)
	if err := kbDoc.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	path := filepath.Join(a.docsDir, doc.ID+".json")
	if err := os.MkdirAll(a.docsDir, 0755); err != nil {
		return fmt.Errorf("creating docs dir: %w", err)
	}

	if err := kb.SaveDoc(path, kbDoc); err != nil {
		return err
	}

	return a.rebuildIndex()
}

func (a *JSONAdapter) Query(filter QueryFilter) ([]*Doc, error) {
	idx, err := a.List()
	if err != nil {
		return nil, err
	}

	// Collect matching IDs from the index.
	candidates := a.filterIDs(idx, filter)

	var docs []*Doc
	for _, id := range candidates {
		doc, err := a.Read(id)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

func (a *JSONAdapter) Delete(id string) error {
	path := filepath.Join(a.docsDir, id+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("deleting doc %q: %w", id, err)
	}

	return a.rebuildIndex()
}

func (a *JSONAdapter) List() (*Index, error) {
	data, err := os.ReadFile(a.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{Version: "1"}, nil
		}
		return nil, fmt.Errorf("reading index: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}

	return &idx, nil
}

func (a *JSONAdapter) BulkWrite(docs []*Doc) error {
	for _, doc := range docs {
		kbDoc := storageDocToKB(doc)
		if err := kbDoc.Validate(); err != nil {
			return fmt.Errorf("validation failed for %q: %w", doc.ID, err)
		}

		path := filepath.Join(a.docsDir, doc.ID+".json")
		if err := os.MkdirAll(a.docsDir, 0755); err != nil {
			return fmt.Errorf("creating docs dir: %w", err)
		}

		if err := kb.SaveDoc(path, kbDoc); err != nil {
			return err
		}
	}

	return a.rebuildIndex()
}

func (a *JSONAdapter) Health() (*HealthStatus, error) {
	info, err := os.Stat(a.docsDir)
	if err != nil {
		return &HealthStatus{OK: false, Message: fmt.Sprintf("docs dir not accessible: %v", err)}, nil
	}
	if !info.IsDir() {
		return &HealthStatus{OK: false, Message: "docs path is not a directory"}, nil
	}

	// Try writing a temp file to verify write access.
	tmp := filepath.Join(a.docsDir, ".health-check")
	if err := os.WriteFile(tmp, []byte("ok"), 0644); err != nil {
		return &HealthStatus{OK: false, Message: fmt.Sprintf("no write access: %v", err)}, nil
	}
	os.Remove(tmp)

	return &HealthStatus{OK: true, Message: "ok"}, nil
}

// Reindex publicly exposes rebuildIndex for callers outside this package
// (e.g. the update command after copying files directly to the docs dir).
func (a *JSONAdapter) Reindex() error {
	return a.rebuildIndex()
}

// rebuildIndex scans all docs and rebuilds the index.json.
func (a *JSONAdapter) rebuildIndex() error {
	entries, err := os.ReadDir(a.docsDir)
	if err != nil {
		return fmt.Errorf("reading docs dir: %w", err)
	}

	byTag := make(map[string][]string)
	byType := make(map[string][]string)
	byScope := make(map[string][]string)
	byLifecycle := make(map[string][]string)
	total := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(a.docsDir, e.Name())
		doc, err := kb.LoadDoc(path)
		if err != nil {
			continue
		}

		total++
		id := doc.ID

		for _, tag := range doc.Tags {
			byTag[tag] = appendUnique(byTag[tag], id)
		}
		byType[doc.Type] = appendUnique(byType[doc.Type], id)
		byScope[doc.Scope] = appendUnique(byScope[doc.Scope], id)
		byLifecycle[doc.Lifecycle] = appendUnique(byLifecycle[doc.Lifecycle], id)
	}

	// Sort all slices for deterministic output.
	sortMapValues(byTag)
	sortMapValues(byType)
	sortMapValues(byScope)
	sortMapValues(byLifecycle)

	// Find most connected tag.
	mostConnected := ""
	maxConns := 0
	for tag, ids := range byTag {
		if len(ids) > maxConns {
			maxConns = len(ids)
			mostConnected = tag
		}
	}

	// Count unique tags.
	totalTags := len(byTag)

	// Count docs by type.
	docsByType := make(map[string]int)
	for t, ids := range byType {
		docsByType[t] = len(ids)
	}

	idx := map[string]any{
		"version":      "1",
		"last_rebuilt": time.Now().UTC().Format(time.RFC3339),
		"stats": map[string]any{
			"total_docs":         total,
			"total_tags":         totalTags,
			"docs_by_type":       docsByType,
			"stale_count":        0,
			"most_connected_tag": mostConnected,
		},
		"by_tag":       byTag,
		"by_type":      byType,
		"by_scope":     byScope,
		"by_lifecycle": byLifecycle,
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling index: %w", err)
	}

	data = append(data, '\n')
	if err := os.WriteFile(a.indexPath, data, 0644); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	return nil
}

func (a *JSONAdapter) filterIDs(idx *Index, filter QueryFilter) []string {
	seen := make(map[string]bool)
	var result []string

	addAll := func(ids []string) {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}

	hasFilter := false

	if filter.Type != "" {
		hasFilter = true
		if ids, ok := idx.ByType[filter.Type]; ok {
			addAll(ids)
		}
	}
	if filter.Scope != "" {
		hasFilter = true
		if ids, ok := idx.ByScope[filter.Scope]; ok {
			if hasFilter {
				result = intersect(result, ids)
			} else {
				addAll(ids)
			}
		}
	}
	if filter.Lifecycle != "" {
		hasFilter = true
		if ids, ok := idx.ByLifecycle[filter.Lifecycle]; ok {
			if len(result) > 0 {
				result = intersect(result, ids)
			} else {
				addAll(ids)
			}
		}
	}
	for _, tag := range filter.Tags {
		hasFilter = true
		if ids, ok := idx.ByTag[tag]; ok {
			if len(result) > 0 {
				result = intersect(result, ids)
			} else {
				addAll(ids)
			}
		}
	}

	if !hasFilter {
		// No filter — return all doc IDs from all types.
		for _, ids := range idx.ByType {
			addAll(ids)
		}
	}

	sort.Strings(result)
	return result
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

func sortMapValues(m map[string][]string) {
	for _, v := range m {
		sort.Strings(v)
	}
}

func intersect(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var result []string
	for _, s := range a {
		if set[s] {
			result = append(result, s)
		}
	}
	return result
}

// Conversion helpers between storage.Doc and kb.Doc.
func kbDocToStorage(d *kb.Doc) *Doc {
	return &Doc{
		ID:              d.ID,
		Type:            d.Type,
		Boot:            d.Boot,
		Lifecycle:       d.Lifecycle,
		Scope:           d.Scope,
		Tags:            d.Tags,
		Created:         d.Created,
		CreatedBy:       d.CreatedBy,
		Updated:         d.Updated,
		UpdatedBy:       d.UpdatedBy,
		SessionID:       d.SessionID,
		Confidence:      d.Confidence,
		PromotionState:  d.PromotionState,
		Classification:  d.Classification,
		Compartments:    d.Compartments,
		Provenance:      d.Provenance,
		Landmark:        d.Landmark,
		CentralityScore: d.CentralityScore,
		Content:         d.Content,
	}
}

func storageDocToKB(d *Doc) *kb.Doc {
	return &kb.Doc{
		ID:              d.ID,
		Type:            d.Type,
		Boot:            d.Boot,
		Lifecycle:       d.Lifecycle,
		Scope:           d.Scope,
		Tags:            d.Tags,
		Created:         d.Created,
		CreatedBy:       d.CreatedBy,
		Updated:         d.Updated,
		UpdatedBy:       d.UpdatedBy,
		SessionID:       d.SessionID,
		Confidence:      d.Confidence,
		PromotionState:  d.PromotionState,
		Classification:  d.Classification,
		Compartments:    d.Compartments,
		Provenance:      d.Provenance,
		Landmark:        d.Landmark,
		CentralityScore: d.CentralityScore,
		Content:         d.Content,
	}
}
