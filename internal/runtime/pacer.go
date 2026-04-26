package runtime

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// AccountPacer enforces per-account rate limits to avoid Facebook bans.
// All timing state is persisted in account_rate_limits so it survives restarts.
type AccountPacer struct {
	accountID      int64
	db             *sql.DB
	mu             sync.Mutex
	minDelayMs     int
	maxDelayMs     int
	maxLoadsPerHr  int
	maxSessionMins int
	sessionStart   time.Time
}

// NewAccountPacer creates a pacer for the given account with sane defaults.
func NewAccountPacer(accountID int64, db *sql.DB) *AccountPacer {
	return &AccountPacer{
		accountID:      accountID,
		db:             db,
		minDelayMs:     3000,
		maxDelayMs:     8000,
		maxLoadsPerHr:  20,
		maxSessionMins: 30,
		sessionStart:   time.Now(),
	}
}

// WaitBeforeLoad enforces the minimum inter-request delay and hourly cap.
// Returns ErrCooldownActive if the account is in cooldown or session TTL exceeded.
// Returns ErrRateLimitExceeded if the hourly cap would be breached.
func (p *AccountPacer) WaitBeforeLoad(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Session TTL
	if time.Since(p.sessionStart) > time.Duration(p.maxSessionMins)*time.Minute {
		return CDPError{Code: ErrCooldownActive, Message: "session TTL exceeded"}
	}

	row := p.db.QueryRowContext(ctx,
		`SELECT loads_this_hour, hour_start, last_request_at, cooldown_until
		 FROM account_rate_limits WHERE account_id=?`, p.accountID)

	var loads int
	var hourStartStr, lastReqStr, cooldownStr sql.NullString
	if err := row.Scan(&loads, &hourStartStr, &lastReqStr, &cooldownStr); err != nil && err != sql.ErrNoRows {
		return nil // can't read — proceed without limiting
	}

	now := time.Now().UTC()

	// Cooldown check
	if cooldownStr.Valid && cooldownStr.String != "" {
		cd, err := time.Parse(time.RFC3339, cooldownStr.String)
		if err == nil && now.Before(cd) {
			return CDPError{Code: ErrCooldownActive, Message: "cooldown active until " + cd.Format(time.RFC3339)}
		}
	}

	// Hourly cap: reset counter if the hour has rolled over
	hourStart := now
	if hourStartStr.Valid {
		if hs, err := time.Parse(time.RFC3339, hourStartStr.String); err == nil {
			hourStart = hs
		}
	}
	if now.Sub(hourStart) >= time.Hour {
		loads = 0
		hourStart = now
	}
	if loads >= p.maxLoadsPerHr {
		return CDPError{Code: ErrRateLimitExceeded, Message: "hourly page load cap reached"}
	}

	// Min delay since last request
	if lastReqStr.Valid && lastReqStr.String != "" {
		if lastReq, err := time.Parse(time.RFC3339, lastReqStr.String); err == nil {
			elapsed := now.Sub(lastReq)
			jitter := time.Duration(p.minDelayMs+(loads%3)*500) * time.Millisecond
			if elapsed < jitter {
				wait := jitter - elapsed
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
		}
	}

	return nil
}

// RecordLoad updates the rate-limit counters after a successful page load.
func (p *AccountPacer) RecordLoad(ctx context.Context) {
	p.db.ExecContext(ctx, `
		INSERT INTO account_rate_limits (account_id, loads_this_hour, hour_start, last_request_at)
		VALUES (?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(account_id) DO UPDATE SET
			loads_this_hour = CASE
				WHEN (julianday(CURRENT_TIMESTAMP) - julianday(hour_start)) * 24 >= 1
				THEN 1
				ELSE loads_this_hour + 1
			END,
			hour_start = CASE
				WHEN (julianday(CURRENT_TIMESTAMP) - julianday(hour_start)) * 24 >= 1
				THEN CURRENT_TIMESTAMP
				ELSE hour_start
			END,
			last_request_at = CURRENT_TIMESTAMP`,
		p.accountID,
	)
}

// RecordBan marks the account as banned and starts a cooldown period.
func (p *AccountPacer) RecordBan(ctx context.Context, banType string) {
	cooldownUntil := time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339)
	p.db.ExecContext(ctx, `
		INSERT INTO account_rate_limits (account_id, ban_detected_at, ban_type, cooldown_until)
		VALUES (?, CURRENT_TIMESTAMP, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			ban_detected_at = CURRENT_TIMESTAMP,
			ban_type        = excluded.ban_type,
			cooldown_until  = excluded.cooldown_until`,
		p.accountID, banType, cooldownUntil,
	)
}
