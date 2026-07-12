// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestFacebookCrawlRuns_Integrity(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	t.Run("source from another campaign rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7001)
		var otherCampaign int64
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO facebook_crawl_campaigns (org_id, name) VALUES ($1, 'c2') RETURNING id`,
			base.org).Scan(&otherCampaign); err != nil {
			t.Fatalf("second campaign: %v", err)
		}
		otherSource := seedSource(t, ctx, tx, crawlSeed{org: base.org, campaign: otherCampaign}, "key-z")
		_, err := insertRun(ctx, tx, base, otherSource, "queued", sql.NullInt64{})
		wantConstraint(t, err, "fk_fb_crawl_runs_source")
	})

	t.Run("account outside pool rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7002)
		outside := seedCrawlAccount(t, ctx, tx, base.org)
		_, err := insertRun(ctx, tx, base, base.source, "queued", nullInt(outside))
		wantConstraint(t, err, "fk_fb_crawl_runs_account")
	})

	t.Run("queued and waiting may have null account", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7003)
		if _, err := insertRun(ctx, tx, base, base.source, "queued", sql.NullInt64{}); err != nil {
			t.Fatalf("queued null account must be accepted: %v", err)
		}
		second := seedSource(t, ctx, tx, base, "key-w")
		if _, err := insertRun(ctx, tx, base, second, "waiting_for_connector_upgrade", sql.NullInt64{}); err != nil {
			t.Fatalf("waiting null account must be accepted: %v", err)
		}
	})

	t.Run("running with null account rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7004)
		_, err := insertRun(ctx, tx, base, base.source, "running", sql.NullInt64{})
		wantConstraint(t, err, "ck_fb_crawl_runs_running_requires_account")
	})

	t.Run("negative counter rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7005)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, posts_seen)
			 VALUES ($1, $2, $3, -1)`, base.org, base.campaign, base.source)
		wantConstraint(t, err, "ck_fb_crawl_runs_nonnegative_counters")
	})

	t.Run("non-positive attempt rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7006)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, attempt)
			 VALUES ($1, $2, $3, 0)`, base.org, base.campaign, base.source)
		wantConstraint(t, err, "ck_fb_crawl_runs_attempt")
	})

	t.Run("duplicate task id rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 7007)
		insertTerminalTask(t, ctx, tx, base, base.source, "task-x")
		second := seedSource(t, ctx, tx, base, "key-t")
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, status, task_id)
			 VALUES ($1, $2, $3, 'succeeded', 'task-x')`, base.org, base.campaign, second)
		wantConstraint(t, err, "ux_fb_crawl_runs_org_task")
	})
}

func insertTerminalTask(t *testing.T, ctx context.Context, tx *sql.Tx, s crawlSeed, source int64, task string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, status, task_id)
		 VALUES ($1, $2, $3, 'succeeded', $4)`, s.org, s.campaign, source, task); err != nil {
		t.Fatalf("seed terminal run task %q: %v", task, err)
	}
}
