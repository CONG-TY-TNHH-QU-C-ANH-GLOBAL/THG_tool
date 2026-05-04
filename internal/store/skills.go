package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// migrateSkills creates the org_skills + skill_executions tables. Called
// from Store.migrate. Idempotent — safe to run on every boot.
func (s *Store) migrateSkills() error {
	stmts := []string{
		// Per-org enablement of skills. A row's absence means "use the
		// skill's DefaultEnabled value"; an explicit row overrides.
		`CREATE TABLE IF NOT EXISTS org_skills (
			org_id     INTEGER NOT NULL,
			skill_id   TEXT    NOT NULL,
			enabled    INTEGER NOT NULL DEFAULT 1,
			config     TEXT    NOT NULL DEFAULT '{}',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_by INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (org_id, skill_id)
		)`,
		// Audit trail. Every prompt → resolver → skill execution lands
		// here so operators can answer "why did the bot send X?" without
		// reading code.
		`CREATE TABLE IF NOT EXISTS skill_executions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id      INTEGER NOT NULL,
			user_id     INTEGER NOT NULL DEFAULT 0,
			source      TEXT    NOT NULL,
			skill_id    TEXT    NOT NULL,
			args_json   TEXT    NOT NULL DEFAULT '{}',
			summary     TEXT    NOT NULL DEFAULT '',
			success     INTEGER NOT NULL DEFAULT 0,
			error       TEXT    NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_executions_org
			ON skill_executions(org_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_executions_skill
			ON skill_executions(skill_id, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("skills migrate: %w (stmt: %s)", err, stmt)
		}
	}
	return nil
}

// LoadOrgSkillOverrides returns the explicit enable/disable overrides
// stored for orgID. Map key = skill ID, value = enabled flag. Missing
// keys mean "use the skill's DefaultEnabled value".
//
// Top-level helper (not a Store method) so internal/skills can call it
// without holding a Store receiver type — keeps the package's import
// surface minimal.
func LoadOrgSkillOverrides(ctx context.Context, db *Store, orgID int64) (map[string]bool, error) {
	if db == nil || orgID <= 0 {
		return nil, nil
	}
	rows, err := db.db.QueryContext(ctx,
		`SELECT skill_id, enabled FROM org_skills WHERE org_id = ?`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		var enabled int
		if err := rows.Scan(&id, &enabled); err != nil {
			return nil, err
		}
		out[strings.TrimSpace(id)] = enabled != 0
	}
	return out, rows.Err()
}

// SetOrgSkillEnabled flips one skill's enable flag for orgID. Caller
// must already have verified that the actor has admin rights — the
// store layer does no role check.
func (s *Store) SetOrgSkillEnabled(ctx context.Context, orgID int64, skillID string, enabled bool, updatedBy int64) error {
	if orgID <= 0 || strings.TrimSpace(skillID) == "" {
		return fmt.Errorf("org_id and skill_id are required")
	}
	flag := 0
	if enabled {
		flag = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_skills (org_id, skill_id, enabled, updated_at, updated_by)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
		 ON CONFLICT(org_id, skill_id) DO UPDATE SET
		   enabled = excluded.enabled,
		   updated_at = CURRENT_TIMESTAMP,
		   updated_by = excluded.updated_by`,
		orgID, skillID, flag, updatedBy,
	)
	return err
}

// SetOrgSkillConfig stores admin-controlled per-skill config JSON. The
// AI tool layer is forbidden from writing this — the same way
// outbound_mode is admin-only.
func (s *Store) SetOrgSkillConfig(ctx context.Context, orgID int64, skillID, config string, updatedBy int64) error {
	if orgID <= 0 || strings.TrimSpace(skillID) == "" {
		return fmt.Errorf("org_id and skill_id are required")
	}
	if strings.TrimSpace(config) == "" {
		config = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_skills (org_id, skill_id, enabled, config, updated_at, updated_by)
		 VALUES (?, ?, 1, ?, CURRENT_TIMESTAMP, ?)
		 ON CONFLICT(org_id, skill_id) DO UPDATE SET
		   config = excluded.config,
		   updated_at = CURRENT_TIMESTAMP,
		   updated_by = excluded.updated_by`,
		orgID, skillID, config, updatedBy,
	)
	return err
}

// GetOrgSkillConfig returns the admin-set config JSON for one skill,
// empty string when no override exists.
func (s *Store) GetOrgSkillConfig(ctx context.Context, orgID int64, skillID string) (string, error) {
	var cfg string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(config, '{}') FROM org_skills WHERE org_id = ? AND skill_id = ?`,
		orgID, skillID,
	).Scan(&cfg)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return cfg, err
}

// SkillExecution is the audit row written for every prompt → skill
// invocation. Mirrors the skill_executions table.
type SkillExecution struct {
	ID       int64
	OrgID    int64
	UserID   int64
	Source   string
	SkillID  string
	ArgsJSON string
	Summary  string
	Success  bool
	Error    string
	At       time.Time
}

// RecordSkillExecution writes one audit row. Errors are returned to
// the caller but the registry treats them as non-fatal — failing to
// audit must not block the user-visible result.
func RecordSkillExecution(ctx context.Context, db *Store, e SkillExecution) error {
	if db == nil {
		return nil
	}
	success := 0
	if e.Success {
		success = 1
	}
	source := strings.TrimSpace(e.Source)
	if source == "" {
		source = "api"
	}
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO skill_executions (org_id, user_id, source, skill_id, args_json, summary, success, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.OrgID, e.UserID, source, e.SkillID, e.ArgsJSON, e.Summary, success, e.Error, e.At.UTC().Format(time.RFC3339),
	)
	return err
}

// ListRecentSkillExecutions returns the most recent N audit rows for
// orgID, newest first. Used by the dashboard execution feed.
func (s *Store) ListRecentSkillExecutions(ctx context.Context, orgID int64, limit int) ([]SkillExecution, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, user_id, source, skill_id, args_json, summary, success, error, created_at
		 FROM skill_executions
		 WHERE org_id = ?
		 ORDER BY created_at DESC LIMIT ?`,
		orgID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillExecution
	for rows.Next() {
		var e SkillExecution
		var success int
		var createdAt string
		if err := rows.Scan(&e.ID, &e.OrgID, &e.UserID, &e.Source, &e.SkillID, &e.ArgsJSON, &e.Summary, &success, &e.Error, &createdAt); err != nil {
			return nil, err
		}
		e.Success = success != 0
		e.At = parseSQLiteTime(createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneSkillExecutions deletes audit rows older than maxAge. Wire from
// a daily background tick if the table grows beyond comfort.
func (s *Store) PruneSkillExecutions(ctx context.Context, maxAge time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM skill_executions WHERE created_at < datetime('now', ?)`,
		fmt.Sprintf("-%d seconds", int(maxAge.Seconds())),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
