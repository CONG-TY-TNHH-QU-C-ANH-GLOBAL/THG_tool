package coordination_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/coordination"
)

// TestContributionLeaderboard pins the Attribution projection: verified-success
// interactions are credited to the IMMUTABLE created_by member, ranked by count;
// created_by=0 (unattributed) is excluded.
func TestContributionLeaderboard(t *testing.T) {
	_, db := newCoordinationStore(t, "attribution.db")
	ctx := context.Background()

	const org int64 = 1
	// member 10 = 3 verified comments; member 20 = 1 verified comment + 1 queued
	// (not counted); member 0 = system (excluded).
	seed := func(createdBy int64, target, outcome string) {
		if _, err := db.RecordActionLedger(ctx, coordination.ActionLedgerEntry{
			OrgID: org, ActionType: "comment", TargetURL: target, AccountID: 1,
			CreatedBy: createdBy, Outcome: outcome,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seed(10, "https://fb.com/p/1", "succeeded")
	seed(10, "https://fb.com/p/2", "succeeded")
	seed(10, "https://fb.com/p/3", "succeeded")
	seed(20, "https://fb.com/p/4", "succeeded")
	seed(20, "https://fb.com/p/5", "queued") // in-flight, not a contribution
	seed(0, "https://fb.com/p/6", "succeeded") // system/unattributed

	board, err := db.ContributionLeaderboard(ctx, org, time.Time{}, 10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(board) != 2 {
		t.Fatalf("expected 2 attributed members (0 excluded), got %d: %+v", len(board), board)
	}
	if board[0].UserID != 10 || board[0].Total != 3 {
		t.Fatalf("champion should be member 10 with 3, got %+v", board[0])
	}
	if board[1].UserID != 20 || board[1].Total != 1 {
		t.Fatalf("runner-up should be member 20 with 1, got %+v", board[1])
	}
	if board[0].ByType["comment"] != 3 {
		t.Fatalf("by_type breakdown wrong: %+v", board[0].ByType)
	}
}
