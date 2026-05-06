package leadingest

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/scoring"
)

// IngestPost without AppStore/LegacyDB exercises only the classify path so we
// can assert routing decisions (filter / cold / rejected / gate / qualified)
// without dragging in a real SQLite store.

func TestIngestPost_BlankContentSkipped(t *testing.T) {
	out, err := IngestPost(context.Background(), Deps{}, Input{Content: "   "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Skipped != "filter" {
		t.Fatalf("expected skipped=filter, got %q", out.Skipped)
	}
	if out.Inserted {
		t.Fatalf("expected not inserted")
	}
}

func TestIngestPost_DeterministicHotLeadQualifies(t *testing.T) {
	// Strong buyer-demand phrasing in Vietnamese should cross the deterministic
	// hot threshold even without AI classification.
	deps := Deps{
		Scorer: scoring.New(scoring.DefaultConfig()),
		Guidance: scoring.Guidance{
			TargetAuthorRole: "customers",
			TargetSignals:    []string{"ship đi mỹ", "supplier"},
		},
		Keywords: []string{"pod", "dropship", "supplier"},
	}
	in := Input{
		TaskID:    "task-1",
		OrgID:     1,
		Content:   "Mình cần tìm supplier POD/dropship, cần báo giá ship đi Mỹ. Ai biết bên nào uy tín không? Cần tư vấn gấp.",
		Reactions: 12,
		Comments:  5,
	}
	out, err := IngestPost(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Skipped != "" {
		t.Fatalf("expected qualified, got skipped=%q signals=%v", out.Skipped, out.Signals)
	}
	if out.Category != "hot" && out.Category != "warm" {
		t.Fatalf("expected hot or warm, got %q score=%.2f", out.Category, out.Score)
	}
	// Inserted is false because AppStore/LegacyDB are nil; the function still
	// reports the classification so callers can persist or report metrics.
}

func TestIngestPost_ColdLeadIsSkippedNotInserted(t *testing.T) {
	deps := Deps{
		Scorer:   scoring.New(scoring.DefaultConfig()),
		Keywords: []string{"unrelated"},
	}
	in := Input{
		Content: "Just a friendly hello to everyone, hope you have a wonderful day today.",
	}
	out, err := IngestPost(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Skipped != "cold" {
		t.Fatalf("expected skipped=cold, got %q (category=%q)", out.Skipped, out.Category)
	}
	if out.Inserted {
		t.Fatalf("cold leads must not be inserted")
	}
}

func TestIngestPost_RejectRuleHardRejects(t *testing.T) {
	deps := Deps{
		Scorer: scoring.New(scoring.DefaultConfig()),
		SignalGate: SignalGate{
			RejectRules: []string{"recruiting affiliates"},
		},
		Keywords: []string{"pod"},
	}
	in := Input{
		// Otherwise a strong buyer-intent post; the gate phrase still wins.
		Content: "Looking for a supplier urgently — recruiting affiliates next week, need quotes.",
	}
	out, err := IngestPost(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Skipped != "gate_negative" {
		t.Fatalf("expected skipped=gate_negative, got %q", out.Skipped)
	}
	if out.Category != "rejected" {
		t.Fatalf("expected category=rejected, got %q", out.Category)
	}
	matched := false
	for _, sig := range out.Signals {
		if strings.HasPrefix(sig, "gate_reject:") {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("expected gate_reject signal, got %v", out.Signals)
	}
}

func TestSignalGateFromMap_TolerantOfMissingFields(t *testing.T) {
	gate := SignalGateFromMap(map[string]any{
		"target_role":      "customers",
		"positive_signals": []any{"need supplier", "looking for"},
		"reject_rules":     []any{"spam links"},
		"min_confidence":   0.7,
	})
	if gate.TargetRole != "customers" {
		t.Fatalf("target_role: %q", gate.TargetRole)
	}
	if len(gate.PositiveSignals) != 2 {
		t.Fatalf("positive_signals: %v", gate.PositiveSignals)
	}
	if gate.MinConfidence != 0.7 {
		t.Fatalf("min_confidence: %v", gate.MinConfidence)
	}
	if len(gate.RejectRules) != 1 || gate.RejectRules[0] != "spam links" {
		t.Fatalf("reject_rules: %v", gate.RejectRules)
	}
	// Missing field tolerated.
	empty := SignalGateFromMap(nil)
	if empty.TargetRole != "" || len(empty.PositiveSignals) != 0 {
		t.Fatalf("expected zero gate, got %+v", empty)
	}
}
