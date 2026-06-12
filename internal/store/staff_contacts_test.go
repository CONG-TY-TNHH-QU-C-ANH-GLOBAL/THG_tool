package store

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Round-trip + org isolation for staff contact profiles (PR-5).
func TestStaffContactProfiles_CRUDAndOrgScope(t *testing.T) {
	db := newSharedStore(t, "staff_contacts.db")

	p := &models.StaffContactProfile{
		UserID: 7, OrgID: 1, DisplayName: "Nguyễn A", Telegram: "saleA",
		Zalo: "0901111111", PreferredCTA: "Inbox mình nhé", Visibility: "team", Active: true,
	}
	if err := db.UpsertStaffContactProfile(p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := db.GetStaffContactProfile(1, 7)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Telegram != "saleA" || !got.Active || got.ContactLine() == "" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Wrong org never sees the profile (tenant isolation).
	if other, _ := db.GetStaffContactProfile(2, 7); other != nil {
		t.Fatalf("cross-org read leaked: %+v", other)
	}
	if list, _ := db.ListStaffContactProfiles(2); len(list) != 0 {
		t.Fatalf("cross-org list leaked: %v", list)
	}

	// Full-replace semantics: emptied field stays empty.
	p.Telegram = ""
	if err := db.UpsertStaffContactProfile(p); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = db.GetStaffContactProfile(1, 7)
	if got.Telegram != "" || got.ContactLine() != "Zalo 0901111111" {
		t.Fatalf("emptied field persisted: %+v", got)
	}
}
