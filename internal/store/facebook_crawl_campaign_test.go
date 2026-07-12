// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
)

func TestFacebookCrawlPool_TenantIntegrity(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	t.Run("same-org account joins pool", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 5001)
		other := seedCrawlAccount(t, ctx, tx, base.org)
		addToPool(t, ctx, tx, base.org, base.campaign, other)
	})

	t.Run("nonexistent account rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 5002)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_accounts (org_id, campaign_id, account_id) VALUES ($1, $2, 999999999)`,
			base.org, base.campaign)
		wantConstraint(t, err, "fk_fb_crawl_campaign_accounts_account")
	})

	t.Run("cross-org account rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 5003)
		foreign := seedCrawlAccount(t, ctx, tx, 5099)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_accounts (org_id, campaign_id, account_id) VALUES ($1, $2, $3)`,
			base.org, base.campaign, foreign)
		wantConstraint(t, err, "fk_fb_crawl_campaign_accounts_account")
	})

	t.Run("duplicate membership rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 5004)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_accounts (org_id, campaign_id, account_id) VALUES ($1, $2, $3)`,
			base.org, base.campaign, base.accountID)
		wantConstraint(t, err, "pk_fb_crawl_campaign_accounts")
	})
}

func TestFacebookCrawlSource_Integrity(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	t.Run("cross-org campaign rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 6001)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_sources (org_id, campaign_id, source_url, normalized_source_key)
			 VALUES (6099, $1, 'u', 'k')`, base.campaign)
		wantConstraint(t, err, "fk_fb_crawl_sources_campaign")
	})

	t.Run("duplicate normalized key rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 6002)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_sources (org_id, campaign_id, source_url, normalized_source_key)
			 VALUES ($1, $2, 'u', 'key-a')`, base.org, base.campaign)
		wantConstraint(t, err, "uq_fb_crawl_sources_org_campaign_key")
	})

	t.Run("same key in another campaign accepted", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 6003)
		var other int64
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO facebook_crawl_campaigns (org_id, name) VALUES ($1, 'c2') RETURNING id`,
			base.org).Scan(&other); err != nil {
			t.Fatalf("second campaign: %v", err)
		}
		seedSource(t, ctx, tx, crawlSeed{org: base.org, campaign: other}, "key-a")
	})

	t.Run("null preferred account accepted", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 6004)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_sources (org_id, campaign_id, source_url, normalized_source_key, preferred_account_id)
			 VALUES ($1, $2, 'u', 'key-b', NULL)`, base.org, base.campaign); err != nil {
			t.Fatalf("null preferred_account_id must be accepted: %v", err)
		}
	})

	t.Run("preferred account outside pool rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 6005)
		outside := seedCrawlAccount(t, ctx, tx, base.org)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_campaign_sources (org_id, campaign_id, source_url, normalized_source_key, preferred_account_id)
			 VALUES ($1, $2, 'u', 'key-c', $3)`, base.org, base.campaign, outside)
		wantConstraint(t, err, "fk_fb_crawl_sources_preferred_account")
	})

	t.Run("dropping pool account with live affinity rejected", func(t *testing.T) {
		tx := beginTx(t, ctx, s)
		base := seedCrawlBaseline(t, ctx, tx, 6006)
		if _, err := tx.ExecContext(ctx,
			`UPDATE facebook_crawl_campaign_sources SET preferred_account_id = $1 WHERE id = $2`,
			base.accountID, base.source); err != nil {
			t.Fatalf("set affinity: %v", err)
		}
		_, err := tx.ExecContext(ctx,
			`DELETE FROM facebook_crawl_campaign_accounts WHERE org_id = $1 AND campaign_id = $2 AND account_id = $3`,
			base.org, base.campaign, base.accountID)
		wantConstraint(t, err, "fk_fb_crawl_sources_preferred_account")
	})
}
