package crawlrun_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/crawlrun"
)

// These tests exercise the real Postgres queue/lifecycle transactions against a
// live database (gated on POSTGRES_PLATFORM_TEST_DSN). Because the store opens
// its own transactions, tests can't wrap everything in one outer rollback like
// the M2B constraint tests do — instead each test uses a unique org id and
// registers org-scoped cleanup. The DSN may point at a shared database, so
// cleanup deletes only the test's own org rows, never table-wide.

// open returns the crawlrun store under test plus the raw handle used for
// seeding and asserting (both over the same connection). Seeds use the raw
// handle so they never route through the code being tested.
func open(t *testing.T) (*crawlrun.Store, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping crawlrun Postgres tests")
	}
	s, err := store.New(dsn)
	if err != nil {
		t.Fatalf("store.New(postgres dsn): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return crawlrun.NewStore(s.DB(), s.Dialect()), s.DB()
}

func cleanupOrg(t *testing.T, db *sql.DB, org int64) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		// Reverse FK order; best-effort — leftover rows only leak into the next
		// run under this test's own org id, never mask an assertion.
		for _, table := range []string{
			"facebook_crawl_runs",
			"facebook_crawl_campaign_sources",
			"facebook_crawl_campaign_accounts",
			"facebook_crawl_campaigns",
			"accounts",
		} {
			_, _ = db.ExecContext(ctx, "DELETE FROM "+table+" WHERE org_id = $1", org)
		}
	})
}

type seed struct {
	org      int64
	campaign int64
	account  int64
}

// seedCampaign creates an org, one account in the campaign pool, and returns the
// ids. cadenceMinutes/freshnessWindowMinutes configure the campaign row.
func seedCampaign(t *testing.T, db *sql.DB, org int64, cadenceMinutes, freshnessWindowMinutes int) seed {
	t.Helper()
	ctx := context.Background()
	s := seed{org: org}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO accounts (name, org_id) VALUES ('acct', $1) RETURNING id`,
		org).Scan(&s.account); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_campaigns (org_id, name, cadence_minutes, freshness_window_minutes)
		 VALUES ($1, 'c', $2, $3) RETURNING id`,
		org, cadenceMinutes, freshnessWindowMinutes).Scan(&s.campaign); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO facebook_crawl_campaign_accounts (org_id, campaign_id, account_id) VALUES ($1, $2, $3)`,
		org, s.campaign, s.account); err != nil {
		t.Fatalf("seed pool member: %v", err)
	}
	return s
}

func addAccount(t *testing.T, db *sql.DB, s seed) int64 {
	t.Helper()
	ctx := context.Background()
	var acct int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO accounts (name, org_id) VALUES ('acct', $1) RETURNING id`,
		s.org).Scan(&acct); err != nil {
		t.Fatalf("add account: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO facebook_crawl_campaign_accounts (org_id, campaign_id, account_id) VALUES ($1, $2, $3)`,
		s.org, s.campaign, acct); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	return acct
}

// seedSource inserts a source. lastRun nil means "never run" (always due).
func seedSource(t *testing.T, db *sql.DB, s seed, key string, lastRun *time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_campaign_sources
		     (org_id, campaign_id, source_url, normalized_source_key, last_run_at)
		 VALUES ($1, $2, 'https://facebook.com/groups/x', $3, $4) RETURNING id`,
		s.org, s.campaign, key, lastRun).Scan(&id); err != nil {
		t.Fatalf("seed source %q: %v", key, err)
	}
	return id
}

// runningRun seeds a source, enqueues it, and claims it so the returned run is
// in 'running' state — the precondition for heartbeat and recovery tests.
func runningRun(t *testing.T, st *crawlrun.Store, db *sql.DB, s seed, now time.Time) crawlrun.ClaimedRun {
	t.Helper()
	ctx := context.Background()
	seedSource(t, db, s, "run-src", nil)
	if _, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: s.org, Now: now}); err != nil {
		t.Fatalf("enqueue for running run: %v", err)
	}
	claimed, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: s.org, AccountID: s.account, Now: now})
	if err != nil || !ok {
		t.Fatalf("claim running run: ok=%v err=%v", ok, err)
	}
	return claimed
}

func setSourcePreferredAccount(t *testing.T, db *sql.DB, org, sourceID, account int64) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`UPDATE facebook_crawl_campaign_sources SET preferred_account_id = $3 WHERE org_id = $1 AND id = $2`,
		org, sourceID, account); err != nil {
		t.Fatalf("set preferred account: %v", err)
	}
}

func setSourceStatus(t *testing.T, db *sql.DB, org, sourceID int64, status string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`UPDATE facebook_crawl_campaign_sources SET status = $3 WHERE org_id = $1 AND id = $2`,
		org, sourceID, status); err != nil {
		t.Fatalf("set source status: %v", err)
	}
}

func runStatus(t *testing.T, db *sql.DB, org, runID int64) (status string, account sql.NullInt64) {
	t.Helper()
	if err := db.QueryRowContext(context.Background(),
		`SELECT status, account_id FROM facebook_crawl_runs WHERE org_id = $1 AND id = $2`,
		org, runID).Scan(&status, &account); err != nil {
		t.Fatalf("read run %d: %v", runID, err)
	}
	return status, account
}
