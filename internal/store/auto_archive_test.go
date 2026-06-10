// Domain: leads (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"slices"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// The sweep archives cold/aged leads with a typed reason and leaves fresh leads alone.
func TestArchiveSweep_ArchivesColdKeepsFresh(t *testing.T) {
	db := newSharedStore(t, "archive_sweep.db")
	ctx := context.Background()

	fresh := seedListableLead(t, db, 1, "https://facebook.com/post/AS1", "https://facebook.com/profile/AS1")
	cold := seedAgedLead(t, db, 1, "https://facebook.com/post/AS2", "https://facebook.com/profile/AS2", 45)

	report, err := db.Leads().ArchiveSweep(ctx, 1, models.DefaultLeadLifecyclePolicy(), models.DefaultCoveragePolicy(), "")
	if err != nil {
		t.Fatalf("ArchiveSweep: %v", err)
	}
	if report.Archived != 1 {
		t.Fatalf("archived = %d, want 1 (reasons=%v scanned=%d)", report.Archived, report.ByReason, report.Scanned)
	}
	if report.ByReason[models.ArchiveReasonCold] != 1 {
		t.Errorf("expected 1 cold archive, got %v", report.ByReason)
	}

	// Cold lead is now hidden; fresh lead remains in the default list.
	list, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered: %v", err)
	}
	if containsLeadID(list, cold) {
		t.Errorf("cold lead %d should have been archived (hidden)", cold)
	}
	if !containsLeadID(list, fresh) {
		t.Errorf("fresh lead %d must remain visible", fresh)
	}

	// Idempotent: a second sweep archives nothing new.
	report2, err := db.Leads().ArchiveSweep(ctx, 1, models.DefaultLeadLifecyclePolicy(), models.DefaultCoveragePolicy(), "")
	if err != nil {
		t.Fatalf("ArchiveSweep (2nd): %v", err)
	}
	if report2.Archived != 0 {
		t.Errorf("second sweep archived = %d, want 0", report2.Archived)
	}
}

// LeadLifecycleSummary tallies live + archived leads for the copilot suggestion.
func TestLeadLifecycleSummary(t *testing.T) {
	db := newSharedStore(t, "lifecycle_summary.db")
	ctx := context.Background()

	seedListableLead(t, db, 1, "https://facebook.com/post/S1", "https://facebook.com/profile/S1") // active
	seedAgedLead(t, db, 1, "https://facebook.com/post/S2", "https://facebook.com/profile/S2", 40)  // stale
	arch := seedListableLead(t, db, 1, "https://facebook.com/post/S3", "https://facebook.com/profile/S3")
	if err := db.Leads().ArchiveLead(ctx, 1, arch, models.ArchiveReasonNotRelevant); err != nil {
		t.Fatalf("archive: %v", err)
	}

	sum, err := db.Leads().LeadLifecycleSummary(ctx, 1)
	if err != nil {
		t.Fatalf("LeadLifecycleSummary: %v", err)
	}
	if sum.Active != 1 {
		t.Errorf("active = %d, want 1", sum.Active)
	}
	if sum.Stale != 1 {
		t.Errorf("stale = %d, want 1", sum.Stale)
	}
	if sum.Archived != 1 {
		t.Errorf("archived = %d, want 1", sum.Archived)
	}
}

// OrgIDsWithActiveLeads returns only orgs that still own non-archived leads.
func TestOrgIDsWithActiveLeads(t *testing.T) {
	db := newSharedStore(t, "archive_orgs.db")
	ctx := context.Background()

	a := seedListableLead(t, db, 7, "https://facebook.com/post/O7", "https://facebook.com/profile/O7")
	seedListableLead(t, db, 9, "https://facebook.com/post/O9", "https://facebook.com/profile/O9")
	if err := db.Leads().ArchiveLead(ctx, 7, a, models.ArchiveReasonNotRelevant); err != nil {
		t.Fatalf("archive: %v", err)
	}

	orgIDs, err := db.Leads().OrgIDsWithActiveLeads(ctx)
	if err != nil {
		t.Fatalf("OrgIDsWithActiveLeads: %v", err)
	}
	has := func(id int64) bool { return slices.Contains(orgIDs, id) }
	// Org 7's only lead is archived → should not appear; org 9 still active.
	if has(7) {
		t.Errorf("org 7 has no live leads, should not appear: %v", orgIDs)
	}
	if !has(9) {
		t.Errorf("org 9 has a live lead, should appear: %v", orgIDs)
	}
}
