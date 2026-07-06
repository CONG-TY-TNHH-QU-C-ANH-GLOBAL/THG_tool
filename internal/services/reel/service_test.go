// Reel service tests. The underlying store is Postgres-platform-only (see
// internal/store/reel), so these run against a REAL PostgreSQL database via
// reeltest.OpenStore, gated on POSTGRES_PLATFORM_TEST_DSN, so `go test
// ./...` stays green without a database.
package reel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/services/reel"
	reelstore "github.com/thg/scraper/internal/store/reel"
	"github.com/thg/scraper/internal/store/reel/reeltest"
)

// newTestService returns a service under test plus the underlying store
// handle, so tests can assert persisted state directly — the service
// deliberately has no Get/Load passthrough (see service.go's design note).
func newTestService(t *testing.T) (*reel.Service, *reelstore.Store) {
	t.Helper()
	s := reeltest.OpenStore(t)
	return reel.NewService(s.Reel(), reel.FakeRenderer{}), s.Reel()
}

// createDraft creates a reel and fails the test on error, collapsing the
// CreateDraft-then-check-error boilerplate every test needs before its own
// scenario starts.
func createDraft(t *testing.T, svc *reel.Service, orgID, userID int64, title, brief string) int64 {
	t.Helper()
	reelID, err := svc.CreateDraft(context.Background(), orgID, userID, title, brief)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	return reelID
}

// createApprovedDraft creates a reel, generates its script, and approves
// it — the "already approved" starting state a render test needs, with the
// three setup steps collapsed so the render call under test stays the
// visible point of interest.
func createApprovedDraft(t *testing.T, svc *reel.Service, orgID, userID int64, title, brief string) int64 {
	t.Helper()
	ctx := context.Background()
	reelID := createDraft(t, svc, orgID, userID, title, brief)
	if _, err := svc.GenerateScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("GenerateScript: %v", err)
	}
	if err := svc.ApproveLatestScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("ApproveLatestScript: %v", err)
	}
	return reelID
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

func TestReelService_HappyPath_DraftToDone(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	const orgID, userID int64 = 5001, 1

	reelID := createDraft(t, svc, orgID, userID, "Summer promo", "30s product reel")
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

func TestReelService_ApproveWithNoScript_Fails(t *testing.T) {
	svc, _ := newTestService(t)
	const orgID, userID int64 = 5002, 1

	reelID := createDraft(t, svc, orgID, userID, "no script yet", "brief")

	if err := svc.ApproveLatestScript(context.Background(), orgID, reelID); !errors.Is(err, reel.ErrNoScript) {
		t.Fatalf("ApproveLatestScript = %v, want ErrNoScript", err)
	}
}

func TestReelService_RenderBeforeApproval_Fails(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const orgID, userID int64 = 5003, 1

	reelID := createDraft(t, svc, orgID, userID, "not approved", "brief")
	if _, err := svc.GenerateScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("GenerateScript: %v", err)
	}

	if err := svc.RenderFake(ctx, orgID, reelID); !errors.Is(err, reel.ErrScriptNotApproved) {
		t.Fatalf("RenderFake = %v, want ErrScriptNotApproved", err)
	}
}
