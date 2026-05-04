package logbook

import "github.com/momhq/mom/cli/internal/librarian"

// Migrations returns the schema migrations Logbook owns. Callers
// concatenate these with Librarian's migrations and pass the combined
// list to vault.Open. The list is exposed via librarian.Migration to
// keep "only Librarian imports vault" auditable.
//
// Migration 3 creates the operational stream table. There is NO
// separate `event_log` table; this is the single source of truth for
// "what MOM did" and what `mom lens` reads.
func Migrations() []librarian.Migration {
	return []librarian.Migration{
		{
			Version: 3,
			Stmts: []string{
				`CREATE TABLE op_events (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					event_type  TEXT NOT NULL,
					session_id  TEXT NOT NULL,
					created_at  TEXT NOT NULL,
					payload     TEXT
				)`,
				`CREATE INDEX idx_op_events_type    ON op_events(event_type)`,
				`CREATE INDEX idx_op_events_session ON op_events(session_id)`,
				`CREATE INDEX idx_op_events_created ON op_events(created_at)`,
			},
		},
	}
}
