// RenderFake's failure branch (StatusFailed + errors.Join) is unreachable
// under FakeRenderer, which never errors — this test-only VideoRenderer
// exercises it. Test-only fakes live in package reel_test, not the
// production package (internal/store/DOMAINS.md §3.6).
package reel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/services/reel"
)

var errFakeRenderBoom = errors.New("boom")

type failingRenderer struct{}

func (failingRenderer) Render(context.Context, reel.RenderRequest) error {
	return errFakeRenderBoom
}

func TestReelService_RenderFailure_MarksReelFailed(t *testing.T) {
	_, store := newTestService(t)
	svc := reel.NewService(store, failingRenderer{})
	ctx := context.Background()
	const orgID, userID int64 = 5004, 1

	reelID, err := svc.CreateDraft(ctx, orgID, userID, "will fail", "brief")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := svc.GenerateScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("GenerateScript: %v", err)
	}
	if err := svc.ApproveLatestScript(ctx, orgID, reelID); err != nil {
		t.Fatalf("ApproveLatestScript: %v", err)
	}

	if err := svc.RenderFake(ctx, orgID, reelID); !errors.Is(err, errFakeRenderBoom) {
		t.Fatalf("RenderFake = %v, want it to wrap errFakeRenderBoom", err)
	}
	assertReelStatus(t, ctx, store, orgID, reelID, reel.StatusFailed)
}
