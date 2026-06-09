package models

import "time"

// Coordination Plane PR-4: Lead Engagement State.
//
// This is a READ-side projection of the Action Ledger keyed by lead, NOT
// an access-control layer. Battlefield model: every staff still sees
// every lead. The badge + entries surface competitive metadata so the
// team self-coordinates by *visibility*, not by *exclusion*.
//
// See feedback_shared_battlefield_not_crm.md + project_distributed_coordination.md.

// LeadEngagement is one row of engagement history for a lead — one
// action by one staff member via one account, taken from the Action
// Ledger and enriched with operator identity for surfacing in the UI.
type LeadEngagement struct {
	UserID      int64     `json:"user_id"`     // al.created_by — the IMMUTABLE initiator; 0 = system/agent
	UserName    string    `json:"user_name"`   // initiator name; "" if user_id == 0
	AccountID   int64     `json:"account_id"`
	AccountName string    `json:"account_name"`
	// Facebook actor attribution (Lead Facebook Interaction Attribution). The
	// account that actually performed the interaction — execution is OWNED even
	// though the lead is SHARED. Observability only; no ownership/lock.
	FBDisplayName string `json:"fb_display_name"`
	FBProfileURL  string `json:"fb_profile_url"`
	ActorVerdict  string `json:"actor_verdict"` // verified | mismatch | unknown | ""
	ActorBlocked  bool   `json:"actor_blocked"`
	Channel       string `json:"channel"` // "facebook"
	Action      string    `json:"action"`      // comment | inbox | group_post | profile_post
	TargetURL   string    `json:"target_url"`
	Outcome     string    `json:"outcome"`     // queued | succeeded | failed | skipped
	PerformedAt time.Time `json:"performed_at"`
}

// LeadEngagementBadge is the single-word UI summary the dashboard renders
// next to a lead. Drives the list-view triage decision.
type LeadEngagementBadge string

const (
	// Untouched — no engagement recorded against this lead. Highest opportunity.
	LeadBadgePriority LeadEngagementBadge = "priority"
	// Engaged within the protected window — don't pile on. Especially relevant
	// for inbox actions where a same-minute follow-up reads as spam cluster.
	LeadBadgeProtected LeadEngagementBadge = "protected"
	// Inbox sent, no inbound reply yet, past the protected window — the
	// queueing staff is still the natural follow-up owner.
	LeadBadgeFollowupPending LeadEngagementBadge = "followup_pending"
	// Engaged but not recent — team-visible, anyone can act on it.
	LeadBadgeVisible LeadEngagementBadge = "visible"
	// Conversation thread closed or converted. Don't re-engage.
	LeadBadgeClosed LeadEngagementBadge = "closed"
)

// LeadEngagementState is what the API returns for one lead.
type LeadEngagementState struct {
	LeadID            int64               `json:"lead_id"`
	Badge             LeadEngagementBadge `json:"badge"`
	Entries           []LeadEngagement    `json:"entries"`             // most-recent first
	LastEngagedAt     time.Time           `json:"last_engaged_at"`     // zero if no entries
	LastEngagedBy     string              `json:"last_engaged_by"`     // user_name of latest entry; "" if none
	LastEngagedAction string              `json:"last_engaged_action"` // action of latest entry; "" if none
	ThreadStatus      string              `json:"thread_status"`       // conversation_threads.status if any (closed/converted/replied/initiated); "" if none

	// Coordination Layer (observability ONLY — Organic Sales Network). Multiple
	// members engaging one shared lead is VALID (amplification); these fields just
	// surface "what's happening" so the team self-coordinates by visibility.
	ActiveContributors []string `json:"active_contributors"` // distinct member names who engaged this lead
	Champion           string   `json:"champion"`            // top contributor (most verified touches); analytics only, no rights
}

// DeriveChampion is a pure projection over the engagement entries: it returns
// the distinct contributor names (most-recent first by first appearance) and the
// "champion" — the member with the most verified touches on this lead. Champion
// is ANALYTICS ONLY (Ownership ⊥ Champion): it grants no routing/execution
// priority and never implies lead ownership. Ties break by earliest contributor.
func DeriveChampion(entries []LeadEngagement) (champion string, contributors []string) {
	counts := map[string]int{}
	order := []string{}
	for _, e := range entries {
		name := e.UserName
		if name == "" {
			continue
		}
		if _, seen := counts[name]; !seen {
			order = append(order, name)
		}
		counts[name]++
	}
	best := 0
	for _, name := range order {
		if counts[name] > best {
			best = counts[name]
			champion = name
		}
	}
	return champion, order
}

// DefaultProtectedWindow is how recent an engagement must be to mark the
// lead as "protected" in the UI. 15 minutes is short enough that idle
// leads don't stay protected forever, and long enough that two staff
// don't race each other in the same minute.
const DefaultProtectedWindow = 15 * time.Minute

// DefaultFollowupWindow bounds how long an inbox-without-reply keeps the
// followup_pending badge. After this window the lead falls back to
// visible — the original queueing staff has had their fair chance.
const DefaultFollowupWindow = 24 * time.Hour

// DeriveBadge is the pure decision function. Callers pass entries sorted
// most-recent first, the optional conversation thread (for inbox replies),
// and a fixed "now" so tests are deterministic.
//
// Rules (order matters — first match wins):
//   1. Thread closed/converted → closed.
//   2. No VERIFIED-TOUCH entries → priority.
//   3. Latest verified-touch entry < protected window → protected.
//   4. Latest verified-touch action was inbox + thread awaiting reply
//      + < followup window → followup_pending.
//   5. Otherwise → visible.
//
// VERIFIED-TOUCH filter: per the autonomous-verified-execution model
// (project goal, May-2026), only entries whose ledger outcome is
// IsLedgerOutcomeVerifiedTouch == true count toward the badge. Queued,
// failed, context_drift, blocked, and rate_limited entries are
// FILTERED OUT here even though the SQL projection upstream already
// filters them — defense in depth so a future caller wiring DeriveBadge
// against an unfiltered slice cannot silently regress the invariant.
func DeriveBadge(entries []LeadEngagement, threadStatus string, threadAwaitingReply bool, now time.Time, protectedWindow, followupWindow time.Duration) LeadEngagementBadge {
	if protectedWindow <= 0 {
		protectedWindow = DefaultProtectedWindow
	}
	if followupWindow <= 0 {
		followupWindow = DefaultFollowupWindow
	}
	if threadStatus == "closed" || threadStatus == "converted" {
		return LeadBadgeClosed
	}
	verified := filterVerifiedTouches(entries)
	if len(verified) == 0 {
		return LeadBadgePriority
	}
	latest := verified[0]
	if now.Sub(latest.PerformedAt) < protectedWindow {
		return LeadBadgeProtected
	}
	if latest.Action == "inbox" && threadAwaitingReply && now.Sub(latest.PerformedAt) < followupWindow {
		return LeadBadgeFollowupPending
	}
	return LeadBadgeVisible
}

// filterVerifiedTouches retains only entries that represent a real
// customer touch — i.e. action_ledger rows whose outcome was
// "succeeded". Backed by IsLedgerOutcomeVerifiedTouch so the rule is
// single-sourced.
func filterVerifiedTouches(entries []LeadEngagement) []LeadEngagement {
	if len(entries) == 0 {
		return entries
	}
	out := make([]LeadEngagement, 0, len(entries))
	for _, e := range entries {
		if IsLedgerOutcomeVerifiedTouch(e.Outcome) {
			out = append(out, e)
		}
	}
	return out
}
