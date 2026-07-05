package sessions

import "database/sql"

// Migrate creates the browser_sessions table and its additive columns.
// Called from the parent store's schema bootstrap BEFORE the sessions.Store
// instance is constructed — mirrors the prompts.Migrate / connectors.
// InitSelectorCache pre-construction bootstrap pattern. Idempotent — safe to
// run on every boot.
//
// This is the SINGLE owner of the browser_sessions bootstrap (the
// byte-identical duplicate in app.Migrate was removed 2026-07-05, once
// the *AppStore wrapper was gone). browser_sessions is a local-runtime
// plane table: it is deliberately NOT in the versioned migrations —
// see internal/store/migrations/README.md "Bootstrap layers".
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
