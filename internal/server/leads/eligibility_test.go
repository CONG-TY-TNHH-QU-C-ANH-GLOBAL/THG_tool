package leads

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestBuildCommentEligibility(t *testing.T) {
	policy := models.DefaultCoveragePolicy() // MaxAccountsPerLead = 2
	now := time.Unix(1_700_000_000, 0).UTC()
	empty := &models.LeadEngagementState{}

	// 1. No comments + NO ready account → not eligible, no_ready_account.
	noReady := buildCommentEligibility(
		models.LeadCoverageState{}, workspaceActorReadiness{candidate: 1, ready: 0}, policy, empty, now)
	if noReady.NextCommentEligible || noReady.EligibilityState != models.EligibilityNoReadyActor {
		t.Fatalf("no ready account: got eligible=%v state=%q", noReady.NextCommentEligible, noReady.EligibilityState)
	}
	if noReady.IneligibilityMessageVI == "" || !strings.Contains(noReady.IneligibilityMessageVI, "Facebook") {
		t.Errorf("no_ready_account must carry a VN message, got %q", noReady.IneligibilityMessageVI)
	}

	// 2. No comments + a ready account → eligible.
	ready := buildCommentEligibility(
		models.LeadCoverageState{}, workspaceActorReadiness{candidate: 1, ready: 1, readyAccountIDs: []int64{10}}, policy, empty, now)
	if !ready.NextCommentEligible || ready.EligibilityState != models.EligibilityEligible {
		t.Fatalf("ready: got eligible=%v state=%q", ready.NextCommentEligible, ready.EligibilityState)
	}
	if ready.EligibleActorCount != 1 || ready.ReadyActorCount != 1 || ready.CandidateActorCount != 1 {
		t.Errorf("ready counts wrong: %+v", ready)
	}

	// 3. coverage_full (OrgTouchCount >= MaxAccountsPerLead) → blocked, reason matches the planner.
	covFull := models.LeadCoverageState{ActorsTouched: []int64{5, 6}, OrgTouchCount: 2}
	full := buildCommentEligibility(covFull, workspaceActorReadiness{candidate: 3, ready: 1, readyAccountIDs: []int64{10}}, policy, empty, now)
	if full.NextCommentEligible || full.IneligibilityReason != models.CoverageFull {
		t.Fatalf("coverage_full: got eligible=%v reason=%q", full.NextCommentEligible, full.IneligibilityReason)
	}
	if full.CommentedByCount != 2 || full.MaxCoverage != 2 {
		t.Errorf("coverage_full counts: commented=%d max=%d", full.CommentedByCount, full.MaxCoverage)
	}
	// Parity: the planner calls models.EvaluateCoverage — the dashboard reason must equal it.
	if _, plannerReason := models.EvaluateCoverage(covFull, policy, 10, now); plannerReason != full.IneligibilityReason {
		t.Errorf("parity broken: planner=%q dashboard=%q", plannerReason, full.IneligibilityReason)
	}

	// 4. Duplicate actor: the only ready account already commented → already_commented_by_this_actor.
	covDup := models.LeadCoverageState{ActorsTouched: []int64{10}, OrgTouchCount: 1}
	dup := buildCommentEligibility(covDup, workspaceActorReadiness{candidate: 1, ready: 1, readyAccountIDs: []int64{10}}, policy, empty, now)
	if dup.NextCommentEligible || dup.IneligibilityReason != models.CoverageAlreadyThisActor {
		t.Fatalf("duplicate actor: got eligible=%v reason=%q", dup.NextCommentEligible, dup.IneligibilityReason)
	}
	if _, plannerReason := models.EvaluateCoverage(covDup, policy, 10, now); plannerReason != dup.IneligibilityReason {
		t.Errorf("parity broken (dup): planner=%q dashboard=%q", plannerReason, dup.IneligibilityReason)
	}

	// 4b. A DIFFERENT ready actor (not yet touched) can still comment when coverage isn't full.
	mixed := buildCommentEligibility(covDup, workspaceActorReadiness{candidate: 2, ready: 1, readyAccountIDs: []int64{99}}, policy, empty, now)
	if !mixed.NextCommentEligible {
		t.Errorf("a fresh ready actor must be eligible when coverage not full, got %+v", mixed)
	}

	// 5. last_comment_attempt surfaces the latest comment entry's outcome.
	st := &models.LeadEngagementState{Entries: []models.LeadEngagement{
		{Action: "comment", Outcome: "failed"},
		{Action: "inbox", Outcome: "succeeded"},
	}}
	att := buildCommentEligibility(models.LeadCoverageState{}, workspaceActorReadiness{candidate: 1, ready: 1, readyAccountIDs: []int64{10}}, policy, st, now)
	if att.LastCommentAttemptStatus != "failed" {
		t.Errorf("last_comment_attempt_status: got %q want failed", att.LastCommentAttemptStatus)
	}
}

// TestCommentEligibilityNoSecrets guards that the §6 projection never serializes
// session/credential material — it is counts + reason codes only.
func TestCommentEligibilityNoSecrets(t *testing.T) {
	e := buildCommentEligibility(
		models.LeadCoverageState{ActorsTouched: []int64{7}, OrgTouchCount: 1},
		workspaceActorReadiness{candidate: 2, ready: 1, readyAccountIDs: []int64{7}},
		models.DefaultCoveragePolicy(), &models.LeadEngagementState{}, time.Now().UTC())
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"cookie", "token", "session", "fb_user_id", "password", "secret"} {
		if strings.Contains(strings.ToLower(string(raw)), bad) {
			t.Errorf("eligibility JSON leaked %q: %s", bad, raw)
		}
	}
}
