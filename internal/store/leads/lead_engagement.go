// Domain: leads (see internal/store/DOMAINS.md)
package leads

import (
	"github.com/thg/scraper/internal/store/dbutil"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Coordination Plane PR-4: Lead Engagement State.
//
// Read-side projection of the Action Ledger keyed by lead. No new table:
// every row comes from action_ledger JOIN accounts JOIN users + an
// optional lookup against conversation_threads for inbox reply state.
//
// Battlefield model (feedback_shared_battlefield_not_crm.md): visibility
// only, never access control. Every staff sees every engagement.

// engagementMatchURLs collects the URLs a lead can be engaged through.
// A lead is "engaged" if the Action Ledger has an entry whose target_url
// matches any of these. Empty values are filtered out so we don't match
// blank target_urls against every empty-string ledger row (defensive —
// the ledger requires non-empty target_url at write time, but old rows
// or future writers may slip).
func engagementMatchURLs(lead *models.Lead) []string {
	if lead == nil {
		return nil
	}
	out := make([]string, 0, 3)
	for _, u := range []string{lead.SourceURL, lead.AuthorURL, lead.SecondaryURL} {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		out = append(out, u)
	}
	return out
}

// GetLeadEngagement returns the engagement state for one lead. Performs:
//  1. Load the lead row (need its URLs + author_url).
//  2. Project action_ledger entries matching any of the lead's URLs.
//  3. Enrich entries with assigned_user_id + user name + account name.
//  4. Look up the conversation_thread (if any) for inbox reply state.
//  5. Derive the badge via models.DeriveBadge.
//
// Org-scoped — a lead in another org returns sql.ErrNoRows.
func (s *Store) GetLeadEngagement(ctx context.Context, orgID, leadID int64) (*models.LeadEngagementState, error) {
	if orgID <= 0 || leadID <= 0 {
		return nil, fmt.Errorf("org_id and lead_id are required")
	}
	lead, err := s.getLeadForOrg(ctx, orgID, leadID)
	if err != nil {
		return nil, err
	}
	urls := engagementMatchURLs(lead)
	state := &models.LeadEngagementState{LeadID: leadID}

	if len(urls) > 0 {
		entries, err := s.listEngagementEntries(ctx, orgID, urls)
		if err != nil {
			return nil, err
		}
		state.Entries = entries
		if len(entries) > 0 {
			latest := entries[0]
			state.LastEngagedAt = latest.PerformedAt
			state.LastEngagedBy = latest.UserName
			state.LastEngagedAction = latest.Action
		}
		state.Champion, state.ActiveContributors = models.DeriveChampion(entries)
	}

	threadStatus, awaitingReply := s.threadStateForLead(orgID, lead)
	state.ThreadStatus = threadStatus
	state.Badge = models.DeriveBadge(state.Entries, threadStatus, awaitingReply, time.Now().UTC(),
		models.DefaultProtectedWindow, models.DefaultFollowupWindow)

	return state, nil
}

// GetLeadEngagementsBatch returns engagement state for many leads in one
// pass — required for the list view to avoid N+1.
//
// Implementation: build a single (target_url IN (...)) query covering all
// URLs across all leads, then bucket results back by lead. Threads are
// looked up per-lead because the keying is by profile_url and there is
// no batch helper today; cheap enough since leads list is bounded.
func (s *Store) GetLeadEngagementsBatch(ctx context.Context, orgID int64, leadIDs []int64) (map[int64]*models.LeadEngagementState, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("org_id is required")
	}
	out := make(map[int64]*models.LeadEngagementState, len(leadIDs))
	if len(leadIDs) == 0 {
		return out, nil
	}

	leads, err := s.getLeadsByIDsForOrg(ctx, orgID, leadIDs)
	if err != nil {
		return nil, err
	}

	// Build URL → []leadID inverse index so we can bucket ledger rows.
	urlToLeads := make(map[string][]int64)
	allURLs := make([]string, 0)
	for _, l := range leads {
		state := &models.LeadEngagementState{LeadID: l.ID}
		out[l.ID] = state
		for _, u := range engagementMatchURLs(&l) {
			urlToLeads[u] = append(urlToLeads[u], l.ID)
			allURLs = append(allURLs, u)
		}
	}

	// Pull all ledger entries that touch any of those URLs, enriched.
	if len(allURLs) > 0 {
		entriesByURL, err := s.listEngagementEntriesByURLs(ctx, orgID, allURLs)
		if err != nil {
			return nil, err
		}
		for url, leadIDsForURL := range urlToLeads {
			for _, lid := range leadIDsForURL {
				out[lid].Entries = append(out[lid].Entries, entriesByURL[url]...)
			}
		}
	}

	// Batch thread lookup — fixes the N+1 the per-lead loop used to do.
	// For a 50-lead list view this used to fire 50 separate thread queries;
	// now it is a single IN-clause query.
	threadByURL, err := s.batchThreadStateForLeads(orgID, leads)
	if err != nil {
		return nil, err
	}

	// Per-lead: sort entries (desc), look up thread, derive badge.
	now := time.Now().UTC()
	for _, l := range leads {
		state := out[l.ID]
		sortEngagementEntriesDesc(state.Entries)
		if len(state.Entries) > 0 {
			latest := state.Entries[0]
			state.LastEngagedAt = latest.PerformedAt
			state.LastEngagedBy = latest.UserName
			state.LastEngagedAction = latest.Action
		}
		state.Champion, state.ActiveContributors = models.DeriveChampion(state.Entries)
		threadStatus, awaitingReply := threadStateFromBatch(threadByURL, &l)
		state.ThreadStatus = threadStatus
		state.Badge = models.DeriveBadge(state.Entries, threadStatus, awaitingReply, now,
			models.DefaultProtectedWindow, models.DefaultFollowupWindow)
	}
	return out, nil
}

// batchThreadStateForLeads pulls all conversation_threads that match any
// lead's author_url in one IN-clause query. Returns a map keyed by
// profile_url so threadStateFromBatch can lookup per-lead in O(1).
func (s *Store) batchThreadStateForLeads(orgID int64, leads []models.Lead) (map[string]*models.ConversationThread, error) {
	urls := make([]string, 0, len(leads))
	seen := make(map[string]struct{}, len(leads))
	for _, l := range leads {
		u := strings.TrimSpace(l.AuthorURL)
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		return map[string]*models.ConversationThread{}, nil
	}
	return s.Threads().GetThreadsByProfilesForOrg(orgID, urls)
}

// threadStateFromBatch is the pure twin of threadStateForLead — same
// rules, but reads from a pre-fetched map instead of issuing a query.
func threadStateFromBatch(threads map[string]*models.ConversationThread, lead *models.Lead) (string, bool) {
	if lead == nil {
		return "", false
	}
	url := strings.TrimSpace(lead.AuthorURL)
	if url == "" {
		return "", false
	}
	thread, ok := threads[url]
	if !ok || thread == nil {
		return "", false
	}
	awaitingReply := !thread.LastOutboundAt.IsZero() &&
		(thread.LastInboundAt.IsZero() || thread.LastInboundAt.Before(thread.LastOutboundAt))
	return thread.Status, awaitingReply
}

// listEngagementEntries pulls + enriches ledger entries for one lead's
// URLs. Always returns most-recent-first.
func (s *Store) listEngagementEntries(ctx context.Context, orgID int64, urls []string) ([]models.LeadEngagement, error) {
	byURL, err := s.listEngagementEntriesByURLs(ctx, orgID, urls)
	if err != nil {
		return nil, err
	}
	out := make([]models.LeadEngagement, 0)
	for _, list := range byURL {
		out = append(out, list...)
	}
	sortEngagementEntriesDesc(out)
	return out, nil
}

// listEngagementEntriesByURLs is the shared SQL projection. Bucketed by
// target_url so the batch caller can rebind entries to leads.
//
// JOIN chain:
//   action_ledger
//     LEFT JOIN accounts ON accounts.id = action_ledger.account_id
//     LEFT JOIN users    ON users.id    = accounts.assigned_user_id
//
// LEFT JOINs because account / user may be missing (account_id=0 for
// legacy unauth queue paths; assigned_user_id=0 for unowned accounts).
// The projection still surfaces such rows — display layer renders
// "(unassigned)".
func (s *Store) listEngagementEntriesByURLs(ctx context.Context, orgID int64, urls []string) (map[string][]models.LeadEngagement, error) {
	out := make(map[string][]models.LeadEngagement, len(urls))
	if len(urls) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(urls))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, 0, len(urls)+1)
	args = append(args, orgID)
	for _, u := range urls {
		args = append(args, u)
	}

	// VERIFIED-ONLY projection. The autonomous-verified-execution
	// model (project goal, May-2026) makes "Đã chạm" (touched) state
	// derive ONLY from action_ledger rows whose outcome made it all
	// the way to a verified success. Pre-this-change, the projection
	// pulled every ledger row regardless of outcome — queued
	// attempts, redirected_feed failures, context_drift aborts, and
	// rate_limited rejects all rendered as "touched" because the
	// downstream DeriveBadge only counted len(entries)>0. That's the
	// bug behind the screenshot: lead #51 received `redirected_feed`
	// outcome yet the dashboard still showed "ĐÃ CHẠM".
	//
	// The SQL filter is the primary fix. DeriveBadge also defensively
	// filters in case a future caller bypasses this query and feeds
	// raw entries to the badge logic.
	//
	// tenant-ok: cross-domain projection (leads -> coordination). Reads
	// action_ledger as the source of engagement state. Per truth ownership
	// matrix (DOMAINS.md §2.4), action_ledger is owned by the coordination
	// domain; this JOIN is read-only.
	// Attribution uses al.created_by (the IMMUTABLE member who initiated the
	// action) — NOT account.assigned_user_id, which is mutable and would rewrite
	// history when an account is reassigned (Organic Sales Network execution
	// ownership). created_by=0 = system/legacy/unattributed.
	// Facebook actor attribution: a.fb_* identifies the account that performed the
	// interaction; account_runtime_state carries the Verified-Actor verdict (P1b).
	// Read-only; no ledger mutation; LEFT JOINs so a missing account/state degrades
	// gracefully (account_id=0 / verdict="").
	query := `
		SELECT al.target_url,
		       COALESCE(a.id, 0)                  AS account_id,
		       COALESCE(a.name, '')               AS account_name,
		       COALESCE(a.fb_display_name, '')     AS fb_display_name,
		       COALESCE(a.fb_profile_url, '')      AS fb_profile_url,
		       COALESCE(ars.last_actor_verdict, '') AS actor_verdict,
		       COALESCE(ars.actor_blocked, 0)      AS actor_blocked,
		       al.created_by                      AS user_id,
		       COALESCE(u.name, '')               AS user_name,
		       al.action_type,
		       al.outcome,
		       al.performed_at
		  FROM action_ledger al
		  LEFT JOIN accounts a ON a.id = al.account_id
		  LEFT JOIN users    u ON u.id = al.created_by
		  LEFT JOIN account_runtime_state ars ON ars.account_id = al.account_id
		 WHERE al.org_id = ?
		   AND al.outcome = 'succeeded'
		   AND al.target_url IN (` + placeholders + `)
		 ORDER BY al.performed_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			e           models.LeadEngagement
			performed   string
			outcome     sql.NullString
			actorBlocked int
		)
		if err := rows.Scan(&e.TargetURL, &e.AccountID, &e.AccountName, &e.FBDisplayName, &e.FBProfileURL,
			&e.ActorVerdict, &actorBlocked, &e.UserID, &e.UserName,
			&e.Action, &outcome, &performed); err != nil {
			return nil, err
		}
		if outcome.Valid {
			e.Outcome = outcome.String
		}
		e.ActorBlocked = actorBlocked == 1
		e.Channel = "facebook"
		e.PerformedAt = dbutil.ParseSQLiteTime(performed)
		out[e.TargetURL] = append(out[e.TargetURL], e)
	}
	return out, rows.Err()
}

// threadStateForLead returns the thread status + whether we are awaiting
// an inbound reply for a lead. Lookup keys on the lead's AuthorURL —
// that is the canonical profile URL inbox actions use.
// Empty author_url → no thread.
func (s *Store) threadStateForLead(orgID int64, lead *models.Lead) (string, bool) {
	if lead == nil {
		return "", false
	}
	url := strings.TrimSpace(lead.AuthorURL)
	if url == "" {
		return "", false
	}
	thread, err := s.Threads().GetThreadByProfileForOrg(orgID, url)
	if err != nil || thread == nil {
		return "", false
	}
	awaitingReply := !thread.LastOutboundAt.IsZero() &&
		(thread.LastInboundAt.IsZero() || thread.LastInboundAt.Before(thread.LastOutboundAt))
	return thread.Status, awaitingReply
}

// sortEngagementEntriesDesc sorts in place, most-recent first. Sort.Slice
// would work but reaching for the std-lib import for a 3-line bubble is
// not worth it given the typical N is small (<10 per lead).
func sortEngagementEntriesDesc(entries []models.LeadEngagement) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].PerformedAt.After(entries[j-1].PerformedAt); j-- {
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}
}

// getLeadForOrg is a minimal scoped fetch used only by the engagement
// projection. Kept private to this file so it doesn't fork the existing
// leads-store API. Reads only the columns engagement needs.
func (s *Store) getLeadForOrg(ctx context.Context, orgID, leadID int64) (*models.Lead, error) {
	var l models.Lead
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(org_id,0), COALESCE(source_url,''), COALESCE(author_url,''),
		        COALESCE(secondary_url,'')
		   FROM leads
		  WHERE id = ? AND COALESCE(org_id,0) = ?`,
		leadID, orgID,
	).Scan(&l.ID, &l.OrgID, &l.SourceURL, &l.AuthorURL, &l.SecondaryURL)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// getLeadsByIDsForOrg returns the minimum lead columns the batch
// engagement projection needs.
func (s *Store) getLeadsByIDsForOrg(ctx context.Context, orgID int64, leadIDs []int64) ([]models.Lead, error) {
	if len(leadIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(leadIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(leadIDs)+1)
	args = append(args, orgID)
	for _, id := range leadIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(org_id,0), COALESCE(source_url,''), COALESCE(author_url,''),
		        COALESCE(secondary_url,'')
		   FROM leads
		  WHERE COALESCE(org_id,0) = ? AND id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Lead, 0, len(leadIDs))
	for rows.Next() {
		var l models.Lead
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SourceURL, &l.AuthorURL, &l.SecondaryURL); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
