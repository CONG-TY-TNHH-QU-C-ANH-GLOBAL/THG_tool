package models

import "time"

// Lead Lifecycle (spec: specs/domains/facebook-sales-intelligence/features/lead-lifecycle/technical.md). A THIRD orthogonal
// projection over the verified engagement ledger + conversation thread, on the
// work-management axis — distinct from the coordination badge (LeadEngagementState)
// and the multi-actor coverage picture (LeadCoverageState). freshness_state /
// next_action are DERIVED truth and never stored (feedback_verified_state_centric);
// only archived_at + archive_reason are persisted, because archiving is an explicit,
// reversible decision a human or the auto-archive job makes — not derivable from the
// ledger. The function is pure (clock injected) so transitions are unit-testable.

// LeadFreshnessState is the single-word work-management state the dashboard groups by.
type LeadFreshnessState string

const (
	// Fresh and eligible to act on now — newly crawled, or the customer just replied.
	LeadActive LeadFreshnessState = "active"
	// We touched it, the customer has not replied yet, still inside the wait window.
	LeadWaitingReply LeadFreshnessState = "waiting_reply"
	// We touched it, no reply, the wait window passed — time to follow up.
	LeadFollowupDue LeadFreshnessState = "followup_due"
	// We SUBMITTED a comment but it is not DOM-verified yet — held in a verification
	// cooldown so the planner does not re-comment and risk a real Facebook duplicate.
	LeadWaitingVerification LeadFreshnessState = "waiting_verification"
	// No meaningful activity for stale_after_days — cold, an archive candidate.
	LeadStale LeadFreshnessState = "stale"
	// Archived (archived_at set). Hidden from the default list; kept for dedup/coverage.
	LeadArchived LeadFreshnessState = "archived"
)

// LeadNextAction is the suggested next move; drives work-queue ordering in PR-2.
type LeadNextAction string

const (
	NextActionComment  LeadNextAction = "comment"  // untouched eligible lead → comment
	NextActionReply    LeadNextAction = "reply"    // customer replied → respond
	NextActionWait     LeadNextAction = "wait"     // inside the wait window → hold
	NextActionFollowup    LeadNextAction = "followup"     // wait window elapsed → re-engage
	NextActionVerifyLater LeadNextAction = "verify_later" // soft touch → reverify, don't recomment
	NextActionArchive     LeadNextAction = "archive"      // stale → archive candidate
	NextActionNone        LeadNextAction = "none"         // archived → nothing to do
)

// Archive reason codes (stable strings; shared by the maintenance job + UI mapping).
const (
	ArchiveReasonCold            = "cold_no_activity"
	ArchiveReasonCoverageNoReply = "coverage_full_no_reply"
	ArchiveReasonInvalidTarget   = "invalid_target_url"
	ArchiveReasonNotRelevant     = "manual_not_relevant"
	ArchiveReasonThreadInactive  = "thread_inactive"
)

// LeadLifecyclePolicy is the per-org tunable. DefaultLeadLifecyclePolicy() is the
// baseline used until a settings surface exists. Day windows are wired from config
// (LEAD_STALE_AFTER_DAYS etc.); FollowupWindow reuses the engagement default.
type LeadLifecyclePolicy struct {
	StaleAfterDays        int           `json:"stale_after_days"`
	ArchiveAfterDays      int           `json:"archive_after_days"`
	EvidenceRetentionDays int           `json:"evidence_retention_days"`
	RawCrawlRetentionDays int           `json:"raw_crawl_retention_days"`
	FollowupWindow        time.Duration `json:"followup_window"`
	VerificationCooldown  time.Duration `json:"verification_cooldown"` // soft touch holds the lead this long
}

// DefaultVerificationCooldown is how long a submitted-but-unverified lead stays in
// waiting_verification before falling back to a normal (retry-eligible) state. Long enough
// for the async reverify pass to run, short enough that a permanently-unverified soft touch
// does not strand the lead forever.
const DefaultVerificationCooldown = 30 * time.Minute

// DefaultLeadLifecyclePolicy returns the spec defaults (14 / 30 / 14 / 90 days, 24h, 30m).
func DefaultLeadLifecyclePolicy() LeadLifecyclePolicy {
	return LeadLifecyclePolicy{
		StaleAfterDays:        14,
		ArchiveAfterDays:      30,
		EvidenceRetentionDays: 14,
		RawCrawlRetentionDays: 90,
		FollowupWindow:        DefaultFollowupWindow,
		VerificationCooldown:  DefaultVerificationCooldown,
	}
}

// LeadLifecycleInputs carries the explicit per-lead signals the derivation branches on.
// All zero-value times mean "no such event". Callers project these from the verified
// ledger (LastEngagedAt), the thread (LastCustomerReplyAt), and the lead row
// (LastCrawledAt = created_at, ArchivedAt/ArchiveReason).
type LeadLifecycleInputs struct {
	LeadID             int64
	LastCrawledAt      time.Time
	LastEngagedAt      time.Time // latest HARD verified touch (action_ledger outcome=succeeded)
	LastSoftTouchAt    time.Time // latest submitted_unverified touch (soft touch; not verified)
	LastCustomerReplyAt time.Time // conversation_threads.last_inbound_at
	ThreadStatus       string    // closed/converted/replied/initiated/""
	ArchivedAt         time.Time
	ArchiveReason      string
}

// LeadLifecycleState is what the API/work-queue returns for one lead.
type LeadLifecycleState struct {
	LeadID              int64              `json:"lead_id"`
	FreshnessState      LeadFreshnessState `json:"freshness_state"`
	NextAction          LeadNextAction     `json:"next_action"`
	NextActionAt        time.Time          `json:"next_action_at"`
	LastSeenAt          time.Time          `json:"last_seen_at"`
	LastCrawledAt       time.Time          `json:"last_crawled_at"`
	LastEngagedAt       time.Time          `json:"last_engaged_at"`
	LastCustomerReplyAt time.Time          `json:"last_customer_reply_at"`
	ArchivedAt          time.Time          `json:"archived_at"`
	ArchiveReason       string             `json:"archive_reason"`
}

// LifecycleSummary is an at-a-glance tally of an org's leads by freshness state. It
// powers the copilot's "nothing eligible right now, but here is what exists" suggestion
// (PR-5) so "comment N leads" with no fresh targets degrades into a helpful next step.
type LifecycleSummary struct {
	Active              int `json:"active"`
	WaitingReply        int `json:"waiting_reply"`
	WaitingVerification int `json:"waiting_verification"`
	FollowupDue         int `json:"followup_due"`
	Stale               int `json:"stale"`
	Archived            int `json:"archived"`
}

// DeriveLeadLifecycle is the pure decision function. Order matters — first match wins
// (see spec table). It never reads outbound_messages.status or any proxy state; every
// branch is an explicit timestamp comparison.
func DeriveLeadLifecycle(in LeadLifecycleInputs, policy LeadLifecyclePolicy, now time.Time) LeadLifecycleState {
	if policy.StaleAfterDays <= 0 {
		policy.StaleAfterDays = 14
	}
	if policy.FollowupWindow <= 0 {
		policy.FollowupWindow = DefaultFollowupWindow
	}
	if policy.VerificationCooldown <= 0 {
		policy.VerificationCooldown = DefaultVerificationCooldown
	}
	st := LeadLifecycleState{
		LeadID:              in.LeadID,
		LastCrawledAt:       in.LastCrawledAt,
		LastEngagedAt:       in.LastEngagedAt,
		LastCustomerReplyAt: in.LastCustomerReplyAt,
		ArchivedAt:          in.ArchivedAt,
		ArchiveReason:       in.ArchiveReason,
		LastSeenAt:          maxTime(in.LastCrawledAt, in.LastEngagedAt, in.LastSoftTouchAt, in.LastCustomerReplyAt),
	}

	staleCutoff := now.Add(-time.Duration(policy.StaleAfterDays) * 24 * time.Hour)
	customerReplied := !in.LastCustomerReplyAt.IsZero() &&
		(in.LastEngagedAt.IsZero() || in.LastCustomerReplyAt.After(in.LastEngagedAt))
	// A soft touch holds the lead only while it is the freshest signal and inside the
	// cooldown — a later hard touch or customer reply takes precedence below.
	softTouchActive := !in.LastSoftTouchAt.IsZero() &&
		now.Sub(in.LastSoftTouchAt) < policy.VerificationCooldown &&
		in.LastSoftTouchAt.After(in.LastEngagedAt)

	switch {
	case !in.ArchivedAt.IsZero():
		st.FreshnessState, st.NextAction = LeadArchived, NextActionNone
	case customerReplied:
		// The customer is waiting on us — highest-value work, never stale.
		st.FreshnessState, st.NextAction, st.NextActionAt = LeadActive, NextActionReply, now
	case softTouchActive:
		// Submitted but unverified — wait for verification, do NOT re-comment.
		st.FreshnessState, st.NextAction = LeadWaitingVerification, NextActionVerifyLater
		st.NextActionAt = in.LastSoftTouchAt.Add(policy.VerificationCooldown)
	case in.LastEngagedAt.IsZero():
		// Never touched: fresh if recent, stale if it aged out untouched.
		if in.LastCrawledAt.Before(staleCutoff) {
			st.FreshnessState, st.NextAction, st.NextActionAt = LeadStale, NextActionArchive, now
		} else {
			st.FreshnessState, st.NextAction, st.NextActionAt = LeadActive, NextActionComment, now
		}
	case now.Sub(in.LastEngagedAt) < policy.FollowupWindow:
		due := in.LastEngagedAt.Add(policy.FollowupWindow)
		st.FreshnessState, st.NextAction, st.NextActionAt = LeadWaitingReply, NextActionWait, due
	case st.LastSeenAt.Before(staleCutoff):
		st.FreshnessState, st.NextAction, st.NextActionAt = LeadStale, NextActionArchive, now
	default:
		st.FreshnessState, st.NextAction, st.NextActionAt = LeadFollowupDue, NextActionFollowup, now
	}
	return st
}

// maxTime returns the latest of the given times (zero values ignored unless all zero).
func maxTime(times ...time.Time) time.Time {
	var max time.Time
	for _, t := range times {
		if t.After(max) {
			max = t
		}
	}
	return max
}
