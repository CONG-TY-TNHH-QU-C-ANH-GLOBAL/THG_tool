package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// seedRetrievalWithTrace inserts one retrieval event with a full
// trace + budget so the replay-query path has real data to read.
func seedRetrievalWithTrace(t *testing.T, db *Store, orgID int64, retrievalID, query string, hits []retrieval.ScoredHit) {
	t.Helper()
	trace := retrieval.Trace{
		Query:                query,
		CandidatesConsidered: 12,
		Selected:             hits,
		Rejected: []retrieval.RejectedCandidate{
			{AssetID: 99, Title: "ignored", Reason: retrieval.RejectStateFilter},
		},
		TotalByReason: map[retrieval.RejectionReason]int{retrieval.RejectStateFilter: 3},
		SearcherImpl:  "test-v0",
	}
	budget := retrieval.AssemblyBudget{
		AssembledProducts: 2,
		AssembledPolicies: 1,
		AssembledCTAs:     1,
		EstimatedTokens:   140,
	}
	db.RecordKnowledgeRetrievalWithTrace(context.Background(), orgID, retrievalID, query, "comment_drafted", trace, budget)
}

// Happy-path: a retrieval + matching outcome compose into one
// replay-event row, latest-first.
func TestListKnowledgeReplayEvents_HappyPath(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	seedRetrievalWithTrace(t, db, 7, "ret_a", "cat tee", []retrieval.ScoredHit{
		{AssetID: 1, Title: "Cat Tee", Type: assets.AssetPODProduct, Score: 0.9,
			Breakdown: retrieval.ScoreBreakdown{TextMatch: 0.55, Pin: 0.25}},
	})
	db.RecordKnowledgeOutcome(ctx, 7, "ret_a", "sent")

	events, err := db.ListKnowledgeReplayEventsForOrg(ctx, 7, "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event; got %d", len(events))
	}
	ev := events[0]
	if ev.RetrievalID != "ret_a" {
		t.Errorf("retrieval_id mismatch: %q", ev.RetrievalID)
	}
	if ev.Outcome != "sent" {
		t.Errorf("outcome should be 'sent'; got %q", ev.Outcome)
	}
	if ev.GeneratedAction != "comment_drafted" {
		t.Errorf("generated_action: got %q", ev.GeneratedAction)
	}
	// Trace must round-trip — re-parse and check selected hit count.
	var trace retrieval.Trace
	if err := json.Unmarshal(ev.Trace, &trace); err != nil {
		t.Fatalf("trace unmarshal: %v", err)
	}
	if len(trace.Selected) != 1 {
		t.Errorf("trace selected count: got %d", len(trace.Selected))
	}
	if trace.Selected[0].Breakdown.Pin != 0.25 {
		t.Errorf("score breakdown round-trip failed: pin=%v", trace.Selected[0].Breakdown.Pin)
	}
}

// Cross-org isolation: org-1's events invisible to org-2 and vice
// versa.
func TestListKnowledgeReplayEvents_TenantScope(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	seedRetrievalWithTrace(t, db, 1, "ret_org1", "cat tee", nil)
	seedRetrievalWithTrace(t, db, 2, "ret_org2", "dog mug", nil)
	db.RecordKnowledgeOutcome(ctx, 1, "ret_org1", "sent")
	db.RecordKnowledgeOutcome(ctx, 2, "ret_org2", "rejected")

	e1, _ := db.ListKnowledgeReplayEventsForOrg(ctx, 1, "", 10)
	if len(e1) != 1 || e1[0].RetrievalID != "ret_org1" {
		t.Errorf("org-1 leaked or missed: %+v", e1)
	}
	if e1[0].Outcome != "sent" {
		t.Errorf("org-1 outcome leak: got %q", e1[0].Outcome)
	}

	e2, _ := db.ListKnowledgeReplayEventsForOrg(ctx, 2, "", 10)
	if len(e2) != 1 || e2[0].RetrievalID != "ret_org2" {
		t.Errorf("org-2 leaked or missed: %+v", e2)
	}
}

// GetKnowledgeReplayEvent against a foreign org's retrieval_id
// returns sql.ErrNoRows — observably identical to "not found".
func TestGetKnowledgeReplayEvent_ForeignOrgIsNotFound(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	seedRetrievalWithTrace(t, db, 1, "ret_owned", "q", nil)

	_, err := db.GetKnowledgeReplayEvent(ctx, 2, "ret_owned")
	if err != sql.ErrNoRows {
		t.Errorf("foreign-org GET should return sql.ErrNoRows; got %v", err)
	}
}

// Latest outcome wins when multiple outcomes share a retrieval_id.
// This is the queued→sent progression the runtime emits.
func TestGetKnowledgeReplayEvent_LatestOutcomeWins(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	seedRetrievalWithTrace(t, db, 7, "ret_x", "q", nil)
	db.RecordKnowledgeOutcome(ctx, 7, "ret_x", "queued")
	db.RecordKnowledgeOutcome(ctx, 7, "ret_x", "sent")

	ev, err := db.GetKnowledgeReplayEvent(ctx, 7, "ret_x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ev.Outcome != "sent" {
		t.Errorf("latest-outcome rule failed; got %q want sent", ev.Outcome)
	}
}

// Stats round-trip: state counts + 30d sums + top-retrieved.
func TestGetKnowledgeStatsForOrg(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	// 3 approved, 1 pending, 1 hidden.
	for i, st := range []assets.AssetState{
		assets.StateApproved,
		assets.StateApproved,
		assets.StateApproved,
		assets.StatePending,
		assets.StateHidden,
	} {
		a := newTestAsset(7, sid, "ext_"+itoa(i), "row "+itoa(i))
		a.State = st
		if _, err := db.UpsertKnowledgeAsset(ctx, a); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	stats, err := db.GetKnowledgeStatsForOrg(ctx, 7)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalAssets != 5 {
		t.Errorf("TotalAssets: got %d want 5", stats.TotalAssets)
	}
	if stats.ApprovedAssets != 3 {
		t.Errorf("ApprovedAssets: got %d want 3", stats.ApprovedAssets)
	}
	if stats.PendingAssets != 1 {
		t.Errorf("PendingAssets: got %d", stats.PendingAssets)
	}
	if stats.HiddenAssets != 1 {
		t.Errorf("HiddenAssets: got %d", stats.HiddenAssets)
	}
}
