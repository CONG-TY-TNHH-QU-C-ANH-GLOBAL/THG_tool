// Atomic reel-workflow store tests (PR-R2.5). These exercise the
// transaction-boundary methods in workflow.go against a REAL PostgreSQL
// database, gated on POSTGRES_PLATFORM_TEST_DSN — same convention as
// reel_test.go, so `go test ./...` stays green without a database.
package reel_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/store/reel/reeltest"
)

// TestReel_CreateScriptAndSetReelStatus_Atomic pins the transaction boundary:
// the script insert and the reel status update commit together, and when the
// insert fails (duplicate version — the version-race loser) the paired status
// update rolls back too, so no partial "status advanced, script missing"
// state can leak.
func TestReel_CreateScriptAndSetReelStatus_Atomic(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgID int64 = 3101
	reeltest.CleanupOrgs(t, s, orgID)
	ctx := context.Background()

	reelID, err := s.Reel().CreateReel(ctx, orgID, "reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}

	// Both writes land together.
	if _, err := s.Reel().CreateScriptAndSetReelStatus(ctx, orgID, reelID, 1, `{"dialogue":"v1"}`, "scripting"); err != nil {
		t.Fatalf("CreateScriptAndSetReelStatus: %v", err)
	}
	got, err := s.Reel().GetReel(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetReel: %v", err)
	}
	if got.Status != "scripting" {
		t.Fatalf("status = %q, want scripting", got.Status)
	}

	// Duplicate version → INSERT fails; the paired status update ("approved")
	// must NOT commit, so the reel stays at "scripting" and no second script
	// row lands.
	if _, err := s.Reel().CreateScriptAndSetReelStatus(ctx, orgID, reelID, 1, `{"dialogue":"dup"}`, "approved"); err == nil {
		t.Fatalf("CreateScriptAndSetReelStatus on duplicate version: want error, got nil")
	}
	got, err = s.Reel().GetReel(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetReel after failed call: %v", err)
	}
	if got.Status != "scripting" {
		t.Fatalf("status = %q after rolled-back call, want scripting (status update must not leak)", got.Status)
	}
	list, err := s.Reel().ListScripts(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("ListScripts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListScripts len = %d, want 1 (failed insert must not persist)", len(list))
	}
}

// TestReel_ApproveScriptAndSetReelStatus_Atomic pins that approve (which gates
// rendering) and the reel status advance commit together, and that a cross-org
// call touches nothing.
func TestReel_ApproveScriptAndSetReelStatus_Atomic(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgID, otherOrg int64 = 3201, 3202
	reeltest.CleanupOrgs(t, s, orgID, otherOrg)
	ctx := context.Background()

	reelID, err := s.Reel().CreateReel(ctx, orgID, "reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	scriptID, err := s.Reel().CreateScript(ctx, orgID, reelID, 1, `{"dialogue":"v1"}`)
	if err != nil {
		t.Fatalf("CreateScript: %v", err)
	}

	if err := s.Reel().ApproveScriptAndSetReelStatus(ctx, orgID, reelID, scriptID, "approved"); err != nil {
		t.Fatalf("ApproveScriptAndSetReelStatus: %v", err)
	}
	got, err := s.Reel().GetReel(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetReel: %v", err)
	}
	latest, err := s.Reel().GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetLatestScript: %v", err)
	}
	if got.Status != "approved" || !latest.Approved {
		t.Fatalf("after approve: status=%q approved=%v, want approved/true", got.Status, latest.Approved)
	}

	// Cross-org call matches zero rows in both statements: no error, no change.
	if err := s.Reel().ApproveScriptAndSetReelStatus(ctx, otherOrg, reelID, scriptID, "done"); err != nil {
		t.Fatalf("cross-org ApproveScriptAndSetReelStatus returned error: %v", err)
	}
	got, err = s.Reel().GetReel(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetReel after cross-org call: %v", err)
	}
	if got.Status != "approved" {
		t.Fatalf("cross-org call mutated owner status to %q, want approved", got.Status)
	}
}
