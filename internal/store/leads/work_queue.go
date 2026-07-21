package leads

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Work Queue read model (spec: specs/domains/facebook-sales-intelligence/features/lead-lifecycle/technical.md, PR-2). Builds the
// act-now candidate set the dashboard shows and the comment planner selects from:
// lifecycle-filtered (active/followup_due by default; archived already excluded upstream
// by GetLeadsFiltered, stale opt-in) and ordered by score → freshness → next_action_at.

// GetWorkQueue projects the work queue for an org. Coverage state is attached only when
// asked (ComputeCoverage / ActorAccountID) so the planner path stays cheap.
func (s *Store) GetWorkQueue(ctx context.Context, orgID int64, opts models.WorkQueueOptions) ([]models.WorkQueueItem, error) {
	fetch := opts.Limit
	if fetch <= 0 {
		fetch = 50
	}
	// GetAutomationLeadsForOrg already excludes archived (GetLeadsFiltered, PR-1) and
	// carries the leads/task_leads merge + dedup the planner relied on.
	leads, err := s.GetAutomationLeadsForOrg(orgID, opts.Score, fetch)
	if err != nil {
		return nil, err
	}
	// Explicit archived override (the planner never sets this; only a deliberate
	// dashboard/copilot request does). Archived leads come from their own read path
	// since the default candidate query excludes them.
	archivedIDs := map[int64]bool{}
	if opts.IncludeArchived {
		archived, aerr := s.ListArchivedLeads(ctx, orgID, fetch, 0)
		if aerr != nil {
			return nil, aerr
		}
		for _, a := range archived {
			archivedIDs[a.ID] = true
		}
		leads = append(leads, archived...)
	}
	now := time.Now()

	items := make([]models.WorkQueueItem, 0, len(leads))
	for _, lead := range leads {
		if item, ok := s.workQueueItemFor(ctx, orgID, lead, opts, archivedIDs[lead.ID], now); ok {
			items = append(items, item)
		}
	}
	models.SortWorkQueue(items)
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}
	return items, nil
}

// workQueueItemFor projects one candidate into a WorkQueueItem, returning ok=false
// when the lead's freshness state is filtered out by the options. Extracted from
// GetWorkQueue's loop body; behavior unchanged. archived selects the full lifecycle
// projection (freshness=archived + reason) for explicitly-appended archived leads.
func (s *Store) workQueueItemFor(ctx context.Context, orgID int64, lead models.Lead, opts models.WorkQueueOptions, archived bool, now time.Time) (models.WorkQueueItem, bool) {
	lc := s.lifecycleForCandidate(ctx, orgID, lead, opts.Policy)
	if archived {
		if full, ferr := s.GetLeadLifecycle(ctx, orgID, lead.ID, opts.Policy); ferr == nil {
			lc = *full
		}
	}
	if !opts.StateAllowed(lc.FreshnessState) {
		return models.WorkQueueItem{}, false
	}
	item := models.WorkQueueItem{Lead: lead, Lifecycle: lc}
	if opts.ComputeCoverage || opts.ActorAccountID > 0 {
		s.attachCoverage(ctx, orgID, &item, opts, now)
	}
	return item, true
}

// WorkQueueLeads returns just the lifecycle-ordered, act-now candidate leads the planner
// selects from — the "select from the work queue, not the raw lead list" change. It skips
// coverage projection (the planner derives coverage/persona itself in its loop).
func (s *Store) WorkQueueLeads(ctx context.Context, orgID int64, score string, limit int) ([]models.Lead, error) {
	items, err := s.GetWorkQueue(ctx, orgID, models.WorkQueueOptions{
		Score:  score,
		Limit:  limit,
		Policy: models.DefaultLeadLifecyclePolicy(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]models.Lead, 0, len(items))
	for _, it := range items {
		out = append(out, it.Lead)
	}
	return out, nil
}

// lifecycleForCandidate projects a candidate's lifecycle robustly: it works whether the
// lead lives in the canonical leads table or only in task_leads (mirror lag). Crawl time
// comes from the lead row; reply time is URL-keyed (table-independent); engagement is
// best-effort — a projection miss simply leaves the lead untouched → active.
func (s *Store) lifecycleForCandidate(ctx context.Context, orgID int64, lead models.Lead, policy models.LeadLifecyclePolicy) models.LeadLifecycleState {
	in := models.LeadLifecycleInputs{
		LeadID:              lead.ID,
		LastCrawledAt:       lead.CreatedAt,
		LastSoftTouchAt:     s.latestSoftTouchAt(ctx, orgID, engagementMatchURLs(&lead)),
		LastCustomerReplyAt: s.lastCustomerReplyAt(orgID, lead.AuthorURL),
	}
	if eng, err := s.GetLeadEngagement(ctx, orgID, lead.ID); err == nil {
		in.LastEngagedAt = eng.LastEngagedAt
		in.ThreadStatus = eng.ThreadStatus
	}
	return models.DeriveLeadLifecycle(in, policy, time.Now())
}

// attachCoverage best-effort-fills the multi-actor coverage state and, when an actor is
// given, that actor's eligibility verdict.
func (s *Store) attachCoverage(ctx context.Context, orgID int64, item *models.WorkQueueItem, opts models.WorkQueueOptions, now time.Time) {
	cov, err := s.GetLeadCoverageState(ctx, orgID, item.Lead.ID, opts.Website)
	if err != nil {
		// No coverage info yet (e.g. task_leads-only row) → not blocked by coverage.
		if opts.ActorAccountID > 0 {
			item.ActorEligible, item.ActorReason = true, models.CoverageOK
		}
		return
	}
	item.Coverage = *cov
	if opts.ActorAccountID > 0 {
		item.ActorEligible, item.ActorReason = models.EvaluateCoverage(*cov, opts.CoveragePolicy, opts.ActorAccountID, now)
	}
}
