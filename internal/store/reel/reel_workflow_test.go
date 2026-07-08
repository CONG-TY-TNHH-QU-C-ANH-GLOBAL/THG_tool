// Atomic reel-workflow store tests (PR-R2.5). These exercise the
// transaction-boundary methods in workflow.go against a REAL PostgreSQL
// database, gated on POSTGRES_PLATFORM_TEST_DSN — same convention as
// reel_test.go, so `go test ./...` stays green without a database.
package reel_test

import (
	"context"
	"database/sql"
	"errors"
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

	// Cross-org call matches zero rows (the reel_id/script_id belong to
	// another org): the RowsAffected guard rejects with sql.ErrNoRows and
	// nothing is mutated.
	if err := s.Reel().ApproveScriptAndSetReelStatus(ctx, otherOrg, reelID, scriptID, "done"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-org ApproveScriptAndSetReelStatus err = %v, want sql.ErrNoRows", err)
	}
	got, err = s.Reel().GetReel(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetReel after cross-org call: %v", err)
	}
	if got.Status != "approved" {
		t.Fatalf("cross-org call mutated owner status to %q, want approved", got.Status)
	}
}

// TestReel_ApproveScriptAndSetReelStatus_ScriptReelMismatch pins the
// data-integrity fix: approving a scriptID that belongs to a DIFFERENT reel
// (same org) must reject and touch nothing — the approval is scoped by
// reel_id, so a mismatch is 0 rows (sql.ErrNoRows) and the target reel's
// status is never advanced off the wrong script.
func TestReel_ApproveScriptAndSetReelStatus_ScriptReelMismatch(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgID int64 = 3301
	reeltest.CleanupOrgs(t, s, orgID)
	ctx := context.Background()

	reelA, err := s.Reel().CreateReel(ctx, orgID, "reel A", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel A: %v", err)
	}
	scriptA, err := s.Reel().CreateScript(ctx, orgID, reelA, 1, `{"dialogue":"A"}`)
	if err != nil {
		t.Fatalf("CreateScript A: %v", err)
	}
	reelB, err := s.Reel().CreateReel(ctx, orgID, "reel B", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel B: %v", err)
	}

	// script A does not belong to reel B → reject, nothing changes.
	err = s.Reel().ApproveScriptAndSetReelStatus(ctx, orgID, reelB, scriptA, "approved")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("mismatched script/reel: err = %v, want sql.ErrNoRows", err)
	}

	latestA, err := s.Reel().GetLatestScript(ctx, orgID, reelA)
	if err != nil {
		t.Fatalf("GetLatestScript A: %v", err)
	}
	if latestA.Approved {
		t.Fatalf("script A approved via wrong reel — must stay unapproved")
	}
	gotB, err := s.Reel().GetReel(ctx, orgID, reelB)
	if err != nil {
		t.Fatalf("GetReel B: %v", err)
	}
	if gotB.Status != "draft" {
		t.Fatalf("reel B status = %q after rejected mismatch, want draft (unchanged)", gotB.Status)
	}
}
