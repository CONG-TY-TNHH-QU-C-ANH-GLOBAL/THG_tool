package sessions

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Migrate creates the browser_sessions table and its additive columns for
// the given boot dialect. Called from the parent store's schema bootstrap
// BEFORE the sessions.Store instance is constructed — mirrors the
// prompts.Migrate / connectors.InitSelectorCache pre-construction bootstrap
// pattern. Idempotent — safe to run on every boot.
//
// This is the SINGLE owner of the browser_sessions bootstrap (the
// byte-identical duplicate in app.Migrate was removed 2026-07-05, once
// the *AppStore wrapper was gone). browser_sessions is browser/session
// runtime state (local-runtime plane by doctrine) and is deliberately
// NOT in the versioned migrations — see
// internal/store/migrations/README.md "Bootstrap layers".
func Migrate(db *sql.DB, dialect dbutil.Dialect) error {
	if dialect.Name() == "postgres" {
		return migratePostgres(db)
	}
	return migrateSQLite(db)
}

// migrateSQLite is byte-identical to the pre-dialect-split Migrate body —
// SQLite behavior must not change.
func migrateSQLite(db *sql.DB) error {
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

// migratePostgres is the PostgreSQL-valid counterpart of migrateSQLite: same
// table and columns, translated so it does not use SQLite-only syntax
// (INTEGER PRIMARY KEY AUTOINCREMENT, bare DATETIME) that fails on a real PG
// boot. PG natively supports `ADD COLUMN IF NOT EXISTS`, so additive columns
// are checked instead of error-ignored.
func migratePostgres(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS browser_sessions (
		id             BIGSERIAL PRIMARY KEY,
		account_id     BIGINT NOT NULL UNIQUE,
		org_id         BIGINT NOT NULL DEFAULT 0,
		status         TEXT NOT NULL DEFAULT 'idle',
		cdp_port       INTEGER NOT NULL DEFAULT 0,
		vnc_port       INTEGER NOT NULL DEFAULT 0,
		started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		error_msg      TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	stmts := []string{
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS version        INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS worker_id      TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS retry_count    INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS heartbeat_at   TIMESTAMPTZ`,
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS status_prev    TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS checkpoint_url TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE browser_sessions ADD COLUMN IF NOT EXISTS checkpoint_at  TIMESTAMPTZ`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
