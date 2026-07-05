// Domain: app (see internal/store/DOMAINS.md)
package app

import (
	"database/sql"
	"fmt"
)

// Migrate is the idempotent bootstrap for the app-domain and legacy
// browser-infra tables (app_tasks, task_leads, browser_sessions,
// browser_identities, learning_*, port_registry, account_rate_limits,
// circuit_breaker_state, session_audit_log, post_seen_cache) plus the
// additive column ALTERs. Moved verbatim from the retired
// *AppStore.migrate() (AppStore dissolution PR6, 2026-07-05) — same
// statements, same order. Called by store.New (both dialect paths)
// right after sessions.Migrate, so every process that opens the store
// gets the tables strictly earlier than the old NewAppStore-time
// bootstrap.
func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_tasks (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id        TEXT    NOT NULL UNIQUE,
			org_id         INTEGER NOT NULL DEFAULT 0,
			intent         TEXT    NOT NULL,
			status         TEXT    NOT NULL DEFAULT 'pending',
			total_fetched  INTEGER NOT NULL DEFAULT 0,
			total_returned INTEGER NOT NULL DEFAULT 0,
			error          TEXT    NOT NULL DEFAULT '',
			created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_tasks_org ON app_tasks(org_id, intent, status, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS task_leads (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id             TEXT    NOT NULL,
			org_id              INTEGER NOT NULL DEFAULT 0,
			source_url          TEXT    NOT NULL,
			author_profile_url  TEXT    NOT NULL DEFAULT '',
			author_name         TEXT    NOT NULL DEFAULT '',
			content             TEXT    NOT NULL DEFAULT '',
			lead_score          REAL    NOT NULL DEFAULT 0,
			category            TEXT    NOT NULL DEFAULT 'cold',
			signals_json        TEXT    NOT NULL DEFAULT '[]',
			created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_id, source_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_leads_org ON task_leads(org_id, category, lead_score DESC)`,
		// ── Browser intelligence tables ──────────────────────────────────────────
		// browser_sessions is OWNED by the sessions domain: created + ALTERed
		// by sessions.Migrate, which store.initDomains always runs BEFORE this
		// function. The byte-identical duplicate block that lived here (a
		// tolerated no-op "while *AppStore still exists") was removed 2026-07-05
		// with the AppStore wrapper gone.
		`CREATE TABLE IF NOT EXISTS browser_identities (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id     INTEGER NOT NULL UNIQUE,
			org_id         INTEGER NOT NULL DEFAULT 0,
			user_agent     TEXT    NOT NULL DEFAULT '',
			screen_w       INTEGER NOT NULL DEFAULT 1920,
			screen_h       INTEGER NOT NULL DEFAULT 1080,
			timezone       TEXT    NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
			languages      TEXT    NOT NULL DEFAULT 'vi-VN,vi',
			webgl_vendor   TEXT    NOT NULL DEFAULT '',
			webgl_renderer TEXT    NOT NULL DEFAULT '',
			session_state  TEXT    NOT NULL DEFAULT 'clean',
			updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// ── Self-learning tables ──────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS learning_profiles (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id            INTEGER NOT NULL UNIQUE,
			keyword_relevance REAL    NOT NULL DEFAULT 0.40,
			engagement        REAL    NOT NULL DEFAULT 0.30,
			content_quality   REAL    NOT NULL DEFAULT 0.30,
			converted_count   INTEGER NOT NULL DEFAULT 0,
			rejected_count    INTEGER NOT NULL DEFAULT 0,
			ignored_count     INTEGER NOT NULL DEFAULT 0,
			updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS outcome_events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id     INTEGER NOT NULL,
			lead_id    INTEGER NOT NULL,
			category   TEXT    NOT NULL,
			outcome    TEXT    NOT NULL,
			score      REAL    NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_outcome_events_org ON outcome_events(org_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS learning_history (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id          INTEGER NOT NULL,
			weights_json    TEXT    NOT NULL DEFAULT '{}',
			trigger_outcome TEXT    NOT NULL DEFAULT '',
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_learning_history_org ON learning_history(org_id, created_at DESC)`,
		// ── Production infra tables (phase 2 upgrade) ────────────────────────────
		`CREATE TABLE IF NOT EXISTS port_registry (
			port       INTEGER PRIMARY KEY,
			port_type  TEXT    NOT NULL DEFAULT 'cdp',
			account_id INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS account_rate_limits (
			account_id      INTEGER PRIMARY KEY,
			loads_this_hour INTEGER NOT NULL DEFAULT 0,
			hour_start      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_request_at DATETIME,
			cooldown_until  DATETIME,
			ban_detected_at DATETIME,
			ban_type        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS circuit_breaker_state (
			scope      TEXT PRIMARY KEY,
			state      TEXT    NOT NULL DEFAULT 'closed',
			failures   INTEGER NOT NULL DEFAULT 0,
			first_fail DATETIME,
			opens_until DATETIME,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS session_audit_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id   INTEGER NOT NULL,
			from_status  TEXT    NOT NULL DEFAULT '',
			to_status    TEXT    NOT NULL,
			triggered_by TEXT    NOT NULL DEFAULT 'system',
			reason       TEXT    NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_audit_account ON session_audit_log(account_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS post_seen_cache (
			source_url TEXT NOT NULL,
			post_id    TEXT NOT NULL,
			seen_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (source_url, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_seen_at ON post_seen_cache(seen_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w\nstmt: %s", err, stmt)
		}
	}

	// Additive column migrations (idempotent — errors ignored)
	// Coordination Plane Phase B: thread role axis on the connector lead
	// table. See project_thread_role_architecture.md.
	db.Exec(`ALTER TABLE task_leads ADD COLUMN thread_role TEXT NOT NULL DEFAULT 'intent_originator'`)
	db.Exec(`ALTER TABLE selector_cache ADD COLUMN version    INTEGER NOT NULL DEFAULT 1`)
	db.Exec(`ALTER TABLE selector_cache ADD COLUMN fail_count INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE selector_cache ADD COLUMN deprecated INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE selector_cache ADD COLUMN dom_hash   TEXT    NOT NULL DEFAULT ''`)
	// Checkpoint fields on accounts
	db.Exec(`ALTER TABLE accounts ADD COLUMN fb_user_id        TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE accounts ADD COLUMN fb_display_name   TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE accounts ADD COLUMN fb_username       TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE accounts ADD COLUMN fb_profile_url    TEXT    NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE accounts ADD COLUMN checkpoint_count  INTEGER NOT NULL DEFAULT 0`)

	// NOTE: Verified-Actor columns (P1b) on execution_attempts /
	// account_runtime_state are added by migration 0006_add_actor_verification
	// (the canonical migrator path), NOT here — adding them in both places
	// would make 0006 fail on a duplicate column at boot.

	return nil
}
