// Domain: leads (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func newLifecycleTestStore(t *testing.T) *Store {
	return newSharedStore(t, "lifecycle.db")
}

// seedListableLead inserts a lead with a non-null author, matching how the ingest
// pipeline (InsertLead) actually writes — so GetLeadsFiltered can scan it. The shared
// seedLead helper leaves author NULL, which only the engagement projection tolerates.
func seedListableLead(t *testing.T, db *Store, orgID int64, sourceURL, authorURL string) int64 {
	t.Helper()
	res, err := db.db.Exec(
		`INSERT INTO leads (org_id, source_type, source_id, source_url, author, author_url,
		                    platform, content, score, service_match, author_role, pain_point,
		                    ai_reasoning, niche, classified_at)
		 VALUES (?, 'post', 0, ?, 'Lead Author', ?, 'facebook', 'hi', 'cold', 'None',
		         'unknown', '', '', 'logistics', CURRENT_TIMESTAMP)`,
		orgID, sourceURL, authorURL,
	)
	if err != nil {
		t.Fatalf("seed listable lead: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// A freshly-seeded, untouched lead projects to active (work queue input).
func TestLeadLifecycle_FreshIsActive(t *testing.T) {
	db := newLifecycleTestStore(t)
	ctx := context.Background()
	leadID := seedListableLead(t, db, 1, "https://facebook.com/post/L1", "https://facebook.com/profile/L1")

	st, err := db.Leads().GetLeadLifecycle(ctx, 1, leadID, models.DefaultLeadLifecyclePolicy())
	if err != nil {
		t.Fatalf("GetLeadLifecycle: %v", err)
	}
	if st.FreshnessState != models.LeadActive {
		t.Errorf("freshness_state = %s, want active", st.FreshnessState)
	}
	if st.NextAction != models.NextActionComment {
		t.Errorf("next_action = %s, want comment", st.NextAction)
	}
}

// ArchiveLead → lifecycle reports archived, the default list hides the lead, and
// UnarchiveLead restores it. No hard delete: the row survives throughout.
func TestLeadLifecycle_ArchiveHidesAndRestores(t *testing.T) {
	db := newLifecycleTestStore(t)
	ctx := context.Background()
	leadID := seedListableLead(t, db, 1, "https://facebook.com/post/L2", "https://facebook.com/profile/L2")

	// Visible before archiving.
	before, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered before: %v", err)
	}
	if !containsLeadID(before, leadID) {
		t.Fatalf("lead %d should be in the default list before archiving", leadID)
	}

	if err := db.Leads().ArchiveLead(ctx, 1, leadID, models.ArchiveReasonNotRelevant); err != nil {
		t.Fatalf("ArchiveLead: %v", err)
	}

	st, err := db.Leads().GetLeadLifecycle(ctx, 1, leadID, models.DefaultLeadLifecyclePolicy())
	if err != nil {
		t.Fatalf("GetLeadLifecycle after archive: %v", err)
	}
	if st.FreshnessState != models.LeadArchived {
		t.Errorf("freshness_state = %s, want archived", st.FreshnessState)
	}
	if st.ArchiveReason != models.ArchiveReasonNotRelevant || st.ArchivedAt.IsZero() {
		t.Errorf("archive metadata not set: %+v", st)
	}

	// Hidden from the default list (which also backs planner selection).
	after, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered after: %v", err)
	}
	if containsLeadID(after, leadID) {
		t.Errorf("archived lead %d must NOT appear in the default list", leadID)
	}

	// Restore.
	if err := db.Leads().UnarchiveLead(ctx, 1, leadID); err != nil {
		t.Fatalf("UnarchiveLead: %v", err)
	}
	restored, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered restored: %v", err)
	}
	if !containsLeadID(restored, leadID) {
		t.Errorf("unarchived lead %d should reappear in the default list", leadID)
	}
}

// Archiving is org-scoped: another tenant cannot archive this org's lead.
func TestLeadLifecycle_ArchiveIsTenantScoped(t *testing.T) {
	db := newLifecycleTestStore(t)
	ctx := context.Background()
	leadID := seedListableLead(t, db, 1, "https://facebook.com/post/L3", "https://facebook.com/profile/L3")

	if err := db.Leads().ArchiveLead(ctx, 2, leadID, models.ArchiveReasonCold); err != nil {
		t.Fatalf("ArchiveLead (wrong org): %v", err)
	}
	// Lead belongs to org 1, so org 2's archive is a no-op — still visible to org 1.
	got, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered: %v", err)
	}
	if !containsLeadID(got, leadID) {
		t.Errorf("cross-tenant archive must not hide org 1's lead %d", leadID)
	}
}

// The archived list surfaces only archived leads (with lifecycle reason via batch), and
// the batch projection keys by lead id.
func TestLeadLifecycle_ArchivedListAndBatch(t *testing.T) {
	db := newLifecycleTestStore(t)
	ctx := context.Background()
	live := seedListableLead(t, db, 1, "https://facebook.com/post/B1", "https://facebook.com/profile/B1")
	gone := seedListableLead(t, db, 1, "https://facebook.com/post/B2", "https://facebook.com/profile/B2")
	if err := db.Leads().ArchiveLead(ctx, 1, gone, models.ArchiveReasonNotRelevant); err != nil {
		t.Fatalf("archive: %v", err)
	}

	archived, err := db.Leads().ListArchivedLeads(ctx, 1, 50, 0)
	if err != nil {
		t.Fatalf("ListArchivedLeads: %v", err)
	}
	if !containsLeadID(archived, gone) || containsLeadID(archived, live) {
		t.Errorf("archived list should contain only %d; got live=%v gone=%v",
			gone, containsLeadID(archived, live), containsLeadID(archived, gone))
	}

	batch, err := db.Leads().GetLeadLifecyclesBatch(ctx, 1, []int64{live, gone}, models.DefaultLeadLifecyclePolicy())
	if err != nil {
		t.Fatalf("GetLeadLifecyclesBatch: %v", err)
	}
	if batch[live].FreshnessState != models.LeadActive {
		t.Errorf("live lead state = %s, want active", batch[live].FreshnessState)
	}
	if batch[gone].FreshnessState != models.LeadArchived || batch[gone].ArchiveReason != models.ArchiveReasonNotRelevant {
		t.Errorf("archived lead lifecycle wrong: %+v", batch[gone])
	}
}

func containsLeadID(leads []models.Lead, id int64) bool {
	for _, l := range leads {
		if l.ID == id {
			return true
		}
	}
	return false
}
