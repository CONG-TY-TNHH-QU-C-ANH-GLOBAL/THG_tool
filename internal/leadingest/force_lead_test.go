package leadingest

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/scoring"
)

// hasSignal reports whether out.Signals contains an exact signal value.
func hasSignal(sigs []string, want string) bool {
	for _, s := range sigs {
		if s == want {
			return true
		}
	}
	return false
}

// A) Explicit direct-post intake (Deps.ForceLead) overrides a market-signal veto: the
// SAME gate-reject post that a normal crawl drops is created as a lead, annotated with
// the filter verdict. The user already chose this post — the filter must not veto it.
func TestIngestPost_ForceLeadBypassesGateVeto(t *testing.T) {
	mk := func(force bool) Outcome {
		deps := Deps{
			Scorer:     scoring.New(scoring.DefaultConfig()),
			SignalGate: SignalGate{RejectRules: []string{"recruiting affiliates"}},
			Keywords:   []string{"pod"},
			ForceLead:  force,
		}
		in := Input{
			SourceType: "post",
			PrimaryURL: "https://facebook.com/groups/ship.viet.my/permalink/4505595319766639/",
			Content:    "Looking for a supplier urgently — recruiting affiliates next week, need quotes.",
		}
		out, err := IngestPost(context.Background(), deps, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}

	// B) Normal crawl (no ForceLead): the gate veto stands — regression guard.
	normal := mk(false)
	if normal.Skipped != "gate_negative" || normal.Inserted {
		t.Fatalf("normal crawl must still reject: skipped=%q inserted=%v", normal.Skipped, normal.Inserted)
	}

	// A) ForceLead: the same post is created (Inserted, no skip) with the filter verdict
	// recorded for observability and a non-rejected category.
	forced := mk(true)
	if forced.Skipped != "" {
		t.Fatalf("force-lead must not skip, got skipped=%q", forced.Skipped)
	}
	if !forced.Inserted {
		t.Fatalf("force-lead must create the lead (Inserted=true)")
	}
	if forced.Category == "rejected" || forced.Category == "cold" || forced.Category == "" {
		t.Fatalf("force-lead must carry a real category, got %q", forced.Category)
	}
	if !hasSignal(forced.Signals, "filter_override_applied:true") ||
		!hasSignal(forced.Signals, "explicit_user_requested:true") {
		t.Fatalf("force-lead must annotate override signals, got %v", forced.Signals)
	}
	if !hasSignal(forced.Signals, "market_filter_result:gate_reject:recruiting affiliates") {
		t.Fatalf("force-lead must record the market_filter_result verdict, got %v", forced.Signals)
	}
}

// Explicit intake also overrides a COLD verdict (a low-signal post the user explicitly
// chose). The lead is created; a normal crawl still drops a cold post.
func TestIngestPost_ForceLeadBypassesCold(t *testing.T) {
	in := Input{
		SourceType: "post",
		PrimaryURL: "https://facebook.com/groups/ship.viet.my/permalink/4505595319766639/",
		Content:    "Just a friendly hello to everyone, hope you have a wonderful day today.",
	}
	base := Deps{Scorer: scoring.New(scoring.DefaultConfig()), Keywords: []string{"unrelated"}}

	normal, _ := IngestPost(context.Background(), base, in)
	if normal.Skipped != "cold" || normal.Inserted {
		t.Fatalf("normal crawl must drop a cold post: skipped=%q inserted=%v", normal.Skipped, normal.Inserted)
	}

	forceDeps := base
	forceDeps.ForceLead = true
	forced, err := IngestPost(context.Background(), forceDeps, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if forced.Skipped != "" || !forced.Inserted {
		t.Fatalf("force-lead must create a lead from a cold post: skipped=%q inserted=%v", forced.Skipped, forced.Inserted)
	}
	if !hasSignal(forced.Signals, "filter_override_applied:true") {
		t.Fatalf("force-lead cold override must be annotated, got %v", forced.Signals)
	}
}
