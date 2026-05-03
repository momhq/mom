// Package vault is the persistent memory store for v0.30. It owns the
// SQLite connection at $HOME/.mom/mom.db, runs schema migrations, and
// mediates all reads and writes so callers never touch *sql.DB directly.
package vault

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// migration is one applied schema change identified by an integer
// version. Each migration carries one or more statements applied as a
// unit; version is recorded in schema_migrations on success.
type migration struct {
	version int
	stmts   []string
}

// migrations is the ordered list of schema changes for v0.30.
// Re-runs of Migrate skip versions already recorded in schema_migrations.
//
// Provenance columns (provenance_actor, provenance_source_type,
// provenance_trigger_event) are plain TEXT with no CHECK constraints —
// see ADR 0015 (open vocabularies, UTM-style).
//
// type defaults to 'untyped' per ADR 0012; promotion_state defaults to
// 'draft'. There is no valid_to column — temporal validity moves to
// edges in the future relations layer (ADR 0011).
var migrations = []migration{
	{1, []string{
		`CREATE TABLE IF NOT EXISTS memories (
			id                       TEXT PRIMARY KEY,
			type                     TEXT NOT NULL DEFAULT 'untyped',
			summary                  TEXT,
			content                  TEXT NOT NULL,
			created_at               TEXT NOT NULL,
			session_id               TEXT,
			provenance_actor         TEXT,
			provenance_source_type   TEXT,
			provenance_trigger_event TEXT,
			promotion_state          TEXT NOT NULL DEFAULT 'draft',
			landmark                 INTEGER NOT NULL DEFAULT 0,
			centrality_score         REAL
		)`,
		`CREATE TABLE IF NOT EXISTS entities (
			id           TEXT PRIMARY KEY,
			type         TEXT NOT NULL,
			display_name TEXT,
			created_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS memory_tags (
			memory_id  TEXT NOT NULL,
			tag_id     TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (memory_id, tag_id),
			FOREIGN KEY (memory_id) REFERENCES memories(id),
			FOREIGN KEY (tag_id)    REFERENCES tags(id)
		)`,
		`CREATE TABLE IF NOT EXISTS memory_entities (
			memory_id    TEXT NOT NULL,
			entity_id    TEXT NOT NULL,
			relationship TEXT NOT NULL,
			created_at   TEXT NOT NULL,
			PRIMARY KEY (memory_id, entity_id, relationship),
			FOREIGN KEY (memory_id) REFERENCES memories(id),
			FOREIGN KEY (entity_id) REFERENCES entities(id)
		)`,
		`CREATE TABLE IF NOT EXISTS event_log (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			timestamp  TEXT NOT NULL,
			session_id TEXT,
			payload    TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS filter_audit (
			category        TEXT PRIMARY KEY,
			redaction_count INTEGER NOT NULL DEFAULT 0,
			last_fired_at   TEXT
		)`,
		// FTS5 virtual table indexing the searchable content of memories.
		// content_text is extracted from the `content` JSON's `$.text`
		// field by the sync triggers below. Per ADR 0007: bm25 weights
		// applied at query time as bm25(memories_fts, 0, 2, 10) — zero
		// on id (opaque UUID), light on summary, heavy on content. Tags
		// are not in FTS5 in v0.30; tag-based recall uses SQL joins on
		// memory_tags (ADR 0010).
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			id UNINDEXED,
			summary,
			content_text,
			tokenize='porter unicode61'
		)`,
		`CREATE TRIGGER IF NOT EXISTS memories_fts_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, id, summary, content_text)
			VALUES (new.rowid, new.id, COALESCE(new.summary, ''), COALESCE(json_extract(new.content, '$.text'), ''));
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_fts_ad AFTER DELETE ON memories BEGIN
			DELETE FROM memories_fts WHERE rowid = old.rowid;
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_fts_au AFTER UPDATE ON memories BEGIN
			DELETE FROM memories_fts WHERE rowid = old.rowid;
			INSERT INTO memories_fts(rowid, id, summary, content_text)
			VALUES (new.rowid, new.id, COALESCE(new.summary, ''), COALESCE(json_extract(new.content, '$.text'), ''));
		END`,
	}},
}

// Vault is the SQLite-backed memory store. The zero value is not usable;
// construct via Open.
type Vault struct {
	db *sql.DB
}

// pragmas applied once after opening the database. These are the
// production defaults inherited from the pre-0.30 storage layer:
// WAL for concurrent readers, foreign keys enforced, normal sync for
// good performance with WAL safety, ~8 MB page cache.
var openPragmas = []string{
	"PRAGMA journal_mode=WAL",
	"PRAGMA foreign_keys=ON",
	"PRAGMA synchronous=NORMAL",
	"PRAGMA cache_size=-8000",
}

// Open opens or creates the SQLite database at path and returns a usable
// Vault. Callers must Close the Vault when done.
func Open(path string) (*Vault, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite at %s: %w", path, err)
	}
	for _, pragma := range openPragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("applying %q: %w", pragma, err)
		}
	}
	return &Vault{db: db}, nil
}

// Close releases the underlying database connection.
func (v *Vault) Close() error {
	if v.db == nil {
		return nil
	}
	return v.db.Close()
}

// Migrate applies any pending schema migrations to bring the vault to the
// current schema version. Safe to call multiple times — already-applied
// migrations are skipped via the schema_migrations tracking table.
func (v *Vault) Migrate() error {
	if _, err := v.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var applied int
		if err := v.db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`,
			m.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if applied > 0 {
			continue
		}
		// Each migration runs atomically: all statements + the
		// schema_migrations row commit together, or none do.
		if err := v.Tx(func(tx *sql.Tx) error {
			for _, stmt := range m.stmts {
				if _, err := tx.Exec(stmt); err != nil {
					return fmt.Errorf("statement: %w", err)
				}
			}
			_, err := tx.Exec(
				`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
				m.version,
				time.Now().UTC().Format(time.RFC3339),
			)
			return err
		}); err != nil {
			return fmt.Errorf("applying migration %d: %w", m.version, err)
		}
	}
	return nil
}

// Query runs a read-only query and invokes scan with the resulting rows.
// Vault closes the *sql.Rows after scan returns. Callers never see
// *sql.DB.
func (v *Vault) Query(query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := v.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := scan(rows); err != nil {
		return err
	}
	return rows.Err()
}

// Tx runs fn inside a transaction. The transaction commits on nil
// return; any non-nil error (or panic) rolls it back. Callers receive a
// *sql.Tx but never *sql.DB.
func (v *Vault) Tx(fn func(*sql.Tx) error) (err error) {
	tx, err := v.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return err
}
