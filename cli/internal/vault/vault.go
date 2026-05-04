// Package vault is the persistent memory store for v0.30. It owns the
// SQLite connection at $HOME/.mom/mom.db, runs schema migrations, and
// mediates all reads and writes so callers never touch *sql.DB directly.
package vault

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
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

// init validates that the migrations slice is sorted by version and
// contains no duplicate versions. A bad slice is a programming error
// detected at package load — preferable to silently applying
// migrations in the wrong order or skipping one because of a duplicate
// version number.
func init() {
	if err := validateMigrations(migrations); err != nil {
		panic("vault: invalid migrations: " + err.Error())
	}
}

// validateMigrations returns an error if versions are not strictly
// increasing (out of order or duplicated). Pure function, exposed for
// testing. Empty slice is valid.
func validateMigrations(ms []migration) error {
	for i := 1; i < len(ms); i++ {
		if ms[i].version <= ms[i-1].version {
			return fmt.Errorf("migration %d follows %d (must be strictly increasing)",
				ms[i].version, ms[i-1].version)
		}
	}
	return nil
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
			content                  TEXT NOT NULL CHECK (json_valid(content)),
			created_at               TEXT NOT NULL,
			session_id               TEXT NOT NULL,
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
			created_at   TEXT NOT NULL,
			UNIQUE (type, display_name)
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
			session_id TEXT NOT NULL,
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

// dsnPragmas are SQLite pragmas embedded in the connection string so
// modernc.org/sqlite applies them on EVERY new connection in the pool.
// Per-connection pragmas (foreign_keys, synchronous, cache_size) do not
// persist across pool growth when applied via db.Exec; embedding them
// in the DSN is the documented fix.
//
// WAL mode is database-wide (persists in the file) and is set separately
// via db.Exec after Open since it is not a per-connection setting.
const dsnPragmas = "?_pragma=foreign_keys(1)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_pragma=cache_size(-8000)"

// Open opens or creates the SQLite database at path, applies pragmas,
// runs any pending schema migrations, and returns a usable Vault.
// Callers must Close the Vault when done.
//
// On POSIX systems, the database file is created (or chmod'd if it
// already exists) with mode 0600. The vault may contain captured
// memories; group/world readability is the wrong default. modernc.org/
// sqlite does not expose a perms DSN parameter, so the file is
// materialized here before sql.Open.
//
// Migration is run automatically: a returned Vault is always at the
// current schema version. There is no opt-out — every consumer wants a
// migrated vault, and the central vault has no inspect-before-migrate
// use case (mom upgrade reads legacy per-folder vaults, not this one).
func Open(path string) (*Vault, error) {
	if runtime.GOOS != "windows" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("creating vault file at %s: %w", path, err)
		}
		_ = f.Close()
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("chmod %s to 0600: %w", path, err)
		}
	}

	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite at %s: %w", path, err)
	}
	// WAL is database-wide — set once via Exec; persists in the file.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting journal_mode=WAL: %w", err)
	}

	v := &Vault{db: db}
	if err := v.Migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating vault at %s: %w", path, err)
	}
	return v, nil
}

// Close releases the underlying database connection. Idempotent —
// calling Close on an already-closed Vault is a no-op.
func (v *Vault) Close() error {
	if v.db == nil {
		return nil
	}
	err := v.db.Close()
	v.db = nil
	return err
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

// TODO(#238): add QueryContext / TxContext variants when Recall needs
// query cancellation / timeout. Current callers are all CLI-bound and
// don't yet have a context to thread; adding speculative ctx-taking
// methods now would be premature.

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
