package leadingest

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/scoring"
)

// Characterization harness for IngestPost — deterministic Outcome-level behavior with
// nil AppStore/LegacyDB (no DB, no AI). Pins the classify/route/gate/signal contract in
// specs/domains/facebook-sales-intelligence/features/lead-ingestion/technical.md §6–§10 BEFORE the PR23C complexity reduction. Asserts
// CURRENT behavior. Not duplicated here (covered elsewhere): blank filter, hot/warm
// qualify, cold skip, RejectRules gate, ValidateRouting, repairPrimaryURL, ForceLead
// gate-reject/cold overrides.

// containsSignalPrefix reports whether any signal in sigs has the given prefix.
func containsSignalPrefix(t *testing.T, sigs []string, prefix string) bool {
	t.Helper()
	for _, s := range sigs {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// §10: a routing failure inside IngestPost maps to Outcome{Skipped:"invalid_routing"} with
// an "invalid_routing:<msg>" signal + the url telemetry signal, and never inserts (distinct
// from TestValidateRouting, which exercises the validator directly, not the Outcome mapping).
func TestIngestPost_InvalidRoutingMapsToOutcome(t *testing.T) {
	deps := Deps{Scorer: scoring.New(scoring.DefaultConfig())}
	in := Input{
		SourceType:    "post",
		PrimaryURL:    "https://facebook.com/x?comment_id=99", // comment-only link → invalid
		Content:       "Mình cần tìm supplier, cần báo giá gấp.",
		URLRepairPath: "anchor_clean",
	}
	out, err := IngestPost(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("invalid routing must not be a Go error, got %v", err)
	}
	if out.Skipped != "invalid_routing" {
		t.Fatalf("expected skipped=invalid_routing, got %q", out.Skipped)
	}
	if out.Inserted {
		t.Fatalf("invalid routing must not insert")
	}
	if !containsSignalPrefix(t, out.Signals, "invalid_routing:") {
		t.Fatalf("expected an invalid_routing:<msg> signal, got %v", out.Signals)
	}
	if !slices.Contains(out.Signals, "url:anchor_clean") {
		t.Fatalf("expected url telemetry signal to ride through, got %v", out.Signals)
	}
}

// §6: Deps.ExtraSignals are appended to Outcome.Signals on the qualifying path (ingest.go:266).
func TestIngestPost_ExtraSignalsAppended(t *testing.T) {
	deps := Deps{
		Scorer: scoring.New(scoring.DefaultConfig()),
		Guidance: scoring.Guidance{
			TargetAuthorRole: "customers",
			TargetSignals:    []string{"ship đi mỹ", "supplier"},
		},
		Keywords:     []string{"pod", "dropship", "supplier"},
		ExtraSignals: []string{"chrome_extension_crawl"},
	}
	in := Input{
		TaskID:     "task-extra",
		OrgID:      1,
		SourceType: "post",
		PrimaryURL: "https://facebook.com/groups/1/posts/201",
		Content:    "Mình cần tìm supplier POD/dropship, cần báo giá ship đi Mỹ. Ai biết bên nào uy tín không? Cần tư vấn gấp.",
		Reactions:  12,
		Comments:   5,
	}
	out, err := IngestPost(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(out.Signals, "chrome_extension_crawl") {
		t.Fatalf("expected ExtraSignals to be appended, got %v", out.Signals)
	}
}

// §7/§9: the url-repair telemetry signal surfaces through Outcome.Signals — crawler path
// passes through as "url:<path>"; an in-pipeline synth upgrades it to "url:repaired_in_pipeline".
func TestIngestPost_URLRepairSignalSurfaces(t *testing.T) {
	t.Run("crawler path passes through", func(t *testing.T) {
		deps := Deps{Scorer: scoring.New(scoring.DefaultConfig())}
		in := Input{
			SourceType:    "post",
			PrimaryURL:    "https://facebook.com/groups/1/posts/202", // already valid
			Content:       "just saying hello to everyone today",
			URLRepairPath: "anchor_clean",
		}
		out, _ := IngestPost(context.Background(), deps, in)
		if !slices.Contains(out.Signals, "url:anchor_clean") {
			t.Fatalf("expected url:anchor_clean passthrough, got %v", out.Signals)
		}
	})

	t.Run("in-pipeline synth upgrades the signal", func(t *testing.T) {
		deps := Deps{Scorer: scoring.New(scoring.DefaultConfig())}
		in := Input{
			SourceType:    "post",
			PrimaryURL:    "https://www.facebook.com/groups/123", // group shell → repaired
			PostFBID:      "456",
			GroupFBID:     "123",
			Content:       "just saying hello to everyone today",
			URLRepairPath: "anchor_clean", // superseded by the in-pipeline synth
		}
		out, _ := IngestPost(context.Background(), deps, in)
		if !slices.Contains(out.Signals, "url:repaired_in_pipeline") {
			t.Fatalf("expected url:repaired_in_pipeline after synth, got %v", out.Signals)
		}
		if slices.Contains(out.Signals, "url:anchor_clean") {
			t.Fatalf("in-pipeline synth must supersede the crawler path, got %v", out.Signals)
		}
	})
}

// §7: buildURLRepairSignal precedence — pipelineRepaired wins over any crawler path.
func TestBuildURLRepairSignal(t *testing.T) {
	cases := []struct {
		name, crawlerPath string
		repaired          bool
		want              string
	}{
		{"empty + not repaired -> none", "", false, ""},
		{"crawler path passthrough", "anchor_clean", false, "url:anchor_clean"},
		{"repaired wins over path", "anchor_clean", true, "url:repaired_in_pipeline"},
		{"repaired with empty path", "", true, "url:repaired_in_pipeline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildURLRepairSignal(tc.crawlerPath, tc.repaired); got != tc.want {
				t.Fatalf("buildURLRepairSignal(%q,%v)=%q want %q", tc.crawlerPath, tc.repaired, got, tc.want)
			}
		})
	}
}

// §6/§9: the NegativeSignals gate rejects with the "gate_negative:" prefix (distinct from
// RejectRules' "gate_reject:", though both set Skipped="gate_negative"); ForceLead downgrades
// that veto to a "warm" lead annotated with the filter verdict (existing tests cover
// RejectRules + cold). One fixture exercises both the normal reject and the override.
func TestIngestPost_NegativeGateRejectsAndForceLeadOverrides(t *testing.T) {
	mk := func(force bool) Outcome {
		deps := Deps{
			Scorer:     scoring.New(scoring.DefaultConfig()),
			SignalGate: SignalGate{NegativeSignals: []string{"chỉ bán sỉ"}},
			Keywords:   []string{"pod"},
			ForceLead:  force,
		}
		in := Input{
			SourceType: "post",
			PrimaryURL: "https://facebook.com/groups/1/posts/203",
			Content:    "Cần tìm supplier POD, nhưng shop mình chỉ bán sỉ thôi nhé.",
		}
		out, err := IngestPost(context.Background(), deps, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}

	normal := mk(false)
	if normal.Skipped != "gate_negative" || normal.Category != "rejected" || normal.Inserted {
		t.Fatalf("normal crawl must reject negative gate: skipped=%q category=%q inserted=%v", normal.Skipped, normal.Category, normal.Inserted)
	}
	if !containsSignalPrefix(t, normal.Signals, "gate_negative:") {
		t.Fatalf("expected gate_negative:<phrase> signal, got %v", normal.Signals)
	}
	if containsSignalPrefix(t, normal.Signals, "gate_reject:") {
		t.Fatalf("NegativeSignals must NOT emit a gate_reject: signal, got %v", normal.Signals)
	}

	forced := mk(true)
	if forced.Skipped != "" || !forced.Inserted {
		t.Fatalf("force-lead must create the lead: skipped=%q inserted=%v", forced.Skipped, forced.Inserted)
	}
	if forced.Category == "rejected" || forced.Category == "" {
		t.Fatalf("force-lead must carry a real category, got %q", forced.Category)
	}
	if !slices.Contains(forced.Signals, "filter_override_applied:true") ||
		!slices.Contains(forced.Signals, "explicit_user_requested:true") {
		t.Fatalf("force-lead must annotate override signals, got %v", forced.Signals)
	}
	if !containsSignalPrefix(t, forced.Signals, "market_filter_result:gate_negative:") {
		t.Fatalf("force-lead must record the negative-gate verdict, got %v", forced.Signals)
	}
}
