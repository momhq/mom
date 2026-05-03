// Package store is the v0.30 domain layer over the SQLite vault. It
// owns the MemoryStore (CRUD on memories with substance immutability)
// and the GraphStore (tags, entities, edges).
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/momhq/mom/cli/internal/vault"
)

// Memory is the v0.30 in-memory shape of a memory document. Substance
// fields are immutable after Insert; operational fields are mutated
// only via the explicit Set* methods on MemoryStore.
type Memory struct {
	// Substance — immutable after Insert.
	ID                     string
	Type                   string
	Summary                string
	Content                map[string]any
	CreatedAt              time.Time
	SessionID              string
	ProvenanceActor        string
	ProvenanceSourceType   string
	ProvenanceTriggerEvent string

	// Operational — mutable via Set* methods.
	PromotionState  string
	Landmark        bool
	CentralityScore *float64
}

// MemoryStore is the CRUD surface for memories. Substance immutability
// is enforced by API absence: there is no public method to mutate
// substance fields after Insert.
type MemoryStore struct {
	v *vault.Vault
}

// NewMemoryStore returns a MemoryStore backed by the given vault.
func NewMemoryStore(v *vault.Vault) *MemoryStore {
	return &MemoryStore{v: v}
}

// nowTimestamp returns the current UTC time formatted for storage in
// the *_at TEXT columns. RFC3339 with nanosecond precision keeps
// ordering stable for high-frequency capture.
func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// Insert persists the memory and returns the canonical Memory with ID
// and CreatedAt populated. ID is minted as a UUID v4 if empty; Type
// defaults to "untyped"; PromotionState defaults to "draft"; CreatedAt
// defaults to time.Now().UTC().
func (s *MemoryStore) Insert(m Memory) (Memory, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.Type == "" {
		m.Type = "untyped"
	}
	if m.PromotionState == "" {
		m.PromotionState = "draft"
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}

	contentJSON, err := json.Marshal(m.Content)
	if err != nil {
		return Memory{}, fmt.Errorf("marshal content: %w", err)
	}

	err = s.v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memories (
				id, type, summary, content, created_at, session_id,
				provenance_actor, provenance_source_type, provenance_trigger_event,
				promotion_state, landmark, centrality_score
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Type, m.Summary, string(contentJSON), m.CreatedAt.Format(time.RFC3339Nano),
			m.SessionID, m.ProvenanceActor, m.ProvenanceSourceType, m.ProvenanceTriggerEvent,
			m.PromotionState, m.Landmark, m.CentralityScore,
		)
		return err
	})
	if err != nil {
		return Memory{}, fmt.Errorf("insert memory: %w", err)
	}
	return m, nil
}

// Get retrieves a memory by ID. Returns ErrNotFound if no such memory
// exists.
func (s *MemoryStore) Get(id string) (Memory, error) {
	var (
		m         Memory
		contentJS string
		createdAt string
	)
	err := s.v.Query(
		`SELECT id, type, summary, content, created_at, session_id,
			provenance_actor, provenance_source_type, provenance_trigger_event,
			promotion_state, landmark, centrality_score
		 FROM memories WHERE id = ?`,
		[]any{id},
		func(rows *sql.Rows) error {
			if !rows.Next() {
				// rows.Next() returns false on either end-of-results
				// OR a mid-iteration failure. Surface the real error
				// rather than reporting it as ErrNotFound.
				if err := rows.Err(); err != nil {
					return err
				}
				return ErrNotFound
			}
			return rows.Scan(
				&m.ID, &m.Type, &m.Summary, &contentJS, &createdAt, &m.SessionID,
				&m.ProvenanceActor, &m.ProvenanceSourceType, &m.ProvenanceTriggerEvent,
				&m.PromotionState, &m.Landmark, &m.CentralityScore,
			)
		},
	)
	if err != nil {
		return Memory{}, err
	}
	if err := json.Unmarshal([]byte(contentJS), &m.Content); err != nil {
		return Memory{}, fmt.Errorf("unmarshal content: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Memory{}, fmt.Errorf("parse created_at: %w", err)
	}
	m.CreatedAt = t
	return m, nil
}

// SetType mutates the type field of an existing memory. Returns
// ErrNotFound if no memory with the given ID exists. Substance fields
// are untouched.
func (s *MemoryStore) SetType(id, t string) error {
	return s.updateOperationalField(id, "type", t)
}

// SetPromotionState mutates the promotion_state field of an existing
// memory. Returns ErrNotFound if no memory with the given ID exists.
func (s *MemoryStore) SetPromotionState(id, state string) error {
	return s.updateOperationalField(id, "promotion_state", state)
}

// SetLandmark marks (or unmarks) a memory as a landmark.
func (s *MemoryStore) SetLandmark(id string, landmark bool) error {
	return s.updateOperationalField(id, "landmark", landmark)
}

// SetCentralityScore sets the centrality_score field. A nil pointer
// sets the column to NULL (the default state for non-landmark
// memories); a non-nil pointer stores the dereferenced value.
func (s *MemoryStore) SetCentralityScore(id string, score *float64) error {
	return s.updateOperationalField(id, "centrality_score", score)
}

// updateOperationalField runs a single-column UPDATE on memories,
// returning ErrNotFound if the row does not exist. The column name is
// trusted (caller is in-package); the value is parameterised.
func (s *MemoryStore) updateOperationalField(id, column string, value any) error {
	return s.v.Tx(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			fmt.Sprintf(`UPDATE memories SET %s = ? WHERE id = ?`, column),
			value, id,
		)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// GraphStore manages tags, entities, and the edges that connect
// memories to them.
type GraphStore struct {
	v *vault.Vault
}

// NewGraphStore returns a GraphStore backed by the given vault.
func NewGraphStore(v *vault.Vault) *GraphStore {
	return &GraphStore{v: v}
}

// UpsertEntity returns the existing entity's ID for the given (type,
// display_name), or creates a new entity (with a fresh UUID) and
// returns the new ID. Idempotent on (type, display_name). Rejects
// empty type or display_name — empty identifiers are not meaningful
// and are almost always upstream bugs.
func (g *GraphStore) UpsertEntity(typ, displayName string) (string, error) {
	if typ == "" {
		return "", fmt.Errorf("UpsertEntity: type cannot be empty")
	}
	if displayName == "" {
		return "", fmt.Errorf("UpsertEntity: display_name cannot be empty")
	}
	var id string
	err := g.v.Tx(func(tx *sql.Tx) error {
		row := tx.QueryRow(
			`SELECT id FROM entities WHERE type = ? AND display_name = ?`,
			typ, displayName,
		)
		if err := row.Scan(&id); err == nil {
			return nil
		}
		id = uuid.NewString()
		_, err := tx.Exec(
			`INSERT INTO entities (id, type, display_name, created_at)
			 VALUES (?, ?, ?, ?)`,
			id, typ, displayName, nowTimestamp(),
		)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("upsert entity (%s, %s): %w", typ, displayName, err)
	}
	return id, nil
}

// LinkEntity connects a memory to an entity with a relationship label
// (e.g. "created_by"). Idempotent on (memory_id, entity_id, relationship).
func (g *GraphStore) LinkEntity(memoryID, entityID, relationship string) error {
	return g.v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO memory_entities (memory_id, entity_id, relationship, created_at)
			 VALUES (?, ?, ?, ?)`,
			memoryID, entityID, relationship,
			nowTimestamp(),
		)
		return err
	})
}

// RenameTag changes the name of an existing tag. Only the tags row is
// mutated — memory_tags edges still reference the same tag.id, and
// memory substance is untouched (ADR 0010). Returns ErrNotFound if no
// tag with oldName exists.
func (g *GraphStore) RenameTag(oldName, newName string) error {
	return g.v.Tx(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE tags SET name = ? WHERE name = ?`,
			newName, oldName,
		)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// MergeTags repoints all memory_tags edges from the source tag to the
// target tag, then deletes the source tag row. If a memory is already
// linked to both source and target, the duplicate edge is dropped (the
// PRIMARY KEY constraint is honored via INSERT OR IGNORE). Memory
// substance is untouched (ADR 0010 — graph-level operation). Returns
// ErrNotFound if either tag does not exist. Returns an error if source
// and target are the same name — without this guard, a typo would
// silently wipe all edges and the tag itself.
//
// Comparison is case-sensitive: MergeTags("mcp", "MCP") is a valid
// merge of two distinct tags. Tag-name normalization (kebab-case) is
// a convention, not enforced by the schema.
func (g *GraphStore) MergeTags(sourceName, targetName string) error {
	if sourceName == targetName {
		return fmt.Errorf("MergeTags: source and target are the same (%q)", sourceName)
	}
	return g.v.Tx(func(tx *sql.Tx) error {
		var srcID, tgtID string
		if err := tx.QueryRow(
			`SELECT id FROM tags WHERE name = ?`, sourceName,
		).Scan(&srcID); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		if err := tx.QueryRow(
			`SELECT id FROM tags WHERE name = ?`, targetName,
		).Scan(&tgtID); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}

		// Add (memory, target) for any memory currently linked to source
		// but not yet to target. Original created_at is preserved.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO memory_tags (memory_id, tag_id, created_at)
			 SELECT memory_id, ?, created_at FROM memory_tags WHERE tag_id = ?`,
			tgtID, srcID,
		); err != nil {
			return err
		}
		// Remove all (memory, source) edges.
		if _, err := tx.Exec(
			`DELETE FROM memory_tags WHERE tag_id = ?`, srcID,
		); err != nil {
			return err
		}
		// Drop the source tag row.
		if _, err := tx.Exec(
			`DELETE FROM tags WHERE id = ?`, srcID,
		); err != nil {
			return err
		}
		return nil
	})
}

// LinkTag connects a memory to a tag. Idempotent — re-linking the same
// pair is a no-op (matches ON CONFLICT DO NOTHING semantics).
func (g *GraphStore) LinkTag(memoryID, tagID string) error {
	return g.v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO memory_tags (memory_id, tag_id, created_at)
			 VALUES (?, ?, ?)`,
			memoryID, tagID, nowTimestamp(),
		)
		return err
	})
}

// MemoriesByTag returns the IDs of all memories linked to the tag with
// the given name. Returns an empty slice if no such tag exists or no
// memories are linked.
func (g *GraphStore) MemoriesByTag(tagName string) ([]string, error) {
	var ids []string
	err := g.v.Query(
		`SELECT mt.memory_id FROM memory_tags mt
		 JOIN tags t ON t.id = mt.tag_id
		 WHERE t.name = ?`,
		[]any{tagName},
		func(rows *sql.Rows) error {
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UpsertTag returns the existing tag's ID for the given name, or
// creates a new tag (with a fresh UUID) and returns the new ID.
// Idempotent: repeated calls with the same name return the same ID.
// Rejects empty name — empty identifiers are not meaningful and are
// almost always upstream bugs.
func (g *GraphStore) UpsertTag(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("UpsertTag: name cannot be empty")
	}
	var id string
	err := g.v.Tx(func(tx *sql.Tx) error {
		row := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, name)
		if err := row.Scan(&id); err == nil {
			return nil
		}
		id = uuid.NewString()
		_, err := tx.Exec(
			`INSERT INTO tags (id, name, created_at) VALUES (?, ?, ?)`,
			id, name, nowTimestamp(),
		)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("upsert tag %q: %w", name, err)
	}
	return id, nil
}
