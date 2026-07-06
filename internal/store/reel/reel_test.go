// Reel Studio PR-R1 store tests. reels/reel_scripts are Postgres-platform-
// only (no SQLite schema — see docs/architecture/decisions/ADR-reel-studio-platform-module.md),
// so these run against a REAL PostgreSQL database, gated on
// POSTGRES_PLATFORM_TEST_DSN (same convention as
// internal/store.TestRealPostgresApply), so `go test ./...` stays green
// without a database.
package reel_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/thg/scraper/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping reel store Postgres tests")
	}
	s, err := store.New(dsn)
	if err != nil {
		t.Fatalf("store.New(postgres dsn): %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = s.DB().ExecContext(ctx, `DELETE FROM reel_scripts`)
		_, _ = s.DB().ExecContext(ctx, `DELETE FROM reels`)
		_ = s.Close()
	})
	return s
}

func TestReel_CreateGetListUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const orgID, userID int64 = 1001, 7

	id, err := s.Reel().CreateReel(ctx, orgID, "Summer promo", "30s product reel", userID)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	if id <= 0 {
		t.Fatalf("CreateReel returned id=%d, want > 0", id)
	}

	got, err := s.Reel().GetReel(ctx, orgID, id)
	if err != nil {
		t.Fatalf("GetReel: %v", err)
	}
	if got.Title != "Summer promo" || got.Status != "draft" || got.CreatedBy != userID {
		t.Fatalf("GetReel mismatch: %+v", got)
	}

	if err := s.Reel().UpdateReelStatus(ctx, orgID, id, "scripting"); err != nil {
		t.Fatalf("UpdateReelStatus: %v", err)
	}
	got, err = s.Reel().GetReel(ctx, orgID, id)
	if err != nil {
		t.Fatalf("GetReel after update: %v", err)
	}
	if got.Status != "scripting" {
		t.Fatalf("status = %q, want scripting", got.Status)
	}

	list, err := s.Reel().ListReels(ctx, orgID)
	if err != nil {
		t.Fatalf("ListReels: %v", err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("ListReels = %+v, want 1 reel with id %d", list, id)
	}
}

func TestReel_NoCrossOrgReads(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const orgA, orgB int64 = 2001, 2002

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

func TestReelScript_CreateGetListApprove(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const orgID int64 = 3001

	reelID, err := s.Reel().CreateReel(ctx, orgID, "reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}

	v1, err := s.Reel().CreateScript(ctx, orgID, reelID, 1, `{"dialogue":"v1"}`)
	if err != nil {
		t.Fatalf("CreateScript v1: %v", err)
	}
	if _, err := s.Reel().CreateScript(ctx, orgID, reelID, 2, `{"dialogue":"v2"}`); err != nil {
		t.Fatalf("CreateScript v2: %v", err)
	}

	latest, err := s.Reel().GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("GetLatestScript: %v", err)
	}
	if latest.Version != 2 || latest.Approved {
		t.Fatalf("GetLatestScript = %+v, want version=2 approved=false", latest)
	}

	list, err := s.Reel().ListScripts(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("ListScripts: %v", err)
	}
	if len(list) != 2 || list[0].Version != 1 || list[1].Version != 2 {
		t.Fatalf("ListScripts = %+v, want versions [1,2] in order", list)
	}

	if err := s.Reel().ApproveScript(ctx, orgID, v1); err != nil {
		t.Fatalf("ApproveScript: %v", err)
	}
	list, _ = s.Reel().ListScripts(ctx, orgID, reelID)
	if !list[0].Approved {
		t.Fatalf("ApproveScript did not persist: %+v", list[0])
	}

	// Cross-org approve is a silent no-op.
	const otherOrg int64 = 3002
	if err := s.Reel().ApproveScript(ctx, otherOrg, v1); err != nil {
		t.Fatalf("cross-org ApproveScript returned error: %v", err)
	}
}
