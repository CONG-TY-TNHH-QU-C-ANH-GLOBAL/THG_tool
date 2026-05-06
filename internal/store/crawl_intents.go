package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultCrawlIntervalMinutes = 30
	defaultCrawlMaxItems        = 50
	maxCrawlMaxItems            = 250
)

// CrawlIntent is an org-scoped, recurring market-intelligence need learned
// from an initial prompt. Scheduled runs reuse this plan without calling AI.
type CrawlIntent struct {
	ID              int64     `json:"id"`
	OrgID           int64     `json:"org_id"`
	AccountID       int64     `json:"account_id"`
	Name            string    `json:"name"`
	Prompt          string    `json:"prompt"`
	Intent          string    `json:"intent"`
	SourceType      string    `json:"source_type"`
	SourceURL       string    `json:"source_url"`
	SourceLabel     string    `json:"source_label"`
	Keywords        []string  `json:"keywords"`
	IntervalMinutes int       `json:"interval_minutes"`
	MaxItems        int       `json:"max_items"`
	Enabled         bool      `json:"enabled"`
	DedupHash       string    `json:"dedup_hash"`
	NextRunAt       time.Time `json:"next_run_at"`
	LastRunAt       time.Time `json:"last_run_at,omitempty"`
	LastTaskID      string    `json:"last_task_id"`
	LastError       string    `json:"last_error"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UpsertCrawlIntent records a reusable crawl plan. It is idempotent per
// org/account/source/keyword set, so repeated prompts refine the same plan
// instead of creating duplicate 24/7 jobs.
func (s *Store) UpsertCrawlIntent(ctx context.Context, in CrawlIntent) (*CrawlIntent, error) {
	normalized, err := normalizeCrawlIntent(in)
	if err != nil {
		return nil, err
	}
	keywordsJSON, _ := json.Marshal(normalized.Keywords)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO org_crawl_intents
			(org_id, account_id, name, prompt, intent, source_type, source_url, source_label,
			 keywords_json, interval_minutes, max_items, enabled, dedup_hash, next_run_at,
			 last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(org_id, dedup_hash) DO UPDATE SET
			account_id = CASE WHEN excluded.account_id > 0 THEN excluded.account_id ELSE org_crawl_intents.account_id END,
			name = excluded.name,
			prompt = excluded.prompt,
			intent = excluded.intent,
			source_type = excluded.source_type,
			source_url = excluded.source_url,
			source_label = excluded.source_label,
			keywords_json = excluded.keywords_json,
			interval_minutes = excluded.interval_minutes,
			max_items = excluded.max_items,
			enabled = 1,
			next_run_at = excluded.next_run_at,
			last_error = '',
			updated_at = CURRENT_TIMESTAMP`,
		normalized.OrgID,
		normalized.AccountID,
		normalized.Name,
		normalized.Prompt,
		normalized.Intent,
		normalized.SourceType,
		normalized.SourceURL,
		normalized.SourceLabel,
		string(keywordsJSON),
		normalized.IntervalMinutes,
		normalized.MaxItems,
		normalized.DedupHash,
		formatDBTime(normalized.NextRunAt),
	)
	if err != nil {
		return nil, err
	}
	return s.getCrawlIntentByHash(ctx, normalized.OrgID, normalized.DedupHash)
}

// ClaimDueCrawlIntents atomically claims due recurring plans by advancing their
// next_run_at inside the same transaction. This keeps multiple API processes
// from enqueueing the same recurring crawl.
func (s *Store) ClaimDueCrawlIntents(ctx context.Context, now time.Time, limit int) ([]CrawlIntent, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
		SELECT id, org_id, account_id, name, prompt, intent, source_type, source_url, source_label,
		       keywords_json, interval_minutes, max_items, enabled, dedup_hash,
		       next_run_at, COALESCE(last_run_at,''), last_task_id, last_error, created_at, updated_at
		FROM org_crawl_intents
		WHERE enabled = 1 AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?`, formatDBTime(now), limit)
	if err != nil {
		return nil, err
	}
	var candidates []CrawlIntent
	for rows.Next() {
		intent, scanErr := scanCrawlIntent(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		candidates = append(candidates, intent)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	claimed := make([]CrawlIntent, 0, len(candidates))
	for _, intent := range candidates {
		interval := time.Duration(intent.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = defaultCrawlIntervalMinutes * time.Minute
		}
		nextRun := now.Add(interval)
		res, err := tx.ExecContext(ctx, `
			UPDATE org_crawl_intents
			SET next_run_at = ?, last_run_at = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND enabled = 1 AND next_run_at <= ?`,
			formatDBTime(nextRun), formatDBTime(now), intent.ID, formatDBTime(now))
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}
		intent.LastRunAt = now
		intent.NextRunAt = nextRun
		claimed = append(claimed, intent)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Store) MarkCrawlIntentRunResult(ctx context.Context, id int64, taskID, errMsg string) error {
	taskID = strings.TrimSpace(taskID)
	errMsg = strings.TrimSpace(errMsg)
	if errMsg == "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE org_crawl_intents
			SET last_task_id = ?, last_error = '', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			taskID, id)
		return err
	}
	// Disable intent after 2 consecutive failures (last_error already set → 2nd failure).
	// This prevents errored intents from spamming the scheduler indefinitely.
	_, err := s.db.ExecContext(ctx, `
		UPDATE org_crawl_intents
		SET last_task_id = ?,
		    last_error = ?,
		    enabled = CASE WHEN last_error != '' THEN 0 ELSE enabled END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		taskID, errMsg, id)
	return err
}

func (s *Store) ListCrawlIntentsForOrg(ctx context.Context, orgID int64, limit int) ([]CrawlIntent, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("org_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, org_id, account_id, name, prompt, intent, source_type, source_url, source_label,
		       keywords_json, interval_minutes, max_items, enabled, dedup_hash,
		       next_run_at, COALESCE(last_run_at,''), last_task_id, last_error, created_at, updated_at
		FROM org_crawl_intents
		WHERE org_id = ?
		ORDER BY enabled DESC, next_run_at ASC
		LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CrawlIntent
	for rows.Next() {
		intent, err := scanCrawlIntent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, intent)
	}
	return out, rows.Err()
}

func (s *Store) SetCrawlIntentEnabled(ctx context.Context, orgID, id int64, enabled bool) error {
	if orgID <= 0 || id <= 0 {
		return fmt.Errorf("org_id and id are required")
	}
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE org_crawl_intents
		SET enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND org_id = ?`, v, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) getCrawlIntentByHash(ctx context.Context, orgID int64, hash string) (*CrawlIntent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, org_id, account_id, name, prompt, intent, source_type, source_url, source_label,
		       keywords_json, interval_minutes, max_items, enabled, dedup_hash,
		       next_run_at, COALESCE(last_run_at,''), last_task_id, last_error, created_at, updated_at
		FROM org_crawl_intents
		WHERE org_id = ? AND dedup_hash = ?`, orgID, hash)
	intent, err := scanCrawlIntent(row)
	if err != nil {
		return nil, err
	}
	return &intent, nil
}

type crawlIntentScanner interface {
	Scan(dest ...any) error
}

func scanCrawlIntent(row crawlIntentScanner) (CrawlIntent, error) {
	var out CrawlIntent
	var keywordsJSON string
	var enabled int
	var nextRun, lastRun, createdAt, updatedAt string
	err := row.Scan(
		&out.ID, &out.OrgID, &out.AccountID, &out.Name, &out.Prompt, &out.Intent,
		&out.SourceType, &out.SourceURL, &out.SourceLabel, &keywordsJSON,
		&out.IntervalMinutes, &out.MaxItems, &enabled, &out.DedupHash,
		&nextRun, &lastRun, &out.LastTaskID, &out.LastError, &createdAt, &updatedAt,
	)
	if err != nil {
		return out, err
	}
	_ = json.Unmarshal([]byte(keywordsJSON), &out.Keywords)
	out.Enabled = enabled == 1
	out.NextRunAt = parseDBTime(nextRun)
	out.LastRunAt = parseDBTime(lastRun)
	out.CreatedAt = parseDBTime(createdAt)
	out.UpdatedAt = parseDBTime(updatedAt)
	return out, nil
}

func normalizeCrawlIntent(in CrawlIntent) (CrawlIntent, error) {
	in.OrgID = maxInt64(in.OrgID, 0)
	if in.OrgID <= 0 {
		return in, fmt.Errorf("org_id is required for recurring crawl intent")
	}
	in.SourceURL = strings.TrimSpace(in.SourceURL)
	if in.SourceURL == "" {
		return in, fmt.Errorf("source_url is required for recurring crawl intent")
	}
	in.Intent = strings.TrimSpace(in.Intent)
	if in.Intent == "" {
		in.Intent = "facebook_crawl"
	}
	in.SourceType = strings.TrimSpace(in.SourceType)
	if in.SourceType == "" {
		in.SourceType = "facebook_group"
	}
	in.SourceLabel = strings.TrimSpace(in.SourceLabel)
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.Keywords = normalizeIntentKeywords(in.Keywords)
	if in.IntervalMinutes < defaultCrawlIntervalMinutes {
		in.IntervalMinutes = defaultCrawlIntervalMinutes
	}
	if in.IntervalMinutes > 24*60 {
		in.IntervalMinutes = 24 * 60
	}
	if in.MaxItems <= 0 {
		in.MaxItems = defaultCrawlMaxItems
	}
	if in.MaxItems > maxCrawlMaxItems {
		in.MaxItems = maxCrawlMaxItems
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = deriveCrawlIntentName(in)
	}
	if in.NextRunAt.IsZero() {
		in.NextRunAt = time.Now().UTC().Add(time.Duration(in.IntervalMinutes) * time.Minute)
	}
	in.DedupHash = crawlIntentDedupHash(in)
	in.Enabled = true
	return in, nil
}

func normalizeIntentKeywords(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\t'
		}) {
			part = strings.ToLower(strings.TrimSpace(part))
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

func crawlIntentDedupHash(in CrawlIntent) string {
	keywords := append([]string(nil), in.Keywords...)
	sort.Strings(keywords)
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"org=%d|account=%d|intent=%s|source_type=%s|source_url=%s|keywords=%s",
		in.OrgID,
		in.AccountID,
		strings.ToLower(strings.TrimSpace(in.Intent)),
		strings.ToLower(strings.TrimSpace(in.SourceType)),
		strings.ToLower(strings.TrimSpace(in.SourceURL)),
		strings.Join(keywords, ","),
	)))
	return hex.EncodeToString(sum[:])
}

func deriveCrawlIntentName(in CrawlIntent) string {
	if in.SourceLabel != "" && in.SourceLabel != "prompt_url" && in.SourceLabel != "group_search" {
		return in.SourceLabel
	}
	if len(in.Keywords) > 0 {
		limit := len(in.Keywords)
		if limit > 3 {
			limit = 3
		}
		return strings.Join(in.Keywords[:limit], ", ")
	}
	return in.SourceType
}

func formatDBTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func parseDBTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func maxInt64(v, min int64) int64 {
	if v < min {
		return min
	}
	return v
}
