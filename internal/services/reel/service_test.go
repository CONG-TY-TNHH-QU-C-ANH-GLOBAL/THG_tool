// Reel service tests. The underlying store is Postgres-platform-only (see
// internal/store/reel), so these run against a REAL PostgreSQL database,
// gated on POSTGRES_PLATFORM_TEST_DSN (same convention as
// internal/store.TestRealPostgresApply), so `go test ./...` stays green
// without a database.
package reel_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/thg/scraper/internal/services/reel"
	"github.com/thg/scraper/internal/store"
	reelstore "github.com/thg/scraper/internal/store/reel"
)

// newTestService returns a service under test plus the underlying store
// handle, so tests can assert persisted state directly — the service
// deliberately has no Get/Load passthrough (see service.go's design note).
func newTestService(t *testing.T) (*reel.Service, *reelstore.Store) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping reel service Postgres tests")
	}
	s, err := store.New(dsn)
	if err != nil {
		t.Fatalf("store.New(postgres dsn): %v", err)
	}
	t.Cleanup(func() {
		// Best-effort teardown: a failure here would only leak test rows
		// into the next run (harmless — every test uses org IDs scoped to
		// its own test), never mask a real assertion.
		ctx := context.Background()
		_, _ = s.DB().ExecContext(ctx, `DELETE FROM reel_scripts`)
		_, _ = s.DB().ExecContext(ctx, `DELETE FROM reels`)
		_ = s.Close()
	})
	return reel.NewService(s.Reel(), reel.FakeRenderer{}), s.Reel()
}

func TestReelService_HappyPath_DraftToDone(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	const orgID, userID int64 = 5001, 1

	reelID, err := svc.CreateDraft(ctx, orgID, userID, "Summer promo", "30s product reel")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	assertReelStatus(t, ctx, store, orgID, reelID, reel.StatusDraft)

	script, err := svc.GenerateScript(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GenerateScript: %v", err)
	}
	if script.Version != 1 || script.Content == "" {
		t.Fatalf("GenerateScript = %+v, want version=1 with content", script)
	}
	assertReelStatus(t, ctx, store, orgID, reelID, reel.StatusScripting)

	if err := svc.ApproveLatestScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("ApproveLatestScript: %v", err)
	}
	assertReelStatus(t, ctx, store, orgID, reelID, reel.StatusApproved)

	if err := svc.RenderFake(ctx, orgID, reelID); err != nil {
		t.Fatalf("RenderFake: %v", err)
	}
	assertReelStatus(t, ctx, store, orgID, reelID, reel.StatusDone)
}

func assertReelStatus(t *testing.T, ctx context.Context, store *reelstore.Store, orgID, reelID int64, want string) {
	t.Helper()
	got, err := store.GetReel(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetReel: %v", err)
	}
	if got.Status != want {
		t.Fatalf("reel status = %q, want %q", got.Status, want)
	}
}

func TestReelService_ApproveWithNoScript_Fails(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const orgID, userID int64 = 5002, 1

	reelID, err := svc.CreateDraft(ctx, orgID, userID, "no script yet", "brief")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	if err := svc.ApproveLatestScript(ctx, orgID, reelID); !errors.Is(err, reel.ErrNoScript) {
		t.Fatalf("ApproveLatestScript = %v, want ErrNoScript", err)
	}
}

func TestReelService_RenderBeforeApproval_Fails(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const orgID, userID int64 = 5003, 1

	reelID, err := svc.CreateDraft(ctx, orgID, userID, "not approved", "brief")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := svc.GenerateScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("GenerateScript: %v", err)
	}

	if err := svc.RenderFake(ctx, orgID, reelID); !errors.Is(err, reel.ErrScriptNotApproved) {
		t.Fatalf("RenderFake = %v, want ErrScriptNotApproved", err)
	}
}
