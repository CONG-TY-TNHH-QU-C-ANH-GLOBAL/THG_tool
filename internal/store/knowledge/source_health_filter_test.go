package knowledge_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// PR-7 review follow-up: the retrieval hot path must NEVER quote
// products/prices from stale / errored / disconnected catalog sources.
// ExcludeUnhealthySources drops their assets at the SQL layer; the
// management view (flag off) still sees everything so operators can fix
// the source.
func TestListAssets_ExcludesUnhealthySources(t *testing.T) {
	db := newKnowledgeStore(t, "source_health.db")
	ctx := context.Background()

	healthySrc := mustSetupSource(t, db, 1)
	staleSrc := mustSetupSource(t, db, 1)
	errorSrc := mustSetupSource(t, db, 1)

	now := time.Now()
	if err := db.UpdateSourceHealth(ctx, staleSrc, 1, sources.Health{Status: sources.HealthStale, LastSyncAt: &now}, 1); err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if err := db.UpdateSourceHealth(ctx, errorSrc, 1, sources.Health{Status: sources.HealthError, Message: "disconnected"}, 1); err != nil {
		t.Fatalf("mark error: %v", err)
	}

	seed := func(src int64, ext, title string) {
		a := newTestAsset(1, src, ext, title)
		a.State = assets.StateApproved
		if _, err := db.UpsertAsset(ctx, a); err != nil {
			t.Fatalf("seed %s: %v", title, err)
		}
	}
	seed(healthySrc, "h1", "Healthy Product")
	seed(staleSrc, "s1", "Stale Product")
	seed(errorSrc, "e1", "Broken Product")

	// Retrieval hot path: only the healthy source's asset survives.
	hot, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{
		States:                  []assets.AssetState{assets.StateApproved},
		ExcludeUnhealthySources: true,
	})
	if err != nil {
		t.Fatalf("hot-path list: %v", err)
	}
	if len(hot) != 1 || hot[0].Title != "Healthy Product" {
		got := make([]string, 0, len(hot))
		for _, a := range hot {
			got = append(got, a.Title)
		}
		t.Fatalf("hot path must exclude unhealthy sources, got %v", got)
	}

	// Management view (flag off): all three remain visible/fixable.
	all, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{
		States: []assets.AssetState{assets.StateApproved},
	})
	if err != nil || len(all) != 3 {
		t.Fatalf("management view must keep all assets, got %d (%v)", len(all), err)
	}

	// Recovery: source healed → its assets are quotable again.
	if err := db.UpdateSourceHealth(ctx, staleSrc, 1, sources.Health{Status: sources.HealthHealthy, LastSyncAt: &now}, 1); err != nil {
		t.Fatalf("heal: %v", err)
	}
	hot, _ = db.ListAssetsForOrg(ctx, 1, assets.ListFilter{
		States:                  []assets.AssetState{assets.StateApproved},
		ExcludeUnhealthySources: true,
	})
	if len(hot) != 2 {
		t.Fatalf("healed source must be quotable again, got %d", len(hot))
	}
}
