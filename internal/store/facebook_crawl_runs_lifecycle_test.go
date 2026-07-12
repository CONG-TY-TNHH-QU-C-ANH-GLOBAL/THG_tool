// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestFacebookCrawlRuns_ActiveAndOpen(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	t.Run("two running rows for one account rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 8001)
		if _, err := insertRun(ctx, tx, base, base.source, "running", nullInt(base.accountID)); err != nil {
			t.Fatalf("first running: %v", err)
		}
		second := seedSource(t, ctx, tx, base, "key-2")
		_, err := insertRun(ctx, tx, base, second, "running", nullInt(base.accountID))
		wantConstraint(t, err, "ux_fb_crawl_runs_one_active_account")
	})

	t.Run("different accounts may each run", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 8002)
		acct2 := seedCrawlAccount(t, ctx, tx, base.org)
		addToPool(t, ctx, tx, base.org, base.campaign, acct2)
		src2 := seedSource(t, ctx, tx, base, "key-2")
		if _, err := insertRun(ctx, tx, base, base.source, "running", nullInt(base.accountID)); err != nil {
			t.Fatalf("run 1: %v", err)
		}
		if _, err := insertRun(ctx, tx, base, src2, "running", nullInt(acct2)); err != nil {
			t.Fatalf("run 2 (distinct account) must be accepted: %v", err)
		}
	})

	t.Run("one source cannot have two open runs", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 8003)
		if _, err := insertRun(ctx, tx, base, base.source, "queued", sql.NullInt64{}); err != nil {
			t.Fatalf("first open run: %v", err)
		}
		_, err := insertRun(ctx, tx, base, base.source, "running", nullInt(base.accountID))
		wantConstraint(t, err, "ux_fb_crawl_runs_one_open_source")
	})

	t.Run("multiple terminal rows for a source accepted", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 8004)
		for i := 0; i < 3; i++ {
			if _, err := insertRun(ctx, tx, base, base.source, "succeeded", sql.NullInt64{}); err != nil {
				t.Fatalf("terminal history row %d must be accepted: %v", i, err)
			}
		}
	})
}

func TestFacebookCrawlRuns_RetryLineage(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	t.Run("same-org retry parent accepted", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 9001)
		parent := seedAbandonedParent(t, ctx, tx, base, base.source)
		child := seedSource(t, ctx, tx, base, "key-r")
		if _, err := insertRetry(ctx, tx, base, child, parent); err != nil {
			t.Fatalf("same-org retry must be accepted: %v", err)
		}
	})

	t.Run("cross-org retry parent rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		orgA := seedCrawlBaseline(t, ctx, tx, 9002)
		orgB := seedCrawlBaseline(t, ctx, tx, 9003)
		parent := seedAbandonedParent(t, ctx, tx, orgA, orgA.source)
		_, err := insertRetry(ctx, tx, orgB, orgB.source, parent)
		wantConstraint(t, err, "fk_fb_crawl_runs_retry_parent")
	})

	t.Run("second automatic retry for one parent rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 9004)
		parent := seedAbandonedParent(t, ctx, tx, base, base.source)
		src1 := seedSource(t, ctx, tx, base, "key-r1")
		src2 := seedSource(t, ctx, tx, base, "key-r2")
		if _, err := insertRetry(ctx, tx, base, src1, parent); err != nil {
			t.Fatalf("first retry: %v", err)
		}
		_, err := insertRetry(ctx, tx, base, src2, parent)
		wantConstraint(t, err, "ux_fb_crawl_runs_one_retry_per_parent")
	})

	// The unique retry index + ON CONFLICT DO NOTHING is the concurrency
	// mechanism: two reapers racing on the same abandoned parent yield exactly
	// one child, the loser a silent no-op. Tested serially — the index is the
	// guarantee, not goroutine timing.
	t.Run("conflicting retry insert is a no-op", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 9005)
		parent := seedAbandonedParent(t, ctx, tx, base, base.source)
		src1 := seedSource(t, ctx, tx, base, "key-c1")
		src2 := seedSource(t, ctx, tx, base, "key-c2")
		if n := upsertRetry(t, ctx, tx, base, src1, parent); n != 1 {
			t.Fatalf("first reaper must insert 1 child, got %d", n)
		}
		if n := upsertRetry(t, ctx, tx, base, src2, parent); n != 0 {
			t.Fatalf("second reaper must no-op, got %d rows", n)
		}
	})
}

func seedAbandonedParent(t *testing.T, ctx context.Context, tx *sql.Tx, s crawlSeed, source int64) int64 {
	t.Helper()
	id, err := insertRun(ctx, tx, s, source, "abandoned", sql.NullInt64{})
	if err != nil {
		t.Fatalf("seed abandoned parent: %v", err)
	}
	return id
}

func insertRetry(ctx context.Context, tx *sql.Tx, s crawlSeed, source, parent int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, status, attempt, retry_of_run_id)
		 VALUES ($1, $2, $3, 'queued', 2, $4) RETURNING id`,
		s.org, s.campaign, source, parent).Scan(&id)
	return id, err
}

func upsertRetry(t *testing.T, ctx context.Context, tx *sql.Tx, s crawlSeed, source, parent int64) int64 {
	t.Helper()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, status, attempt, retry_of_run_id)
		 VALUES ($1, $2, $3, 'queued', 2, $4)
		 ON CONFLICT (org_id, retry_of_run_id) WHERE retry_of_run_id IS NOT NULL DO NOTHING`,
		s.org, s.campaign, source, parent)
	if err != nil {
		t.Fatalf("upsert retry: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected: %v", err)
	}
	return n
}
