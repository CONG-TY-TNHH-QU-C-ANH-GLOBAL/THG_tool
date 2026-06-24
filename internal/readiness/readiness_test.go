package readiness

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapReadinessStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func TestEvaluateCrawlAccountReadiness_RejectsAndReasons(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapReadinessStore, "crawl_readiness")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(5)

	// 1. account_id=0 → account_not_selected (NO silent fallback to a ready account).
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", 0); r != ReasonAccountNotSelected {
		t.Fatalf("account_id=0 → want %q, got %q", ReasonAccountNotSelected, r)
	}

	// 2. Non-existent account → account_not_owned.
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", 9999); r != ReasonAccountNotOwned {
		t.Fatalf("nonexistent account → want %q, got %q", ReasonAccountNotOwned, r)
	}

	// Seed an owned account with no connector.
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "acc-a", Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	// 3. Owned account, no online connector → connector_offline.
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", accID); r != ReasonConnectorOffline {
		t.Fatalf("no connector → want %q, got %q", ReasonConnectorOffline, r)
	}

	// 4. Actor-mismatch-blocked account → actor_mismatch_blocked (takes precedence).
	if err := db.Coordination().RecordAccountActorVerdict(ctx, orgID, accID,
		models.ActorVerdictMismatch, "222", "mismatch", true); err != nil {
		t.Fatalf("RecordAccountActorVerdict: %v", err)
	}
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", accID); r != ReasonActorMismatchBlocked {
		t.Fatalf("blocked account → want %q, got %q", ReasonActorMismatchBlocked, r)
	}
}
