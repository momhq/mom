package librarian

import "github.com/momhq/mom/cli/internal/vault"

// Migration is a re-export of vault.Migration so other v0.30 packages
// (Logbook, future Cartographer state) can register their schema
// changes through Librarian without importing Vault directly. Only
// Librarian touches Vault — this alias keeps the rule auditable.
type Migration = vault.Migration

// Migrations returns the schema migrations Librarian owns. Callers pass
// these to vault.Open so the runner applies them at version boundaries.
//
// Per ADR 0011 (substance-immutability) and ADR 0013 (UUID-only IDs):
// substance columns (id, content, summary, created_at, session_id,
// provenance_*) are write-once; operational columns (type,
// promotion_state, landmark, centrality_score) are mutable.
//
// Per ADR 0010 (graph-fluent schema): tags and entities are first-class
// node tables joined through memory_tags and memory_entities edge
// tables.
//
// Per ADR 0014 (Drafter filtering): filter_audit counters are owned
// here and bumped by Drafter through Librarian.
//
// FTS5 column weights are applied at query time by Finder per ADR 0007;
// this slice creates the virtual table with the columns Finder expects.
func Migrations() []vault.Migration {
	return []vault.Migration{
		{
			Version: 2,
			Stmts: []string{
				`CREATE TABLE memories (
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
				`CREATE TABLE tags (
					id         TEXT PRIMARY KEY,
					name       TEXT NOT NULL UNIQUE,
					created_at TEXT NOT NULL
				)`,
				`CREATE TABLE entities (
					id           TEXT PRIMARY KEY,
					type         TEXT NOT NULL,
					display_name TEXT NOT NULL,
					created_at   TEXT NOT NULL,
					UNIQUE (type, display_name)
				)`,
				`CREATE TABLE memory_tags (
					memory_id  TEXT NOT NULL,
					tag_id     TEXT NOT NULL,
					created_at TEXT NOT NULL,
					PRIMARY KEY (memory_id, tag_id),
					FOREIGN KEY (memory_id) REFERENCES memories(id),
					FOREIGN KEY (tag_id)    REFERENCES tags(id)
				)`,
				`CREATE TABLE memory_entities (
					memory_id    TEXT NOT NULL,
					entity_id    TEXT NOT NULL,
					relationship TEXT NOT NULL,
					created_at   TEXT NOT NULL,
					PRIMARY KEY (memory_id, entity_id, relationship),
					FOREIGN KEY (memory_id) REFERENCES memories(id),
					FOREIGN KEY (entity_id) REFERENCES entities(id)
				)`,
				`CREATE TABLE filter_audit (
					category        TEXT PRIMARY KEY,
					redaction_count INTEGER NOT NULL DEFAULT 0,
					last_fired_at   TEXT
				)`,
				// FTS5 over memories. content_text is extracted from the
				// JSON content's $.text field by the sync triggers below.
				// Per ADR 0007 the column weights apply at query time:
				// bm25(memories_fts, 0, 2, 10) — zero on id (opaque UUID),
				// light on summary, heavy on content. Tags are not in
				// FTS5 in v0.30; tag-based recall uses SQL joins on
				// memory_tags (ADR 0010).
				`CREATE VIRTUAL TABLE memories_fts USING fts5(
					id UNINDEXED,
					summary,
					content_text,
					tokenize='porter unicode61'
				)`,
				`CREATE TRIGGER memories_fts_ai AFTER INSERT ON memories BEGIN
					INSERT INTO memories_fts(rowid, id, summary, content_text)
					VALUES (new.rowid, new.id, COALESCE(new.summary, ''), COALESCE(json_extract(new.content, '$.text'), ''));
				END`,
				`CREATE TRIGGER memories_fts_ad AFTER DELETE ON memories BEGIN
					DELETE FROM memories_fts WHERE rowid = old.rowid;
				END`,
				`CREATE TRIGGER memories_fts_au AFTER UPDATE ON memories BEGIN
					DELETE FROM memories_fts WHERE rowid = old.rowid;
					INSERT INTO memories_fts(rowid, id, summary, content_text)
					VALUES (new.rowid, new.id, COALESCE(new.summary, ''), COALESCE(json_extract(new.content, '$.text'), ''));
				END`,
			},
		},
	}
}
