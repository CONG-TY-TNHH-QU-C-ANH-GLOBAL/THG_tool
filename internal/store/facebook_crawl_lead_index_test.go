// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestFacebookCrawlLeadIndex_Identity(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	t.Run("duplicate post identity rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 10001)
		run := seedRun(t, ctx, tx, base)
		reserve(t, ctx, tx, base.org, "h1", run)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_lead_index (org_id, post_dedup_hash, run_id) VALUES ($1, 'h1', $2)`,
			base.org, run)
		wantConstraint(t, err, "pk_fb_crawl_lead_index")
	})

	t.Run("same identity in another org accepted", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		a := seedCrawlBaseline(t, ctx, tx, 10002)
		b := seedCrawlBaseline(t, ctx, tx, 10003)
		reserve(t, ctx, tx, a.org, "h1", seedRun(t, ctx, tx, a))
		reserve(t, ctx, tx, b.org, "h1", seedRun(t, ctx, tx, b))
	})

	t.Run("cross-org run provenance rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		a := seedCrawlBaseline(t, ctx, tx, 10004)
		b := seedCrawlBaseline(t, ctx, tx, 10005)
		runA := seedRun(t, ctx, tx, a)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_lead_index (org_id, post_dedup_hash, run_id) VALUES ($1, 'h1', $2)`,
			b.org, runA)
		wantConstraint(t, err, "fk_fb_crawl_lead_index_run")
	})

	t.Run("reservation with null lead_id accepted", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 10006)
		run := seedRun(t, ctx, tx, base)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_lead_index (org_id, post_dedup_hash, run_id, lead_id) VALUES ($1, 'h1', $2, NULL)`,
			base.org, run); err != nil {
			t.Fatalf("null lead_id reservation must be accepted: %v", err)
		}
	})

	t.Run("rolled-back claim leaves no reservation", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 10007)
		run := seedRun(t, ctx, tx, base)
		if _, err := tx.ExecContext(ctx, `SAVEPOINT claim`); err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		reserve(t, ctx, tx, base.org, "rollback-h", run)
		if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT claim`); err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM facebook_crawl_lead_index WHERE org_id = $1 AND post_dedup_hash = 'rollback-h'`,
			base.org).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("rolled-back claim must leave no reservation, found %d", n)
		}
	})

	// Provenance to the canonical leads table is deferred in PR-M2B (leads has
	// no (org_id, id) anchor yet). Pin the deferral so a later PR that adds the
	// FK updates this expectation deliberately.
	t.Run("lead provenance FK is deferred", func(t *testing.T) {
		if constraintOrIndexExists(t, ctx, s, "fk_fb_crawl_lead_index_lead") {
			t.Fatal("fk_fb_crawl_lead_index_lead exists; update lead-provenance tests")
		}
	})
}

func reserve(t *testing.T, ctx context.Context, tx *sql.Tx, org int64, hash string, run int64) {
	t.Helper()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO facebook_crawl_lead_index (org_id, post_dedup_hash, run_id) VALUES ($1, $2, $3)`,
		org, hash, run); err != nil {
		t.Fatalf("reserve %q: %v", hash, err)
	}
}

func seedRun(t *testing.T, ctx context.Context, tx *sql.Tx, s crawlSeed) int64 {
	t.Helper()
	id, err := insertRun(ctx, tx, s, s.source, "succeeded", sql.NullInt64{})
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return id
}
