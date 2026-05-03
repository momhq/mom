package vault

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// T1 (tracer bullet): Open on a fresh path creates the DB file and
// returns a usable Vault.
func TestOpen_FreshPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mom.db")

	v, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%s): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = v.Close() })

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("expected DB file at %s: %v", dbPath, err)
	}
}

// newVault is a test helper that opens a fresh Vault in a temp dir and
// registers cleanup. Returns the Vault and its on-disk path.
func newVault(t *testing.T) (*Vault, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mom.db")
	v, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v, dbPath
}

// hasTable reports whether a table exists in the vault's schema.
func hasTable(t *testing.T, v *Vault, name string) bool {
	t.Helper()
	var found bool
	err := v.Query(
		`SELECT 1 FROM sqlite_master WHERE type='table' AND name=?`,
		[]any{name},
		func(rows *sql.Rows) error {
			if rows.Next() {
				found = true
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Query sqlite_master for %s: %v", name, err)
	}
	return found
}

// countSchemaMigrations returns the number of applied migration rows.
func countSchemaMigrations(t *testing.T, v *Vault) int {
	t.Helper()
	var count int
	err := v.Query(
		`SELECT COUNT(*) FROM schema_migrations`,
		nil,
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&count)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	return count
}

// T3: Migrate is idempotent — calling it on an already-migrated vault
// leaves schema_migrations unchanged. (Open auto-migrates, so newVault
// returns a vault that has already had Migrate run once.)
func TestMigrate_Idempotent(t *testing.T) {
	v, _ := newVault(t)

	first := countSchemaMigrations(t, v)
	if first < 1 {
		t.Fatalf("expected at least 1 applied migration from auto-migrate, got %d", first)
	}

	if err := v.Migrate(); err != nil {
		t.Fatalf("explicit Migrate after auto-migrate: %v", err)
	}
	second := countSchemaMigrations(t, v)

	if first != second {
		t.Errorf("schema_migrations count changed across re-run: %d -> %d", first, second)
	}
}

// T4: Migrate creates the full v0.30 table set per the design doc and
// ADRs 0009/0010/0014.
func TestMigrate_CreatesAllV030Tables(t *testing.T) {
	v, _ := newVault(t)

	expected := []string{
		"memories",
		"entities",
		"tags",
		"memory_tags",
		"memory_entities",
		"event_log",
		"filter_audit",
		"schema_migrations",
	}
	for _, name := range expected {
		if !hasTable(t, v, name) {
			t.Errorf("expected table %q after Migrate", name)
		}
	}
}

// insertSimpleMemory writes a minimum memory row through the public Tx
// API. Used by tests that need a memory present without going through
// the full MemoryStore (which lives in #235). content is JSON-encoded
// per the v0.30 schema contract: {"text": "..."}.
func insertSimpleMemory(t *testing.T, v *Vault, id, summary, contentText string) {
	t.Helper()
	contentJSON, err := json.Marshal(map[string]string{"text": contentText})
	if err != nil {
		t.Fatalf("marshal content for %s: %v", id, err)
	}
	err = v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memories (id, type, summary, content, created_at)
			 VALUES (?, 'untyped', ?, ?, ?)`,
			id, summary, string(contentJSON), "2026-05-03T12:00:00Z",
		)
		return err
	})
	if err != nil {
		t.Fatalf("insert memory %s: %v", id, err)
	}
}

// T5: After Migrate, an inserted memory is findable via FTS5 search.
// Verifies that the memories_fts virtual table is wired and that
// triggers keep it in sync with memories on insert.
func TestFTS5_FindsInsertedMemory(t *testing.T) {
	v, _ := newVault(t)

	insertSimpleMemory(t, v, "m1", "quick brown fox summary", "the quick brown fox jumps over the lazy dog")

	var foundID string
	err := v.Query(
		`SELECT id FROM memories_fts WHERE memories_fts MATCH ? LIMIT 1`,
		[]any{"quick"},
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&foundID)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("FTS5 query: %v", err)
	}
	if foundID != "m1" {
		t.Errorf("expected FTS5 to find m1 via 'quick', got %q", foundID)
	}
}

// memoryExists is a query helper for the Tx tests.
func memoryExists(t *testing.T, v *Vault, id string) bool {
	t.Helper()
	var found bool
	err := v.Query(
		`SELECT 1 FROM memories WHERE id = ?`,
		[]any{id},
		func(rows *sql.Rows) error {
			if rows.Next() {
				found = true
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("query memories for %s: %v", id, err)
	}
	return found
}

// T6: Tx commits on success — mutations performed inside the callback
// persist after the callback returns nil.
func TestTx_CommitsOnSuccess(t *testing.T) {
	v, _ := newVault(t)

	insertSimpleMemory(t, v, "commit1", "committed memory", "this should persist")

	if !memoryExists(t, v, "commit1") {
		t.Errorf("expected commit1 to persist after Tx commit")
	}
}

// T7: Tx rolls back on error — when the callback returns a non-nil
// error, mutations performed inside the callback do not persist.
func TestTx_RollsBackOnError(t *testing.T) {
	v, _ := newVault(t)

	sentinel := errors.New("intentional rollback")
	contentJSON := `{"text":"this should not persist"}`

	err := v.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO memories (id, type, content, created_at)
			 VALUES (?, 'untyped', ?, ?)`,
			"rollback1", contentJSON, "2026-05-03T12:00:00Z",
		); err != nil {
			return err
		}
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error from Tx, got %v", err)
	}

	if memoryExists(t, v, "rollback1") {
		t.Errorf("expected rollback1 NOT to persist after Tx rollback")
	}
}

// T8: Inserting novel provenance values succeeds — no CHECK constraints
// on actor / source_type / trigger_event. This locks in the open
// vocabulary contract (ADR 0015 / UTM principle): renames are
// forward-only, new values appear over time and are valid by appearing.
func TestProvenance_OpenVocabulary(t *testing.T) {
	v, _ := newVault(t)

	err := v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memories (
				id, type, content, created_at,
				provenance_actor, provenance_source_type, provenance_trigger_event
			) VALUES (?, 'untyped', ?, ?, ?, ?, ?)`,
			"novel1", `{"text":"x"}`, "2026-05-03T12:00:00Z",
			"future-harness-2027", "novel-source-type", "novel-trigger",
		)
		return err
	})
	if err != nil {
		t.Errorf("expected novel provenance values to succeed (open vocabulary): %v", err)
	}
}

// T9: Inserting a memory without specifying type defaults to 'untyped'
// (ADR 0012). Capture never assigns a real type; classification is
// post-hoc through wrap-up or explicit user/agent action.
func TestMemory_DefaultsToUntyped(t *testing.T) {
	v, _ := newVault(t)

	err := v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memories (id, content, created_at)
			 VALUES (?, ?, ?)`,
			"default1", `{"text":"x"}`, "2026-05-03T12:00:00Z",
		)
		return err
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var typeVal string
	err = v.Query(
		`SELECT type FROM memories WHERE id = ?`,
		[]any{"default1"},
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&typeVal)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("query type: %v", err)
	}

	if typeVal != "untyped" {
		t.Errorf("expected default type='untyped', got %q", typeVal)
	}
}

// T14: A migration that fails partway rolls back atomically — the
// statements that ran before the failure leave no schema artifacts,
// and the migration version is not recorded in schema_migrations.
// Forces injection of a deliberately bad migration via the package-
// level slice; cleanup restores the original.
func TestMigrate_RollsBackOnPartialFailure(t *testing.T) {
	v, _ := newVault(t)

	saved := migrations
	t.Cleanup(func() { migrations = saved })

	migrations = append(append([]migration{}, saved...), migration{
		version: 999,
		stmts: []string{
			`CREATE TABLE will_be_rolled_back (id INTEGER PRIMARY KEY)`,
			`THIS IS DELIBERATELY NOT VALID SQL`,
		},
	})

	if err := v.Migrate(); err == nil {
		t.Fatal("expected Migrate to return an error from the failing migration")
	}

	if hasTable(t, v, "will_be_rolled_back") {
		t.Errorf("partial migration committed: will_be_rolled_back table exists after rollback")
	}

	var version999Count int
	if err := v.Query(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 999`,
		nil,
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&version999Count)
			}
			return nil
		},
	); err != nil {
		t.Fatalf("query schema_migrations for version 999: %v", err)
	}
	if version999Count != 0 {
		t.Errorf("expected version 999 NOT to be in schema_migrations, got count=%d", version999Count)
	}
}

// T13: memories.content is enforced as valid JSON via a CHECK
// constraint. The schema contract says content is JSON (e.g.
// {"text": "..."}); rejecting non-JSON at INSERT prevents the FTS5
// trigger from exploding on json_extract and gives a clear error
// rather than a confusing trigger failure.
func TestMemory_RejectsNonJSONContent(t *testing.T) {
	v, _ := newVault(t)

	err := v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memories (id, type, content, created_at)
			 VALUES (?, 'untyped', ?, ?)`,
			"bad-json-1", "this is not valid json", "2026-05-03T12:00:00Z",
		)
		return err
	})
	if err == nil {
		t.Errorf("expected CHECK (json_valid(content)) to reject non-JSON content")
	}
}

// T12: A freshly opened vault has file mode 0600. The DB file may
// contain captured memories, redaction-adjacent transient state, and
// audit metadata; group/world readability is the wrong default.
// Skipped on Windows since POSIX mode bits do not apply.
func TestOpen_FileModeIs0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode not applicable on Windows")
	}

	_, dbPath := newVault(t)

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat %s: %v", dbPath, err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("expected file mode 0600, got %o", mode)
	}
}

// T11: Foreign key enforcement is on for every connection in the pool.
// Without the DSN-embedded `_pragma=foreign_keys(1)`, FK enforcement is
// per-connection and may silently disable when the pool grows. We
// force a fresh connection by closing idle ones, then verify both that
// PRAGMA foreign_keys reports 1 and that an FK violation is rejected.
func TestForeignKeys_EnforcedAcrossConnections(t *testing.T) {
	v, _ := newVault(t)

	// Drop idle connections so the next query needs a fresh one.
	v.db.SetMaxIdleConns(0)

	var fk int
	if err := v.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("query PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys=1 on fresh connection, got %d", fk)
	}

	err := v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memory_tags (memory_id, tag_id, created_at) VALUES (?, ?, ?)`,
			"nonexistent-memory", "nonexistent-tag", "2026-05-03T12:00:00Z",
		)
		return err
	})
	if err == nil {
		t.Errorf("expected FK violation inserting memory_tags with nonexistent FKs; foreign_keys not enforced")
	}
}

// T10 (smoke): round-trip a memory + a tag + an entity edge through
// Tx, then read back the joined view. Exercises the v0.30 graph-fluent
// schema end-to-end (memory + memory_tags + memory_entities) and the
// foreign key relationships introduced in migration 1.
func TestSmoke_RoundTripMemoryWithTagAndEntity(t *testing.T) {
	v, _ := newVault(t)

	const ts = "2026-05-03T12:00:00Z"
	memID := "smoke-mem-1"
	tagID := "smoke-tag-1"
	entityID := "smoke-ent-1"

	err := v.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO memories (id, type, summary, content, created_at)
			 VALUES (?, 'untyped', ?, ?, ?)`,
			memID, "smoke summary", `{"text":"smoke body"}`, ts,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO tags (id, name, created_at) VALUES (?, ?, ?)`,
			tagID, "smoke", ts,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO memory_tags (memory_id, tag_id, created_at) VALUES (?, ?, ?)`,
			memID, tagID, ts,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO entities (id, type, display_name, created_at) VALUES (?, 'user', ?, ?)`,
			entityID, "Smoke User", ts,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO memory_entities (memory_id, entity_id, relationship, created_at)
			 VALUES (?, ?, 'created_by', ?)`,
			memID, entityID, ts,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("round-trip Tx: %v", err)
	}

	var (
		gotMem, gotTag, gotEntity, gotRel string
	)
	err = v.Query(
		`SELECT m.id, t.name, e.display_name, me.relationship
		 FROM memories m
		 JOIN memory_tags mt     ON mt.memory_id = m.id
		 JOIN tags t             ON t.id = mt.tag_id
		 JOIN memory_entities me ON me.memory_id = m.id
		 JOIN entities e         ON e.id = me.entity_id
		 WHERE m.id = ?`,
		[]any{memID},
		func(rows *sql.Rows) error {
			if rows.Next() {
				return rows.Scan(&gotMem, &gotTag, &gotEntity, &gotRel)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("joined query: %v", err)
	}

	if gotMem != memID {
		t.Errorf("memory id = %q, want %q", gotMem, memID)
	}
	if gotTag != "smoke" {
		t.Errorf("tag name = %q, want %q", gotTag, "smoke")
	}
	if gotEntity != "Smoke User" {
		t.Errorf("entity display_name = %q, want %q", gotEntity, "Smoke User")
	}
	if gotRel != "created_by" {
		t.Errorf("relationship = %q, want %q", gotRel, "created_by")
	}
}
