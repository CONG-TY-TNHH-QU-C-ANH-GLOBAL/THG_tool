package sessions

import "database/sql"

// Migrate creates the browser_sessions table and its additive columns.
// Called from the parent store's schema bootstrap BEFORE the sessions.Store
// instance is constructed — mirrors the prompts.Migrate / connectors.
// InitSelectorCache pre-construction bootstrap pattern. Idempotent — safe to
// run on every boot.
//
// PR2 fix (2026-07-01): browser_sessions was never in the versioned SQL
// migrations (internal/store/migrations/*.sql) — it only existed as a side
// effect of the legacy *AppStore.migrate() bootstrap, which PR1 (2026-07-01)
// wired sessions.Store independently of. Once PR2 migrates callers off
// *AppStore, nothing guarantees AppStore.migrate() ran first, so
// sessions.Store needs its own copy of this bootstrap. The SQL is byte-
// identical to AppStore.migrate()'s browser_sessions statements (still left
// in place there, unmodified — both are idempotent, so running both is
// harmless while *AppStore still exists).
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS browser_sessions (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id     INTEGER NOT NULL UNIQUE,
		org_id         INTEGER NOT NULL DEFAULT 0,
		status         TEXT    NOT NULL DEFAULT 'idle',
		cdp_port       INTEGER NOT NULL DEFAULT 0,
		vnc_port       INTEGER NOT NULL DEFAULT 0,
		started_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		error_msg      TEXT    NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	// Additive columns (idempotent — errors ignored), byte-identical to the
	// ALTER TABLE statements in the legacy AppStore.migrate().
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN version        INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN worker_id      TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN retry_count    INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN heartbeat_at   DATETIME`)
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN status_prev    TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN checkpoint_url TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE browser_sessions ADD COLUMN checkpoint_at  DATETIME`)
	return nil
}
