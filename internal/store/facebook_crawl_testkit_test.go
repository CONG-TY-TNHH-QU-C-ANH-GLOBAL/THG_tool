// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Shared harness for the PR-M2B Facebook crawl schema constraint tests. Every
// scenario runs against a REAL PostgreSQL database (gated on
// POSTGRES_PLATFORM_TEST_DSN) inside a rolled-back transaction, so nothing
// commits and serial ids never collide across runs. The point of these tests
// is the database-enforced tenant / append-only / idempotency invariants —
// they must not be re-expressed as application checks.

func bootPlatform(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping real PostgreSQL crawl schema validation")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("store.New(postgres dsn): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// beginTx opens a transaction that is always rolled back at test end.
func beginTx(t *testing.T, ctx context.Context, s *Store) *sql.Tx {
	t.Helper()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func wantConstraint(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected constraint %q violation, got nil", name)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError for %q, got %T: %v", name, err, err)
	}
	if pgErr.ConstraintName != name {
		t.Fatalf("expected constraint %q, got %q (%s)", name, pgErr.ConstraintName, pgErr.Message)
	}
}

type crawlSeed struct {
	org       int64
	accountID int64
	campaign  int64
	source    int64
}

func seedCrawlBaseline(t *testing.T, ctx context.Context, tx *sql.Tx, org int64) crawlSeed {
	t.Helper()
	s := crawlSeed{org: org}
	s.accountID = seedCrawlAccount(t, ctx, tx, org)
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_campaigns (org_id, name) VALUES ($1, 'c') RETURNING id`,
		org).Scan(&s.campaign); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	addToPool(t, ctx, tx, org, s.campaign, s.accountID)
	s.source = seedSource(t, ctx, tx, s, "key-a")
	return s
}

func seedCrawlAccount(t *testing.T, ctx context.Context, tx *sql.Tx, org int64) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO accounts (name, org_id) VALUES ('acct', $1) RETURNING id`,
		org).Scan(&id); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

func addToPool(t *testing.T, ctx context.Context, tx *sql.Tx, org, campaign, account int64) {
	t.Helper()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO facebook_crawl_campaign_accounts (org_id, campaign_id, account_id) VALUES ($1, $2, $3)`,
		org, campaign, account); err != nil {
		t.Fatalf("seed pool member: %v", err)
	}
}

func seedSource(t *testing.T, ctx context.Context, tx *sql.Tx, s crawlSeed, key string) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_campaign_sources (org_id, campaign_id, source_url, normalized_source_key)
		 VALUES ($1, $2, 'https://facebook.com/groups/x', $3) RETURNING id`,
		s.org, s.campaign, key).Scan(&id); err != nil {
		t.Fatalf("seed source %q: %v", key, err)
	}
	return id
}

func insertRun(ctx context.Context, tx *sql.Tx, s crawlSeed, source int64, status string, account sql.NullInt64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, account_id, status)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		s.org, s.campaign, source, account, status).Scan(&id)
	return id, err
}

func nullInt(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
