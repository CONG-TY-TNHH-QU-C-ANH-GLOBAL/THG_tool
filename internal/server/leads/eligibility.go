package leads

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// §6 Dashboard alignment. Per-lead comment eligibility, computed with the SAME
// gates as comment_all_leads — connectors.PickReadyConnector for account
// readiness, GetLeadCoverageState + models.EvaluateCoverage for coverage — so the
// dashboard and the planner never disagree on the reason. This lives in the
// server (composition) layer because it reads connectors, which the leads store
// domain must not. It exposes only counts + reason codes, never secrets.

// workspaceActorReadiness is the lead-independent half: how many Facebook accounts
// the workspace has and how many can execute a comment right now.
type workspaceActorReadiness struct {
	candidate       int
	ready           int
	readyAccountIDs []int64
}

func computeWorkspaceActorReadiness(ctx context.Context, db *store.Store, orgID int64) workspaceActorReadiness {
	var r workspaceActorReadiness
	accounts, err := db.Identities().GetAllAccounts(orgID)
	if err != nil {
		return r
	}
	conns, _ := db.Connectors().ListLocalConnectors(orgID)
	policy, _ := db.Connectors().GetExtensionPolicy()
	blocked, _ := db.Coordination().AccountActorStatesForOrg(ctx, orgID)
	for _, acc := range accounts {
		if acc.Platform != models.PlatformFacebook {
			continue
		}
		r.candidate++
		if blocked[acc.ID].Blocked { // persisted Verified-Actor block — not runnable
			continue
		}
		if _, reason := connectors.PickReadyConnector(conns, acc.ID, acc.FBUserID, policy); reason == connectors.ConnReady {
			r.ready++
			r.readyAccountIDs = append(r.readyAccountIDs, acc.ID)
		}
	}
	return r
}

// attachEligibility fills st.Eligibility for each lead. Workspace readiness is
// computed once; coverage is per-lead via the planner's own GetLeadCoverageState
// so the reason codes are identical.
func attachEligibility(ctx context.Context, db *store.Store, orgID int64, states map[int64]*models.LeadEngagementState) {
	if len(states) == 0 {
		return
	}
	readiness := computeWorkspaceActorReadiness(ctx, db, orgID)
	policy := models.DefaultCoveragePolicy()
	now := time.Now().UTC()
	for leadID, st := range states {
		if st == nil {
			continue
		}
		cov, err := db.Leads().GetLeadCoverageState(ctx, orgID, leadID, "")
		if err != nil || cov == nil {
			continue
		}
		st.Eligibility = buildCommentEligibility(*cov, readiness, policy, st, now)
	}
}

// buildCommentEligibility is the pure decision: it maps coverage + workspace
// readiness to the §6 contract. Every reason code is one comment_all_leads also
// emits (models.EvaluateCoverage) or the readiness state no_ready_account.
func buildCommentEligibility(cov models.LeadCoverageState, r workspaceActorReadiness, policy models.CoveragePolicy, st *models.LeadEngagementState, now time.Time) *models.CommentEligibility {
	e := &models.CommentEligibility{
		CommentedByCount:    cov.OrgTouchCount,
		MaxCoverage:         policy.MaxAccountsPerLead,
		CandidateActorCount: r.candidate,
		ReadyActorCount:     r.ready,
	}
	e.LastCommentAttemptStatus, e.LastCommentAttemptReason = lastCommentAttempt(st)

	var firstBlock string
	for _, accID := range r.readyAccountIDs {
		if ok, reason := models.EvaluateCoverage(cov, policy, accID, now); ok {
			e.EligibleActorCount++
		} else if firstBlock == "" {
			firstBlock = reason
		}
	}
	e.NextCommentEligible = e.EligibleActorCount > 0

	switch {
	case e.NextCommentEligible:
		e.EligibilityState = models.EligibilityEligible
	case r.ready == 0:
		e.EligibilityState = models.EligibilityNoReadyActor
		e.IneligibilityReason = models.EligibilityNoReadyActor
	case policy.MaxAccountsPerLead > 0 && cov.OrgTouchCount >= policy.MaxAccountsPerLead:
		// Deterministic across accounts — report it directly (matches the planner).
		e.EligibilityState = models.CoverageFull
		e.IneligibilityReason = models.CoverageFull
	default:
		reason := firstBlock
		if reason == "" {
			reason = "not_eligible"
		}
		e.EligibilityState = reason
		e.IneligibilityReason = reason
	}
	e.IneligibilityMessageVI = eligibilityMessageVI(e.IneligibilityReason)
	return e
}

// lastCommentAttempt reads the most-recent comment entry's outcome from the
// engagement projection (entries are most-recent first). The typed failure reason
// is not in the projection today, so reason is best-effort empty.
func lastCommentAttempt(st *models.LeadEngagementState) (status, reason string) {
	if st == nil {
		return "", ""
	}
	for _, en := range st.Entries {
		if en.Action == "comment" {
			return en.Outcome, ""
		}
	}
	return "", ""
}

// eligibilityMessageVI mirrors comment_all_leads' friendlySkipReasons copy for the
// reasons the dashboard surfaces. Empty for the eligible case — the UI composes
// the dynamic "Có thể comment bằng <n> tài khoản…" line from eligible_actor_count.
func eligibilityMessageVI(reason string) string {
	switch reason {
	case "":
		return ""
	case models.EligibilityNoReadyActor:
		return "Chưa thể comment: chưa có tài khoản Facebook sẵn sàng."
	case models.CoverageFull:
		return "Đã đủ số tài khoản tiếp cận lead này."
	case models.CoverageAlreadyThisActor:
		return "Các tài khoản Facebook sẵn sàng đều đã comment lead này."
	case models.CoverageLeadReplied:
		return "Lead đã trả lời — không gửi thêm comment."
	case models.CoverageGapTooSoon:
		return "Chưa đủ giãn cách giữa các lượt comment."
	case models.CoverageSingleActorPolicy:
		return "Chính sách chỉ cho 1 tài khoản tiếp cận mỗi lead."
	default:
		return "Chưa đủ điều kiện comment."
	}
}
