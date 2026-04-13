// Package storage defines the StorageAdapter interface for KB persistence.
package storage

import "time"

// Doc represents a KB document.
type Doc struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Lifecycle string            `json:"lifecycle"`
	Scope     string            `json:"scope"`
	Tags      []string          `json:"tags"`
	Created   time.Time         `json:"created"`
	CreatedBy string            `json:"created_by"`
	Updated   time.Time         `json:"updated"`
	UpdatedBy string            `json:"updated_by"`
	Content   map[string]any    `json:"content"`
}

// Index represents the KB index.
type Index struct {
	Version     string              `json:"version"`
	LastRebuilt time.Time           `json:"last_rebuilt"`
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
// The JSON adapter (free tier) reads/writes flat JSON files in .leo/kb/docs/.
// Future adapters (MongoDB, etc.) implement the same interface.
type Adapter interface {
	// Read returns a single KB document by ID.
	Read(id string) (*Doc, error)

	// Write validates and persists a document, then rebuilds the index.
	Write(doc *Doc) error

	// Query returns documents matching the filter criteria.
	Query(filter QueryFilter) ([]*Doc, error)

	// Delete removes a document by ID and rebuilds the index.
	Delete(id string) error

	// List returns the full KB index.
	List() (*Index, error)

	// BulkWrite persists multiple documents with a single index rebuild.
	BulkWrite(docs []*Doc) error

	// Health checks the storage backend's status.
	Health() (*HealthStatus, error)
}
