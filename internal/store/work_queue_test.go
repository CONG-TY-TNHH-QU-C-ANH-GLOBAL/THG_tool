// Domain: leads (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"strconv"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// seedAgedLead inserts a listable lead whose created_at is `daysAgo` in the past, so the
// lifecycle projection sees it as aged (for stale assertions). SQLite datetime arithmetic.
func seedAgedLead(t *testing.T, db *Store, orgID int64, sourceURL, authorURL string, daysAgo int) int64 {
	t.Helper()
	res, err := db.db.Exec(
		`INSERT INTO leads (org_id, source_type, source_id, source_url, author, author_url,
		                    platform, content, score, service_match, author_role, pain_point,
		                    ai_reasoning, niche, classified_at, created_at)
		 VALUES (?, 'post', 0, ?, 'Lead Author', ?, 'facebook', 'hi', 'cold', 'None',
		         'unknown', '', '', 'logistics', CURRENT_TIMESTAMP, datetime('now', ?))`,
		orgID, sourceURL, authorURL, formatDaysAgo(daysAgo),
	)
	if err != nil {
		t.Fatalf("seed aged lead: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func formatDaysAgo(d int) string {
	return "-" + strconv.Itoa(d) + " days"
}

// The work queue surfaces fresh leads and hides archived + stale by default; the planner
// helper (WorkQueueLeads) sees the same act-now set.
func TestWorkQueue_HidesArchivedAndStale(t *testing.T) {
	db := newSharedStore(t, "workqueue.db")
	ctx := context.Background()

	fresh := seedListableLead(t, db, 1, "https://facebook.com/post/WQ1", "https://facebook.com/profile/WQ1")
	stale := seedAgedLead(t, db, 1, "https://facebook.com/post/WQ2", "https://facebook.com/profile/WQ2", 40)
	archived := seedListableLead(t, db, 1, "https://facebook.com/post/WQ3", "https://facebook.com/profile/WQ3")
	if err := db.Leads().ArchiveLead(ctx, 1, archived, models.ArchiveReasonCold); err != nil {
		t.Fatalf("archive: %v", err)
	}

	items, err := db.Leads().GetWorkQueue(ctx, 1, models.WorkQueueOptions{Limit: 50})
	if err != nil {
		t.Fatalf("GetWorkQueue: %v", err)
	}
	got := map[int64]models.LeadFreshnessState{}
	for _, it := range items {
		got[it.Lead.ID] = it.Lifecycle.FreshnessState
	}
	if _, ok := got[fresh]; !ok {
		t.Errorf("fresh lead %d should be in the work queue", fresh)
	}
	if _, ok := got[stale]; ok {
		t.Errorf("stale lead %d must be hidden by default", stale)
	}
	if _, ok := got[archived]; ok {
		t.Errorf("archived lead %d must never appear", archived)
	}

	// Planner selection path sees the same act-now set.
	leads, err := db.Leads().WorkQueueLeads(ctx, 1, "", 50)
	if err != nil {
		t.Fatalf("WorkQueueLeads: %v", err)
	}
	if !containsLeadID(leads, fresh) || containsLeadID(leads, stale) || containsLeadID(leads, archived) {
		t.Errorf("WorkQueueLeads must include only fresh; got fresh=%v stale=%v archived=%v",
			containsLeadID(leads, fresh), containsLeadID(leads, stale), containsLeadID(leads, archived))
	}
}

// IncludeArchived is the explicit override: archived leads surface only when asked. The
// planner path (WorkQueueLeads) never sets it, so the planner still excludes archived.
func TestWorkQueue_IncludeArchivedOverride(t *testing.T) {
	db := newSharedStore(t, "workqueue_archived.db")
	ctx := context.Background()
	fresh := seedListableLead(t, db, 1, "https://facebook.com/post/IA1", "https://facebook.com/profile/IA1")
	archived := seedListableLead(t, db, 1, "https://facebook.com/post/IA2", "https://facebook.com/profile/IA2")
	if err := db.Leads().ArchiveLead(ctx, 1, archived, models.ArchiveReasonNotRelevant); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// Default: archived hidden.
	def, err := db.Leads().GetWorkQueue(ctx, 1, models.WorkQueueOptions{Limit: 50})
	if err != nil {
		t.Fatalf("GetWorkQueue default: %v", err)
	}
	for _, it := range def {
		if it.Lead.ID == archived {
			t.Fatalf("archived lead must be hidden without the override")
		}
	}

	// Override: archived surfaces with freshness_state=archived.
	over, err := db.Leads().GetWorkQueue(ctx, 1, models.WorkQueueOptions{Limit: 50, IncludeArchived: true})
	if err != nil {
		t.Fatalf("GetWorkQueue override: %v", err)
	}
	foundArchived, foundFresh := false, false
	for _, it := range over {
		if it.Lead.ID == archived {
			foundArchived = true
			if it.Lifecycle.FreshnessState != models.LeadArchived {
				t.Errorf("override lead state = %s, want archived", it.Lifecycle.FreshnessState)
			}
		}
		if it.Lead.ID == fresh {
			foundFresh = true
		}
	}
	if !foundArchived {
		t.Errorf("IncludeArchived should surface archived lead %d", archived)
	}
	if !foundFresh {
		t.Errorf("override should still include the fresh lead %d", fresh)
	}

	// Planner path never opts in → archived stays excluded.
	plannerLeads, err := db.Leads().WorkQueueLeads(ctx, 1, "", 50)
	if err != nil {
		t.Fatalf("WorkQueueLeads: %v", err)
	}
	if containsLeadID(plannerLeads, archived) {
		t.Errorf("planner must never select archived lead %d", archived)
	}
}

// IncludeStale surfaces stale leads when explicitly requested (e.g. an auto-archive sweep).
func TestWorkQueue_IncludeStale(t *testing.T) {
	db := newSharedStore(t, "workqueue_stale.db")
	ctx := context.Background()
	stale := seedAgedLead(t, db, 1, "https://facebook.com/post/WQS", "https://facebook.com/profile/WQS", 40)

	items, err := db.Leads().GetWorkQueue(ctx, 1, models.WorkQueueOptions{Limit: 50, IncludeStale: true})
	if err != nil {
		t.Fatalf("GetWorkQueue: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Lead.ID == stale {
			found = true
			if it.Lifecycle.FreshnessState != models.LeadStale {
				t.Errorf("lead %d state = %s, want stale", stale, it.Lifecycle.FreshnessState)
			}
		}
	}
	if !found {
		t.Errorf("IncludeStale should surface stale lead %d", stale)
	}
}
