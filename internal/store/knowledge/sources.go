package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Workspace Knowledge OS — Layer 1 persistence.
//
// Every method here enforces strict tenant isolation: org_id is the
// gate, and every public method takes an explicit orgID parameter.
// Read [specs/domains/knowledge-platform/features/knowledge-os/technical.md §6] for the full contract.
//
// Two design choices repeated below for visibility:
//
//  1. Foreign-org Get returns sql.ErrNoRows — observably identical to
//     "row does not exist." This is the same convention as
//     GetAccountForOrg ([internal/store/accounts.go]). It prevents
//     a hostile caller from probing for the existence of another
//     workspace's data via timing or error code.
//
//  2. Operator fields (state, pinned, boost) and ingestor fields
//     (label, connection_config) are written through different SQL
//     statements. A re-sync from an ingestor MUST NOT clobber an
//     operator's hide/pin choice.

// GetSource returns a single source owned by orgID, or (nil,
// sql.ErrNoRows) if no such row exists OR the row belongs to a
// different org. Callers should treat both cases identically.
func (s *Store) GetSource(ctx context.Context, sourceID, orgID int64) (*sources.Source, error) {
	if sourceID <= 0 || orgID <= 0 {
		return nil, sql.ErrNoRows
	}
	row := s.queryRowContext(ctx, sourceSelect+`
		WHERE id = ? AND org_id = ?`, sourceID, orgID)
	src, err := scanSource(row)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	return src, err
}

// ListSourcesForOrg returns every source the org has configured,
// ordered by creation time descending (newest first — matches the
// Sources panel default sort).
func (s *Store) ListSourcesForOrg(ctx context.Context, orgID int64, filter sources.ListFilter) ([]*sources.Source, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge: org_id is required")
	}
	args := []any{orgID}
	q := sourceSelect + ` WHERE org_id = ?`

	if len(filter.Types) > 0 {
		q += ` AND type IN (` + placeholders(len(filter.Types)) + `)`
		for _, t := range filter.Types {
			args = append(args, string(t))
		}
	}
	if len(filter.Health) > 0 {
		q += ` AND health_status IN (` + placeholders(len(filter.Health)) + `)`
		for _, h := range filter.Health {
			args = append(args, string(h))
		}
	}
	q += ` ORDER BY created_at DESC`

	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*sources.Source, 0, 8)
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// UpsertSource inserts a new source row (when ID == 0) or updates the
// operator-controlled fields of an existing one (label, type,
// connection_config, sync_policy). It does NOT touch health_status /
// health_message / last_sync_at — those flow through UpdateSourceHealth
// so an operator-edit never clobbers a fresh sync result.
//
// Returns the persisted source with ID, CreatedAt, UpdatedAt filled.
func (s *Store) UpsertSource(ctx context.Context, src *sources.Source) (*sources.Source, error) {
	if err := src.Validate(); err != nil {
		return nil, err
	}
	cfg := []byte(src.ConnectionConfig)
	if src.ID == 0 {
		// RETURNING-based INSERT is the cross-dialect pattern that
		// works on both SQLite (>=3.35) and PG. See risk R1.
		id, err := s.insertReturningID(ctx, `
			INSERT INTO knowledge_sources
				(org_id, type, label, connection_config, sync_policy,
				 health_status, health_message, last_sync_at, last_sync_asset_count,
				 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id`,
			src.OrgID, string(src.Type), src.Label, string(cfg), string(src.SyncPolicy),
			string(src.Health.Status), src.Health.Message, nullableTime(src.Health.LastSyncAt),
			src.LastAssetCount,
		)
		if err != nil {
			return nil, err
		}
		return s.GetSource(ctx, id, src.OrgID)
	}
	// Update path: org_id is part of the WHERE so a misrouted update
	// against a foreign row silently affects 0 rows.
	res, err := s.execContext(ctx, `
		UPDATE knowledge_sources
		   SET type              = ?,
		       label             = ?,
		       connection_config = ?,
		       sync_policy       = ?,
		       updated_at        = CURRENT_TIMESTAMP
		 WHERE id = ? AND org_id = ?`,
		string(src.Type), src.Label, string(cfg), string(src.SyncPolicy),
		src.ID, src.OrgID,
	)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Either the row doesn't exist or it belongs to another org —
		// observably identical to the caller, by design.
		return nil, sql.ErrNoRows
	}
	return s.GetSource(ctx, src.ID, src.OrgID)
}

// UpdateSourceHealth is the only write path for the health_* columns.
// The ingestor runtime calls this after every sync attempt.
// lastAssetCount is the count the ingestor observed; it is cached on
// the source row so the Sources panel can show "47 assets" without
// scanning the assets table.
func (s *Store) UpdateSourceHealth(ctx context.Context, sourceID, orgID int64, h sources.Health, lastAssetCount int) error {
	if sourceID <= 0 || orgID <= 0 {
		return fmt.Errorf("knowledge: source_id and org_id are required")
	}
	if !h.Status.IsKnown() {
		return errors.New("knowledge: unknown health status: " + string(h.Status))
	}
	res, err := s.execContext(ctx, `
		UPDATE knowledge_sources
		   SET health_status         = ?,
		       health_message        = ?,
		       last_sync_at          = ?,
		       last_sync_asset_count = ?,
		       updated_at            = CURRENT_TIMESTAMP
		 WHERE id = ? AND org_id = ?`,
		string(h.Status), h.Message, nullableTime(h.LastSyncAt), lastAssetCount,
		sourceID, orgID,
	)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteSourceForOrg removes a source and cascades to all assets it
// produced. The deletion runs in a transaction so a partial failure
// (source row gone, assets stranded) is impossible.
//
// Returns the count of assets that were removed alongside the source,
// so callers can audit the blast radius before logging.
//
// L3 note: knowledge owns its own transaction here. Phase 4 audit
// found zero cross-package writers that need to thread an external
// *sql.Tx through this path; if that changes, lift the tx parameter
// rather than reintroducing a parent-managed transaction wrapper.
func (s *Store) DeleteSourceForOrg(ctx context.Context, sourceID, orgID int64) (assetsDeleted int64, err error) {
	if sourceID <= 0 || orgID <= 0 {
		return 0, fmt.Errorf("knowledge: source_id and org_id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	// Cascade explicitly so we can return the count. ON DELETE CASCADE
	// at the SQL level would hide this from the caller.
	res, err := tx.ExecContext(ctx,
		s.dialect.Rebind(`DELETE FROM knowledge_assets WHERE source_id = ? AND org_id = ?`),
		sourceID, orgID,
	)
	if err != nil {
		return 0, err
	}
	assetsDeleted, _ = res.RowsAffected()
	res, err = tx.ExecContext(ctx,
		s.dialect.Rebind(`DELETE FROM knowledge_sources WHERE id = ? AND org_id = ?`),
		sourceID, orgID,
	)
	if err != nil {
		return 0, err
	}
	sourcesDeleted, _ := res.RowsAffected()
	if sourcesDeleted == 0 {
		// Roll back the (no-op) asset deletion too, for clean audit.
		return 0, sql.ErrNoRows
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return assetsDeleted, nil
}

// --- internals ---

const sourceSelect = `
	SELECT id, org_id, type, label, connection_config, sync_policy,
	       health_status, health_message, last_sync_at, last_sync_asset_count,
	       created_at, updated_at
	FROM knowledge_sources`

// scanRow is the narrowest contract sql.Row / sql.Rows both satisfy.
// Both *sql.Row and *sql.Rows expose Scan(...) — this lets the same
// scanner serve QueryRow and Query without duplicating the column list.
type scanRow interface {
	Scan(dest ...any) error
}

func scanSource(r scanRow) (*sources.Source, error) {
	var (
		src           sources.Source
		typ           string
		policy        string
		health        string
		cfg           string
		lastSyncRaw   sql.NullString
		createdAtRaw  string
		updatedAtRaw  string
		healthMessage string
		assetCount    int
	)
	if err := r.Scan(
		&src.ID, &src.OrgID, &typ, &src.Label, &cfg, &policy,
		&health, &healthMessage, &lastSyncRaw, &assetCount,
		&createdAtRaw, &updatedAtRaw,
	); err != nil {
		return nil, err
	}
	src.Type = sources.SourceType(typ)
	src.SyncPolicy = sources.SyncPolicy(policy)
	src.ConnectionConfig = json.RawMessage(cfg)
	src.Health = sources.Health{
		Status:     sources.HealthStatus(health),
		Message:    healthMessage,
		LastSyncAt: optionalSQLiteTime(lastSyncRaw),
	}
	src.LastAssetCount = assetCount
	src.CreatedAt = dbutil.ParseSQLiteTime(createdAtRaw)
	src.UpdatedAt = dbutil.ParseSQLiteTime(updatedAtRaw)
	return &src, nil
}

// nullableTime converts a *time.Time into a value the SQLite driver
// stores as NULL when the pointer is nil, otherwise as a formatted
// timestamp. The driver does not accept time.Time{} for "no time"; it
// stores the zero year. Going through a sql.NullTime is the documented
// way to preserve nil semantics.
func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// optionalSQLiteTime is the symmetric reader. Returns nil for NULL or
// empty strings; otherwise parses via the existing parseSQLiteTime
// helper. Nil-pointer is meaningful — "never synced" / "never
// retrieved" — and must not collapse into the Unix epoch.
func optionalSQLiteTime(v sql.NullString) *time.Time {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	t := dbutil.ParseSQLiteTime(v.String)
	if t.IsZero() {
		return nil
	}
	return &t
}

// placeholders returns "?, ?, ?" for n=3 — used when expanding IN clauses.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}
