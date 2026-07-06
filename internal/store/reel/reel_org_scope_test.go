// Tenant-isolation regression tests for the reel subpackage (mirrors
// internal/store/crawl/crawl_org_scope_test.go's split: CRUD behavior in
// reel_test.go, cross-org isolation proofs here).
package reel_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/thg/scraper/internal/store/reel/reeltest"
)

func TestReel_NoCrossOrgReads(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgA, orgB int64 = 2001, 2002
	reeltest.CleanupOrgs(t, s, orgA, orgB)
	ctx := context.Background()

	idB, err := s.Reel().CreateReel(ctx, orgB, "org B reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel(orgB): %v", err)
	}

	if _, err := s.Reel().GetReel(ctx, orgA, idB); err != sql.ErrNoRows {
		t.Fatalf("GetReel(orgA, orgB's id) = %v, want sql.ErrNoRows", err)
	}

	if err := s.Reel().UpdateReelStatus(ctx, orgA, idB, "approved"); err != nil {
		t.Fatalf("UpdateReelStatus(orgA, orgB's id) returned error: %v", err)
	}
	got, err := s.Reel().GetReel(ctx, orgB, idB)
	if err != nil {
		t.Fatalf("GetReel(orgB) after cross-org update attempt: %v", err)
	}
	if got.Status != "draft" {
		t.Fatalf("cross-org UpdateReelStatus mutated orgB's reel: status = %q", got.Status)
	}

	listA, err := s.Reel().ListReels(ctx, orgA)
	if err != nil {
		t.Fatalf("ListReels(orgA): %v", err)
	}
	if len(listA) != 0 {
		t.Fatalf("ListReels(orgA) leaked orgB's reel: %+v", listA)
	}
}

// TestReelScript_CrossOrgAssociationImpossible proves the composite FK
// (org_id, reel_id) -> reels(org_id, id) makes a forged cross-org script
// association a schema-level constraint violation, not just an
// application-level convention.
func TestReelScript_CrossOrgAssociationImpossible(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgA, orgB int64 = 4001, 4002
	reeltest.CleanupOrgs(t, s, orgA, orgB)
	ctx := context.Background()

	reelA, err := s.Reel().CreateReel(ctx, orgA, "org A reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel(orgA): %v", err)
	}
	scriptA, err := s.Reel().CreateScript(ctx, orgA, reelA, 1, `{"dialogue":"a"}`)
	if err != nil {
		t.Fatalf("CreateScript(orgA): %v", err)
	}

	// orgB must not be able to attach a script to orgA's reel, even by
	// passing its own org_id — the (org_id, reel_id) pair doesn't exist in
	// reels, so the composite FK rejects the insert outright.
	if _, err := s.Reel().CreateScript(ctx, orgB, reelA, 1, `{"dialogue":"forged"}`); err == nil {
		t.Fatalf("CreateScript(orgB, orgA's reel_id) succeeded, want a foreign-key constraint error")
	}

	// Nothing was inserted: orgA's script list is unchanged, and orgB sees
	// no association with reelA at all.
	listA, err := s.Reel().ListScripts(ctx, orgA, reelA)
	if err != nil {
		t.Fatalf("ListScripts(orgA): %v", err)
	}
	if len(listA) != 1 {
		t.Fatalf("ListScripts(orgA) = %d scripts, want 1 (forged insert must not have landed)", len(listA))
	}

	if _, err := s.Reel().GetLatestScript(ctx, orgB, reelA); err != sql.ErrNoRows {
		t.Fatalf("GetLatestScript(orgB, orgA's reel_id) = %v, want sql.ErrNoRows", err)
	}
	if listB, err := s.Reel().ListScripts(ctx, orgB, reelA); err != nil || len(listB) != 0 {
		t.Fatalf("ListScripts(orgB, orgA's reel_id) = (%+v, %v), want (empty, nil)", listB, err)
	}

	// orgB approving orgA's real script id is a no-op — orgA's script stays
	// unapproved.
	if err := s.Reel().ApproveScript(ctx, orgB, scriptA); err != nil {
		t.Fatalf("cross-org ApproveScript returned error: %v", err)
	}
	got, err := s.Reel().GetLatestScript(ctx, orgA, reelA)
	if err != nil {
		t.Fatalf("GetLatestScript(orgA) after cross-org approve attempt: %v", err)
	}
	if got.Approved {
		t.Fatalf("cross-org ApproveScript approved orgA's script")
	}
}
