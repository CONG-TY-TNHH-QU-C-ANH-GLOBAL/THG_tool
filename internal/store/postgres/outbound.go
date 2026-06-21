package postgres

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thg/scraper/internal/models"
)

// OutboundStore is the PostgreSQL-backed adapter over the outbound task
// lifecycle. It satisfies the PR10 seam
// (internal/server/agent.OutboundLifecycleRepository) with the same method
// signatures the active SQLite store exposes — see the test-time assertion in
// outbound_test.go. Construct via NewOutboundStore with a pool from Open.
type OutboundStore struct {
	pool *pgxpool.Pool
}

// NewOutboundStore wraps an existing pgx pool. It performs no I/O.
func NewOutboundStore(pool *pgxpool.Pool) *OutboundStore {
	return &OutboundStore{pool: pool}
}

// outboundReadColumns is the PG-strict mirror of internal/store/outbound's
// readColumns. Timestamps stay native (TIMESTAMPTZ -> time scan targets)
// rather than COALESCE'd to text, because PostgreSQL is strictly typed and
// cannot COALESCE a timestamptz with ”. Column ORDER matches scanOutboundRow.
const outboundReadColumns = `id, org_id, type, platform, account_id, target_url, target_name,
	content, context, image_path, execution_state, COALESCE(verification_outcome, ''),
	ai_model, sent_at, created_at, execution_id, created_by`

// scanOutboundRow maps one row selecting outboundReadColumns (in order) into a
// models.OutboundMessage. Named string columns (platform, execution_state,
// verification_outcome) are scanned into plain strings then converted, so the
// mapping never depends on driver reflection for defined types. Nullable
// sent_at scans into sql.NullTime; a NULL leaves the zero time, matching the
// SQLite read path's empty-sent_at behavior.
func scanOutboundRow(rows pgx.Rows) (*models.OutboundMessage, error) {
	var (
		m            models.OutboundMessage
		platform     string
		execState    string
		verifOutcome string
		sentAt       sql.NullTime
	)
	if err := rows.Scan(
		&m.ID, &m.OrgID, &m.Type, &platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &execState, &verifOutcome,
		&m.AIModel, &sentAt, &m.CreatedAt, &m.ExecutionID, &m.CreatedBy,
	); err != nil {
		return nil, err
	}
	m.Platform = models.Platform(platform)
	m.ExecutionState = models.ExecutionState(execState)
	m.VerificationOutcome = models.VerificationOutcome(verifOutcome)
	if sentAt.Valid {
		m.SentAt = sentAt.Time
	}
	return &m, nil
}

// GetOutboundByExecutionStateForOrg lists tenant-scoped tasks, optionally
// filtered by execution state and message type, newest first. Empty execState
// or msgType skips that filter (the ($n = ” OR col = $n) form keeps the SQL
// fully static and parameterized — no dynamic SQL construction). Mirrors
// internal/store/outbound.Store.ListByState.
func (s *OutboundStore) GetOutboundByExecutionStateForOrg(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx,
		`SELECT `+outboundReadColumns+`
		 FROM outbound_messages
		 WHERE org_id = $1
		   AND ($2 = '' OR execution_state = $2)
		   AND ($3 = '' OR type = $3)
		 ORDER BY created_at DESC
		 LIMIT $4`,
		orgID, string(execState), msgType, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, scanErr := scanOutboundRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}
