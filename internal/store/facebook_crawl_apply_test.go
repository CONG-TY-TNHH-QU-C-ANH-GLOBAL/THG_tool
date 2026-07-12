// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"os"
	"testing"
)

// TestFacebookCrawlSchema_Applied proves the PR-M2B migrations (0112-0114)
// apply through store.New's normal boot path and that a second boot on the same
// database is a stable no-op (run-once layer 1). Tables and a representative
// constraint per migration are asserted present via the catalog.
func TestFacebookCrawlSchema_Applied(t *testing.T) {
	s := bootPlatform(t)
	ctx := context.Background()

	applied, err := s.appliedMigrationVersions(ctx, s.db)
	if err != nil {
		t.Fatalf("appliedMigrationVersions: %v", err)
	}
	for v := 112; v <= 114; v++ {
		if _, ok := applied[v]; !ok {
			t.Errorf("crawl migration %04d missing from schema_migrations", v)
		}
	}

	for _, table := range []string{
		"facebook_crawl_campaigns",
		"facebook_crawl_campaign_accounts",
		"facebook_crawl_campaign_sources",
		"facebook_crawl_runs",
		"facebook_crawl_lead_index",
	} {
		if !s.tableExists(ctx, table) {
			t.Errorf("table %q missing after boot", table)
		}
	}

	for _, name := range []string{
		"uq_accounts_org_id_id",
		"uq_fb_crawl_campaigns_org_id_id",
		"ux_fb_crawl_runs_one_active_account",
		"pk_fb_crawl_lead_index",
	} {
		if !constraintOrIndexExists(t, ctx, s, name) {
			t.Errorf("expected constraint/index %q to exist after boot", name)
		}
	}

	second, err := New(os.Getenv("POSTGRES_PLATFORM_TEST_DSN"))
	if err != nil {
		t.Fatalf("second boot must be idempotent: %v", err)
	}
	_ = second.Close()
}

func constraintOrIndexExists(t *testing.T, ctx context.Context, s *Store, name string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM (
			SELECT conname AS name FROM pg_constraint
			UNION ALL
			SELECT indexname FROM pg_indexes
		) c WHERE c.name = $1`, name).Scan(&n); err != nil {
		t.Fatalf("catalog lookup for %q: %v", name, err)
	}
	return n > 0
}
