// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// RuntimeEvent is one typed lifecycle event persisted from the
// internal/runtime/events emission stream. The columns are the
// frequently-queried structured fields the dashboard needs without
// JSON-parsing every row; the long tail of context attrs lives in
// attrs_json.
//
// Persistence design (project_runtime_control_plane EXP-1):
//   - Emission stays slog-first (events.Info / events.Warn).
//   - When a Sink is registered (top-level store wires it at boot),
//     each emission ALSO writes a row here. Best-effort — sink
//     failure is logged once and the slog record still landed.
//   - Dashboard "runtime feed" panel reads from this table via
//     ListRecentRuntimeEvents.
type RuntimeEvent struct {
	ID         int64
	OrgID      int64
	AccountID  int64
	Event      string
	Level      string // info | warn | error
	OutboundID int64
	AttemptID  int64
	TargetURL  string
	AttrsJSON  string
	CreatedAt  time.Time
}

// InitRuntimeEvents creates the runtime_events table + indexes. Called
// from the parent store's schema bootstrap BEFORE the coordination
// subpackage instance is constructed — package-level helper taking
// *sql.DB (same pattern as prompts.Migrate, connectors.InitSelectorCache).
// Idempotent.
func InitRuntimeEvents(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS runtime_events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id      INTEGER NOT NULL DEFAULT 0,
			account_id  INTEGER NOT NULL DEFAULT 0,
			event       TEXT NOT NULL,
			level       TEXT NOT NULL DEFAULT 'info',
			outbound_id INTEGER NOT NULL DEFAULT 0,
			attempt_id  INTEGER NOT NULL DEFAULT 0,
			target_url  TEXT NOT NULL DEFAULT '',
			attrs_json  TEXT NOT NULL DEFAULT '{}',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_org_time
			ON runtime_events(org_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_event_time
			ON runtime_events(event, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_outbound
			ON runtime_events(outbound_id, created_at DESC)
			WHERE outbound_id > 0`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("runtime_events migrate: %w (stmt: %s)", err, stmt)
		}
	}
	return nil
}

// recordRuntimeEvent is called from the events sink hook. Best-effort —
// errors do NOT propagate, they only get logged once. The slog record
// is the authoritative emission; the table is a queryable mirror.
//
// attrs is the variadic slog attribute list (alternating key, value).
// Known structural keys are extracted into typed columns; the rest
// land in attrs_json.
func (s *Store) RecordRuntimeEvent(ctx context.Context, level, event string, attrs []any) error {
	if strings.TrimSpace(event) == "" {
		return nil
	}
	row := RuntimeEvent{
		Event: event,
		Level: strings.TrimSpace(level),
	}
	if row.Level == "" {
		row.Level = "info"
	}

	// Walk the attr list, peeling off known structural keys + collecting
	// the rest into a map for JSON serialisation.
	remainder := map[string]any{}
	for i := 0; i+1 < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			continue
		}
		val := attrs[i+1]
		switch key {
		case "org_id":
			row.OrgID = toInt64(val)
		case "account_id":
			row.AccountID = toInt64(val)
		case "outbound_id":
			row.OutboundID = toInt64(val)
		case "attempt_id":
			row.AttemptID = toInt64(val)
		case "target_url":
			if s, ok := val.(string); ok {
				row.TargetURL = s
			}
		default:
			remainder[key] = stringify(val)
		}
	}
	if len(remainder) > 0 {
		b, err := json.Marshal(remainder)
		if err == nil {
			row.AttrsJSON = string(b)
		}
	}
	if row.AttrsJSON == "" {
		row.AttrsJSON = "{}"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runtime_events
		   (org_id, account_id, event, level, outbound_id, attempt_id, target_url, attrs_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		row.OrgID, row.AccountID, row.Event, row.Level,
		row.OutboundID, row.AttemptID, row.TargetURL, row.AttrsJSON,
	)
	return err
}

// ListRecentRuntimeEvents serves the runtime-feed dashboard query.
// Newest-first, bounded by `since`, capped by `limit` (default 100,
// max 500). Org-scoped; pass orgID=0 only from superadmin contexts.
func (s *Store) ListRecentRuntimeEvents(ctx context.Context, orgID int64, since time.Time, limit int) ([]RuntimeEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT id, org_id, account_id, event, level, outbound_id, attempt_id, target_url, attrs_json, created_at
	            FROM runtime_events
	           WHERE created_at >= ?`
	args := []any{since.UTC().Format("2006-01-02 15:04:05")}
	if orgID > 0 {
		query += ` AND (org_id = ? OR org_id = 0)`
		args = append(args, orgID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuntimeEvent
	for rows.Next() {
		var r RuntimeEvent
		var createdAt string
		if err := rows.Scan(&r.ID, &r.OrgID, &r.AccountID, &r.Event, &r.Level,
			&r.OutboundID, &r.AttemptID, &r.TargetURL, &r.AttrsJSON, &createdAt); err != nil {
			return nil, err
		}
		r.CreatedAt = dbutil.ParseSQLiteTime(createdAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneRuntimeEvents drops rows older than the cutoff. The runtime
// events table is a tail — long history belongs in cold-storage logs,
// not the hot transactional DB. Run nightly via the existing reconcile
// cron (or wire as a separate sweep when the table grows).
func (s *Store) PruneRuntimeEvents(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM runtime_events WHERE created_at < ?`,
		olderThan.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- helpers ---

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// stringify converts an arbitrary attr value into a JSON-safe shape.
// Most slog values are primitives; errors get their .Error() string.
func stringify(v any) any {
	switch x := v.(type) {
	case error:
		return x.Error()
	case string, bool, int, int32, int64, float32, float64:
		return x
	default:
		return v
	}
}
