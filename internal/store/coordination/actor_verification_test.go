package coordination_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

func TestClassifyActorVerdict(t *testing.T) {
	cases := []struct {
		name, expected, actual, want string
	}{
		{"both equal -> verified", "111", "111", models.ActorVerdictVerified},
		{"both differ -> mismatch", "111", "222", models.ActorVerdictMismatch},
		{"expected missing -> unknown", "", "222", models.ActorVerdictUnknown},
		{"actual missing -> unknown", "111", "", models.ActorVerdictUnknown},
		{"both missing -> unknown", "", "", models.ActorVerdictUnknown},
		{"whitespace trimmed equal -> verified", " 111 ", "111", models.ActorVerdictVerified},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := coordination.ClassifyActorVerdict(c.expected, c.actual); got != c.want {
				t.Fatalf("ClassifyActorVerdict(%q,%q) = %q, want %q", c.expected, c.actual, got, c.want)
			}
		})
	}
}

// TestActorBlockGate is the integrity-gate contract: a mismatch must DENY the
// next claim for that account (not just label it), and only an operator clear
// re-enables it. This is the behaviour P1b exists to guarantee.
func TestActorBlockGate(t *testing.T) {
	ctx := context.Background()
	db, coord := newCoordinationStore(t, "actor_block_gate")
	const orgID, accountID = int64(7), int64(42)

	// Baseline: a fresh account is allowed to execute.
	if dec := capsCheck(t, db, coord, accountID); !dec.Allowed {
		t.Fatalf("fresh account should be allowed, got reason=%q", dec.Reason)
	}

	// Record a mismatch verdict with a block.
	if err := coord.RecordAccountActorVerdict(ctx, orgID, accountID,
		models.ActorVerdictMismatch, "222", "actor mismatch: expected 111, observed 222", true); err != nil {
		t.Fatalf("RecordAccountActorVerdict(block): %v", err)
	}

	// Gate must now DENY with the typed reason.
	if dec := capsCheck(t, db, coord, accountID); dec.Allowed || dec.Reason != "actor_mismatch_blocked" {
		t.Fatalf("blocked account: got Allowed=%v reason=%q, want denied/actor_mismatch_blocked", dec.Allowed, dec.Reason)
	}

	// State projection surfaces the block.
	states, err := coord.AccountActorStatesForOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("AccountActorStatesForOrg: %v", err)
	}
	if st := states[accountID]; !st.Blocked || st.Verdict != models.ActorVerdictMismatch {
		t.Fatalf("state projection: got %+v, want Blocked=true verdict=mismatch", st)
	}

	// A later VERIFIED verdict must NOT auto-clear the block (operator-only).
	if err := coord.RecordAccountActorVerdict(ctx, orgID, accountID,
		models.ActorVerdictVerified, "111", "", false); err != nil {
		t.Fatalf("RecordAccountActorVerdict(verified): %v", err)
	}
	if dec := capsCheck(t, db, coord, accountID); dec.Allowed {
		t.Fatalf("a verified verdict must not auto-unblock; account should still be denied")
	}

	// Operator clears the block → account is allowed again.
	if err := coord.ClearActorBlock(ctx, orgID, accountID); err != nil {
		t.Fatalf("ClearActorBlock: %v", err)
	}
	if dec := capsCheck(t, db, coord, accountID); !dec.Allowed {
		t.Fatalf("after clear, account should be allowed, got reason=%q", dec.Reason)
	}
}

// capsCheck runs CheckCapsTx in a throwaway transaction.
func capsCheck(t *testing.T, db *store.Store, coord *coordination.Store, accountID int64) coordination.CapsDecision {
	t.Helper()
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	dec, err := coord.CheckCapsTx(tx, accountID, "comment")
	if err != nil {
		t.Fatalf("CheckCapsTx: %v", err)
	}
	return dec
}
