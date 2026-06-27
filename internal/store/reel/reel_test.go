// DB-backed regression for the reel store. Uses the shared storetest schema template
// (one migrate per binary) per STORE_SUBPACKAGE_REFACTOR.
package reel_test

import (
	"errors"
	"testing"

	"github.com/thg/scraper/internal/store"
	reelstore "github.com/thg/scraper/internal/store/reel"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrap(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newStore(t *testing.T, name string) *reelstore.Store {
	dst := storetest.CopyTemplate(t, bootstrap, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.Reel()
}

// TestSpendGateAndCostIdempotent proves the money invariant: StartRenderCAS commits once,
// a redelivered shot completion adds no cost, and the per-shot CAS is single-win.
func TestSpendGateAndCostIdempotent(t *testing.T) {
	s := newStore(t, "reel_spend.db")
	const org = int64(7)

	id, err := s.CreateReel(reelstore.Reel{OrgID: org, BriefStyle: "demo", Status: reelstore.StatusScriptReady})
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}

	// First start wins; second is a no-op (already rendering) — no double-charge.
	started, err := s.StartRenderCAS(org, id, "reel-key-1", 1800)
	if err != nil || !started {
		t.Fatalf("first StartRenderCAS: started=%v err=%v", started, err)
	}
	started2, err := s.StartRenderCAS(org, id, "reel-key-1", 1800)
	if err != nil {
		t.Fatalf("second StartRenderCAS err: %v", err)
	}
	if started2 {
		t.Fatal("second StartRenderCAS must NOT start again (idempotent spend gate)")
	}

	// One shot through the CAS lifecycle.
	if err := s.CreateShot(reelstore.Shot{OrgID: org, ReelID: id, Scene: 1, Kind: "broll"}); err != nil {
		t.Fatalf("CreateShot: %v", err)
	}
	claimed, err := s.ClaimShotForRender(org, id, 1, "fake", "job-1", 900)
	if err != nil || !claimed {
		t.Fatalf("ClaimShotForRender: claimed=%v err=%v", claimed, err)
	}

	// First webhook wins and we charge; redelivery does not.
	applied, err := s.MarkShotDone(org, id, "job-1", "out/1.mp4", 0.06)
	if err != nil || !applied {
		t.Fatalf("first MarkShotDone: applied=%v err=%v", applied, err)
	}
	if err := s.AddCost(org, id, 0.06); err != nil {
		t.Fatalf("AddCost: %v", err)
	}
	applied2, err := s.MarkShotDone(org, id, "job-1", "out/1.mp4", 0.06)
	if err != nil {
		t.Fatalf("second MarkShotDone err: %v", err)
	}
	if applied2 {
		t.Fatal("redelivered MarkShotDone must NOT re-apply (would double-charge)")
	}

	total, done, err := s.CountShots(org, id)
	if err != nil || total != 1 || done != 1 {
		t.Fatalf("CountShots: total=%d done=%d err=%v", total, done, err)
	}
	r, err := s.GetReel(org, id)
	if err != nil {
		t.Fatalf("GetReel: %v", err)
	}
	if r.TotalCostUSD != 0.06 {
		t.Fatalf("total_cost_usd want 0.06 got %v (charged more than once?)", r.TotalCostUSD)
	}
}

// TestTenantIsolation proves a reel is invisible to another org.
func TestTenantIsolation(t *testing.T) {
	s := newStore(t, "reel_tenant.db")
	id, err := s.CreateReel(reelstore.Reel{OrgID: 1, BriefStyle: "x"})
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	if _, err := s.GetReel(2, id); !errors.Is(err, reelstore.ErrReelNotFound) {
		t.Fatalf("cross-org GetReel must be ErrReelNotFound, got %v", err)
	}
	if _, err := s.GetReel(1, id); err != nil {
		t.Fatalf("same-org GetReel must succeed, got %v", err)
	}
}
