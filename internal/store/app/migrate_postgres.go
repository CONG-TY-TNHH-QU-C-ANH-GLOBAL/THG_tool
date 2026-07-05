// Domain: app (see internal/store/DOMAINS.md)
package app

import (
	"database/sql"
	"fmt"
)

// migratePostgres is the PostgreSQL-valid counterpart of migrateSQLite: same
// tables/columns, translated so it does not use SQLite-only syntax (INTEGER
// PRIMARY KEY AUTOINCREMENT, bare DATETIME, REAL) that fails on a real PG
// boot. Split into its own file (not a branch inside migrate.go) to keep
// both files under the 200-line guardrail.
//
// accounts, selector_cache and their fb_*/checkpoint_count/version/
// fail_count/deprecated/dom_hash columns are already created by the
// versioned platform migration 0101_platform_accounts_connectors__postgres —
// the additive ALTERs below are therefore no-ops on Postgres (`IF NOT
// EXISTS` skips them cleanly) rather than duplicate ownership.
func migratePostgres(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_tasks (
			id             BIGSERIAL PRIMARY KEY,
			task_id        TEXT    NOT NULL UNIQUE,
			org_id         BIGINT  NOT NULL DEFAULT 0,
			intent         TEXT    NOT NULL,
			status         TEXT    NOT NULL DEFAULT 'pending',
			total_fetched  INTEGER NOT NULL DEFAULT 0,
			total_returned INTEGER NOT NULL DEFAULT 0,
			error          TEXT    NOT NULL DEFAULT '',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_tasks_org ON app_tasks(org_id, intent, status, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS task_leads (
			id                  BIGSERIAL PRIMARY KEY,
			task_id             TEXT   NOT NULL,
			org_id              BIGINT NOT NULL DEFAULT 0,
			source_url          TEXT   NOT NULL,
			author_profile_url  TEXT   NOT NULL DEFAULT '',
			author_name         TEXT   NOT NULL DEFAULT '',
			content             TEXT   NOT NULL DEFAULT '',
			lead_score          DOUBLE PRECISION NOT NULL DEFAULT 0,
			category            TEXT   NOT NULL DEFAULT 'cold',
			signals_json        TEXT   NOT NULL DEFAULT '[]',
			thread_role         TEXT   NOT NULL DEFAULT 'intent_originator',
			created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(task_id, source_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_leads_org ON task_leads(org_id, category, lead_score DESC)`,
		// browser_sessions is OWNED by the sessions domain (sessions.Migrate).
		`CREATE TABLE IF NOT EXISTS browser_identities (
			id             BIGSERIAL PRIMARY KEY,
			account_id     BIGINT NOT NULL UNIQUE,
			org_id         BIGINT NOT NULL DEFAULT 0,
			user_agent     TEXT NOT NULL DEFAULT '',
			screen_w       INTEGER NOT NULL DEFAULT 1920,
			screen_h       INTEGER NOT NULL DEFAULT 1080,
			timezone       TEXT NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
			languages      TEXT NOT NULL DEFAULT 'vi-VN,vi',
			webgl_vendor   TEXT NOT NULL DEFAULT '',
			webgl_renderer TEXT NOT NULL DEFAULT '',
			session_state  TEXT NOT NULL DEFAULT 'clean',
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS learning_profiles (
			id                BIGSERIAL PRIMARY KEY,
			org_id            BIGINT NOT NULL UNIQUE,
			keyword_relevance DOUBLE PRECISION NOT NULL DEFAULT 0.40,
			engagement        DOUBLE PRECISION NOT NULL DEFAULT 0.30,
			content_quality   DOUBLE PRECISION NOT NULL DEFAULT 0.30,
			converted_count   INTEGER NOT NULL DEFAULT 0,
			rejected_count    INTEGER NOT NULL DEFAULT 0,
			ignored_count     INTEGER NOT NULL DEFAULT 0,
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS outcome_events (
			id         BIGSERIAL PRIMARY KEY,
			org_id     BIGINT NOT NULL,
			lead_id    BIGINT NOT NULL,
			category   TEXT   NOT NULL,
			outcome    TEXT   NOT NULL,
			score      DOUBLE PRECISION NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_outcome_events_org ON outcome_events(org_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS learning_history (
			id              BIGSERIAL PRIMARY KEY,
			org_id          BIGINT NOT NULL,
			weights_json    TEXT NOT NULL DEFAULT '{}',
			trigger_outcome TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_learning_history_org ON learning_history(org_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS port_registry (
			port       INTEGER PRIMARY KEY,
			port_type  TEXT   NOT NULL DEFAULT 'cdp',
			account_id BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS account_rate_limits (
			account_id      BIGINT PRIMARY KEY,
			loads_this_hour INTEGER NOT NULL DEFAULT 0,
			hour_start      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_request_at TIMESTAMPTZ,
			cooldown_until  TIMESTAMPTZ,
			ban_detected_at TIMESTAMPTZ,
			ban_type        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS circuit_breaker_state (
			scope       TEXT PRIMARY KEY,
			state       TEXT NOT NULL DEFAULT 'closed',
			failures    INTEGER NOT NULL DEFAULT 0,
			first_fail  TIMESTAMPTZ,
			opens_until TIMESTAMPTZ,
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS session_audit_log (
			id           BIGSERIAL PRIMARY KEY,
			account_id   BIGINT NOT NULL,
			from_status  TEXT NOT NULL DEFAULT '',
			to_status    TEXT NOT NULL,
			triggered_by TEXT NOT NULL DEFAULT 'system',
			reason       TEXT NOT NULL DEFAULT '',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_audit_account ON session_audit_log(account_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS post_seen_cache (
			source_url TEXT NOT NULL,
			post_id    TEXT NOT NULL,
			seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (source_url, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_seen_at ON post_seen_cache(seen_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w\nstmt: %s", err, stmt)
		}
	}

	// accounts + selector_cache columns already exist on Postgres — created
	// by 0101_platform_accounts_connectors__postgres. `IF NOT EXISTS` makes
	// these safe no-ops instead of duplicate-column errors.
	noopAlters := []string{
		`ALTER TABLE selector_cache ADD COLUMN IF NOT EXISTS version    INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE selector_cache ADD COLUMN IF NOT EXISTS fail_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE selector_cache ADD COLUMN IF NOT EXISTS deprecated INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE selector_cache ADD COLUMN IF NOT EXISTS dom_hash   TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS fb_user_id        TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS fb_display_name   TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS fb_username       TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS fb_profile_url    TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS checkpoint_count  INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range noopAlters {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w\nstmt: %s", err, stmt)
		}
	}

	// Verified-Actor columns (P1b) on execution_attempts / account_runtime_state
	// are owned by migration 0006_add_actor_verification (SQLite) / the
	// equivalent platform migration (Postgres) — NOT here, same reasoning as
	// migrateSQLite.
	return nil
}
