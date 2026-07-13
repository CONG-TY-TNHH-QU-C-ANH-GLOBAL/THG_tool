// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"strings"
	"testing"
)

// TestFacebookCrawlClaimIndex pins the PR-M3B claim-queue read index created by
// migration 0118: it exists on facebook_crawl_runs, keys (org_id, queued_at,
// id), and is partial on status = 'queued'. Real Postgres catalog assertion (no
// unstable query-plan dependence).
func TestFacebookCrawlClaimIndex(t *testing.T) {
	s := bootPlatform(t)
	var indexdef string
	err := s.db.QueryRowContext(context.Background(),
		`SELECT indexdef FROM pg_indexes
		 WHERE schemaname = 'public'
		   AND tablename = 'facebook_crawl_runs'
		   AND indexname = 'ix_fb_crawl_runs_claim_queue'`).Scan(&indexdef)
	if err != nil {
		t.Fatalf("claim-queue index not found in pg_indexes: %v", err)
	}
	for _, want := range []string{"(org_id, queued_at, id)", "status = 'queued'"} {
		if !strings.Contains(indexdef, want) {
			t.Errorf("indexdef %q missing %q", indexdef, want)
		}
	}
}
