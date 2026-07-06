// Cross-org isolation tests for the reel service (mirrors
// internal/store/reel/reel_org_scope_test.go's split). newTestService is
// shared from service_test.go.
package reel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/services/reel"
)

func TestReelService_CrossOrgWorkflow_DoesNotMutateOtherOrg(t *testing.T) {
	const orgA, orgB, userID int64 = 6001, 6002, 1
	svc, store := newTestService(t, orgA, orgB)
	ctx := context.Background()

	reelA := createDraft(t, svc, orgA, userID, "org A reel", "brief")
	if _, err := svc.GenerateScript(ctx, orgA, reelA); err != nil {
		t.Fatalf("GenerateScript(orgA): %v", err)
	}

	// orgB attempting every workflow step against orgA's reel_id must fail
	// with the same "no script"/"not found" errors a genuinely missing
	// reel would produce — never touch orgA's row.
	if err := svc.ApproveLatestScript(ctx, orgB, reelA); !errors.Is(err, reel.ErrNoScript) {
		t.Fatalf("ApproveLatestScript(orgB, orgA's reel) = %v, want ErrNoScript", err)
	}
	if err := svc.RenderFake(ctx, orgB, reelA); !errors.Is(err, reel.ErrNoScript) {
		t.Fatalf("RenderFake(orgB, orgA's reel) = %v, want ErrNoScript", err)
	}
	if _, err := svc.GenerateScript(ctx, orgB, reelA); !errors.Is(err, reel.ErrReelNotFound) {
		t.Fatalf("GenerateScript(orgB, orgA's reel) = %v, want ErrReelNotFound", err)
	}

	// orgA's reel is still exactly where GenerateScript left it: scripting,
	// one unapproved v1 script — none of orgB's attempts mutated it.
	assertReelStatus(t, ctx, store, orgA, reelA, reel.StatusScripting)
	script, err := store.GetLatestScript(ctx, orgA, reelA)
	if err != nil {
		t.Fatalf("GetLatestScript(orgA): %v", err)
	}
	if script.Approved {
		t.Fatalf("orgA script got approved by a cross-org call")
	}
}
