package crmleadsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
)

type Store struct {
	db    *sql.DB
	table string
}

// Outbox tables a Store may use. Each delivery target has its OWN table so one
// target being down or slow never delays another (the CRM keeps its own retry
// schedule while NewHub catches up, and vice versa).
const (
	CRMOutboxTable    = "crm_lead_sync_outbox"
	NewHubOutboxTable = "newhub_lead_sync_outbox"
)

type item struct {
	ID       int64
	Payload  payload
	Attempts int
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, table: CRMOutboxTable}
}

// NewStoreFor returns a Store bound to one of the known outbox tables. The name
// is never taken from configuration text, so it cannot inject SQL.
func NewStoreFor(db *sql.DB, table string) *Store {
	if table != CRMOutboxTable && table != NewHubOutboxTable {
		panic("crmleadsync: unknown outbox table " + table)
	}
	return &Store{db: db, table: table}
}

func (s *Store) Enqueue(ctx context.Context, event leadingest.LeadEvent, suggestions ...models.LeadSuggestion) error {
	var suggestion models.LeadSuggestion
	if len(suggestions) > 0 {
		suggestion = suggestions[0]
	}
	p, ok := payloadFor(event, suggestion)
	if !ok {
		return nil
	}
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal CRM lead payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		s.q(`INSERT OR IGNORE INTO {outbox} (event_key, payload_json, state, available_at)
		 VALUES (?, ?, 'pending', 0)`), p.EventKey, string(body))
	if err != nil {
		return fmt.Errorf("enqueue CRM lead: %w", err)
	}
	return nil
}

func (s *Store) Claim(ctx context.Context, now time.Time, limit int) ([]item, error) {
	if limit < 1 {
		return nil, nil
	}
	if limit > 20 {
		limit = 20
	}
	nowUnix := now.UTC().Unix()
	if _, err := s.db.ExecContext(ctx,
		s.q(`UPDATE {outbox} SET state='pending', locked_until=0, available_at=?
		 WHERE state='sending' AND locked_until > 0 AND locked_until <= ?`), nowUnix, nowUnix); err != nil {
		return nil, fmt.Errorf("reclaim CRM lead sync: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		s.q(`SELECT id, payload_json, attempts FROM {outbox}
		 WHERE state='pending' AND available_at <= ? ORDER BY id LIMIT ?`), nowUnix, limit)
	if err != nil {
		return nil, fmt.Errorf("list CRM lead sync: %w", err)
	}
	defer rows.Close()
	var candidates []item
	for rows.Next() {
		var raw string
		var candidate item
		if err := rows.Scan(&candidate.ID, &raw, &candidate.Attempts); err != nil {
			return nil, fmt.Errorf("scan CRM lead sync: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &candidate.Payload); err != nil {
			return nil, fmt.Errorf("decode CRM lead sync %d: %w", candidate.ID, err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	claimed := make([]item, 0, len(candidates))
	for _, candidate := range candidates {
		result, err := s.db.ExecContext(ctx,
			s.q(`UPDATE {outbox} SET state='sending', locked_until=?
		 WHERE id=? AND state='pending' AND available_at <= ?`), nowUnix+30, candidate.ID, nowUnix)
		if err != nil {
			return nil, fmt.Errorf("claim CRM lead sync %d: %w", candidate.ID, err)
		}
		if changed, _ := result.RowsAffected(); changed == 1 {
			claimed = append(claimed, candidate)
		}
	}
	return claimed, nil
}

func (s *Store) Succeed(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		s.q(`UPDATE {outbox} SET state='succeeded', locked_until=0, last_error=''
		 WHERE id=? AND state='sending'`), id)
	return err
}

func (s *Store) Retry(ctx context.Context, item item, now time.Time, message string) error {
	attempts := item.Attempts + 1
	if attempts >= 8 {
		_, err := s.db.ExecContext(ctx,
			s.q(`UPDATE {outbox} SET state='failed', attempts=?, locked_until=0, last_error=?
		 WHERE id=? AND state='sending'`), attempts, truncate(message, 500), item.ID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		s.q(`UPDATE {outbox} SET state='pending', attempts=?, available_at=?, locked_until=0, last_error=?
		 WHERE id=? AND state='sending'`), attempts, now.UTC().Add(retryDelay(attempts)).Unix(), truncate(message, 500), item.ID)
	return err
}

func (s *Store) Block(ctx context.Context, item item, message string) error {
	_, err := s.db.ExecContext(ctx,
		s.q(`UPDATE {outbox} SET state='blocked', attempts=?, locked_until=0, last_error=?
		 WHERE id=? AND state='sending'`), item.Attempts+1, truncate(message, 500), item.ID)
	return err
}

// q puts this Store's outbox table into a statement (table names cannot be bound
// as SQL parameters; the name itself comes only from NewStore/NewStoreFor).
func (s *Store) q(statement string) string {
	return strings.ReplaceAll(statement, "{outbox}", s.table)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		return 5 * time.Second
	}
	delay := 5 * time.Second * time.Duration(1<<(attempt-1))
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
