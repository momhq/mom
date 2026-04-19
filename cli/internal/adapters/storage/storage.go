// Package storage defines the StorageAdapter interface for KB persistence.
package storage

import (
	"time"

	"github.com/vmarinogg/leo-core/cli/internal/kb"
)

// Doc represents a KB document in the storage layer.
type Doc struct {
	ID              string              `json:"id"`
	Type            string              `json:"type"`
	Boot            bool                `json:"boot,omitempty"`
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
	Provenance      *kb.Provenance      `json:"provenance,omitempty"`
	Landmark        bool                `json:"landmark,omitempty"`
	CentralityScore *float64            `json:"centrality_score,omitempty"`
	Content         map[string]any      `json:"content"`
}

// Index represents the KB index.
type Index struct {
	Version     string              `json:"version"`
	LastRebuilt string              `json:"last_rebuilt"`
	ByTag       map[string][]string `json:"by_tag"`
	ByType      map[string][]string `json:"by_type"`
	ByScope     map[string][]string `json:"by_scope"`
	ByLifecycle map[string][]string `json:"by_lifecycle"`
}

// QueryFilter defines criteria for querying KB documents.
type QueryFilter struct {
	Tags      []string
	Type      string
	Scope     string
	Lifecycle string
}

// HealthStatus reports the storage backend's health.
type HealthStatus struct {
	OK      bool
	Message string
}

// Adapter is the interface that storage backends must implement.
// The JSON adapter (free tier) reads/writes flat JSON files in .leo/memory/.
// Future adapters (MongoDB, etc.) implement the same interface.
type Adapter interface {
	Read(id string) (*Doc, error)
	Write(doc *Doc) error
	Query(filter QueryFilter) ([]*Doc, error)
	Delete(id string) error
	List() (*Index, error)
	BulkWrite(docs []*Doc) error
	Health() (*HealthStatus, error)
}
