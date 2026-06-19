package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// newContactDB opens a fresh, isolated store. A fresh DB per subtest matters:
// staff_contact_profiles is keyed by user_id alone (PRIMARY KEY user_id), so
// reusing user IDs across cases in one DB would collide via ON CONFLICT.
func newContactDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "contact_identity.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedStaff inserts an org-scoped staff contact profile.
func seedStaff(t *testing.T, db *store.Store, orgID, userID int64, active bool, p models.StaffContactProfile) {
	t.Helper()
	p.UserID = userID
	p.OrgID = orgID
	p.Active = active
	if err := db.UpsertStaffContactProfile(&p); err != nil {
		t.Fatalf("UpsertStaffContactProfile(user=%d): %v", userID, err)
	}
}

// seedAssignedAccount creates an account in orgID assigned to assignee and
// returns its id (used to exercise the assignee precedence tier).
func seedAssignedAccount(t *testing.T, db *store.Store, orgID, assignee int64) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "FB",
		AssignedUserID: assignee, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	return id
}

func disableCompanyFallback(t *testing.T, db *store.Store, orgID int64) {
	t.Helper()
	if err := db.Leads().SetContext(fmt.Sprintf("org:%d:allow_company_contact_fallback", orgID), "false"); err != nil {
		t.Fatalf("SetContext fallback=false: %v", err)
	}
}

// TestResolveStaffContactIdentity_Precedence pins the Sprint 5 resolution chain:
// initiator -> account assignee -> company default (if allowed) -> omit. Each
// case asserts the OfficialContact / PrimaryCTA actually carried into the prompt
// + contact guard, so a comment can only cite the resolved contact.
func TestResolveStaffContactIdentity_Precedence(t *testing.T) {
	const orgA int64 = 1
	base := models.CompanyIdentity{OfficialContact: "COMPANY-CONTACT", PrimaryCTA: "COMPANY-CTA"}

	initiatorProfile := models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391", PreferredCTA: "INBOX-ME"}
	assigneeProfile := models.StaffContactProfile{Telegram: "othersale", Zalo: "0900000000", PreferredCTA: "ASSIGNEE-CTA"}

	t.Run("initiator wins on unassigned account (reported bug)", func(t *testing.T) {
		db := newContactDB(t)
		seedStaff(t, db, orgA, 11, true, initiatorProfile)
		got := resolveStaffContactIdentity(db, orgA, 11, 0, base)
		if got.OfficialContact != "Telegram @hairypotter98 · Zalo 0949716391" {
			t.Fatalf("OfficialContact = %q", got.OfficialContact)
		}
		if got.PrimaryCTA != "INBOX-ME" {
			t.Fatalf("PrimaryCTA = %q, want INBOX-ME", got.PrimaryCTA)
		}
	})

	t.Run("initiator wins over a different assignee", func(t *testing.T) {
		db := newContactDB(t)
		seedStaff(t, db, orgA, 11, true, initiatorProfile)
		seedStaff(t, db, orgA, 22, true, assigneeProfile)
		acc := seedAssignedAccount(t, db, orgA, 22)
		got := resolveStaffContactIdentity(db, orgA, 11, acc, base)
		if got.OfficialContact != "Telegram @hairypotter98 · Zalo 0949716391" {
			t.Fatalf("expected initiator (user 11) contact, got %q", got.OfficialContact)
		}
		if got.PrimaryCTA != "INBOX-ME" {
			t.Fatalf("PrimaryCTA = %q, want initiator INBOX-ME", got.PrimaryCTA)
		}
	})

	t.Run("falls back to account assignee when initiator unusable", func(t *testing.T) {
		db := newContactDB(t)
		// initiator user 11 has no profile at all.
		seedStaff(t, db, orgA, 22, true, assigneeProfile)
		acc := seedAssignedAccount(t, db, orgA, 22)
		got := resolveStaffContactIdentity(db, orgA, 11, acc, base)
		if got.OfficialContact != "Telegram @othersale · Zalo 0900000000" {
			t.Fatalf("expected assignee (user 22) contact, got %q", got.OfficialContact)
		}
		if got.PrimaryCTA != "ASSIGNEE-CTA" {
			t.Fatalf("PrimaryCTA = %q, want ASSIGNEE-CTA", got.PrimaryCTA)
		}
	})

	t.Run("company fallback when neither usable and fallback allowed (default)", func(t *testing.T) {
		db := newContactDB(t)
		acc := seedAssignedAccount(t, db, orgA, 22) // assignee 22 has no profile
		got := resolveStaffContactIdentity(db, orgA, 11, acc, base)
		if got.OfficialContact != "COMPANY-CONTACT" {
			t.Fatalf("OfficialContact = %q, want COMPANY-CONTACT", got.OfficialContact)
		}
		if got.PrimaryCTA != "COMPANY-CTA" {
			t.Fatalf("PrimaryCTA = %q, want COMPANY-CTA", got.PrimaryCTA)
		}
	})

	t.Run("omit when neither usable and fallback disabled", func(t *testing.T) {
		db := newContactDB(t)
		disableCompanyFallback(t, db, orgA)
		acc := seedAssignedAccount(t, db, orgA, 22)
		got := resolveStaffContactIdentity(db, orgA, 11, acc, base)
		if got.OfficialContact != "" {
			t.Fatalf("OfficialContact = %q, want empty (fallback disabled)", got.OfficialContact)
		}
	})

	t.Run("empty fields filtered and empty PreferredCTA keeps company CTA", func(t *testing.T) {
		db := newContactDB(t)
		// Telegram+Zalo set, Phone/Email empty, no PreferredCTA.
		seedStaff(t, db, orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391"})
		got := resolveStaffContactIdentity(db, orgA, 11, 0, base)
		if got.OfficialContact != "Telegram @hairypotter98 · Zalo 0949716391" {
			t.Fatalf("OfficialContact = %q (should omit SĐT/Email)", got.OfficialContact)
		}
		if got.PrimaryCTA != "COMPANY-CTA" {
			t.Fatalf("PrimaryCTA = %q, want company CTA preserved", got.PrimaryCTA)
		}
	})

	t.Run("tenant isolation: profile under another org is not leaked", func(t *testing.T) {
		db := newContactDB(t)
		const orgB int64 = 2
		// The only profile row for user 11 lives under orgB; we resolve for orgA.
		seedStaff(t, db, orgB, 11, true, initiatorProfile)
		got := resolveStaffContactIdentity(db, orgA, 11, 0, base)
		if got.OfficialContact != "COMPANY-CONTACT" {
			t.Fatalf("cross-org leak: OfficialContact = %q, want COMPANY-CONTACT", got.OfficialContact)
		}
	})

	t.Run("inactive initiator profile falls through to assignee", func(t *testing.T) {
		db := newContactDB(t)
		seedStaff(t, db, orgA, 11, false, initiatorProfile) // inactive => unusable
		seedStaff(t, db, orgA, 22, true, assigneeProfile)
		acc := seedAssignedAccount(t, db, orgA, 22)
		got := resolveStaffContactIdentity(db, orgA, 11, acc, base)
		if got.OfficialContact != "Telegram @othersale · Zalo 0900000000" {
			t.Fatalf("inactive initiator should fall through; got %q", got.OfficialContact)
		}
	})
}
