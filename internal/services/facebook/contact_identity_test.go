package facebook

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// fakeContactDir is an in-memory ContactDirectory for tests — it keeps the facebook test
// suite free of internal/store (the consumer-owned port is the seam). org-scoped by
// construction, so the tenant-isolation cases are exercised honestly.
type fakeContactDir struct {
	staff    map[[2]int64]*models.StaffContactProfile
	accounts map[int64]*models.Account
	fallback map[int64]string // org-scoped company-contact-fallback setting
}

func newFakeContactDir() *fakeContactDir {
	return &fakeContactDir{
		staff:    map[[2]int64]*models.StaffContactProfile{},
		accounts: map[int64]*models.Account{},
		fallback: map[int64]string{},
	}
}

func (f *fakeContactDir) StaffContactProfile(orgID, userID int64) (*models.StaffContactProfile, error) {
	return f.staff[[2]int64{orgID, userID}], nil
}

func (f *fakeContactDir) AccountForOrg(accountID, orgID int64) (*models.Account, error) {
	a := f.accounts[accountID]
	if a == nil || a.OrgID != orgID {
		return nil, nil
	}
	return a, nil
}

func (f *fakeContactDir) CompanyContactFallbackSetting(orgID int64) (string, error) {
	return f.fallback[orgID], nil
}

// seedStaff inserts an org-scoped staff contact profile into the fake.
func (f *fakeContactDir) seedStaff(orgID, userID int64, active bool, p models.StaffContactProfile) {
	p.UserID = userID
	p.OrgID = orgID
	p.Active = active
	f.staff[[2]int64{orgID, userID}] = &p
}

// seedAssignedAccount records an account in orgID assigned to assignee and returns its id.
func (f *fakeContactDir) seedAssignedAccount(accountID, orgID, assignee int64) int64 {
	f.accounts[accountID] = &models.Account{ID: accountID, OrgID: orgID, AssignedUserID: assignee, Status: models.AccountActive}
	return accountID
}

func (f *fakeContactDir) disableCompanyFallback(orgID int64) {
	f.fallback[orgID] = "false"
}

// assertResolvedContact pins both the resolved OfficialContact and PrimaryCTA carried into
// the prompt + contact guard for the given resolution inputs.
func assertResolvedContact(t *testing.T, dir ContactDirectory, orgID, initiator, accountID int64, base models.CompanyIdentity, wantContact, wantCTA string) {
	t.Helper()
	got := resolveStaffContactIdentity(dir, orgID, initiator, accountID, base)
	if got.OfficialContact != wantContact {
		t.Fatalf("OfficialContact = %q, want %q", got.OfficialContact, wantContact)
	}
	if got.PrimaryCTA != wantCTA {
		t.Fatalf("PrimaryCTA = %q, want %q", got.PrimaryCTA, wantCTA)
	}
}

// assertResolvedOfficialContact pins only the resolved OfficialContact (used by cases where
// the PrimaryCTA is intentionally left unconstrained).
func assertResolvedOfficialContact(t *testing.T, dir ContactDirectory, orgID, initiator, accountID int64, base models.CompanyIdentity, wantContact string) {
	t.Helper()
	got := resolveStaffContactIdentity(dir, orgID, initiator, accountID, base)
	if got.OfficialContact != wantContact {
		t.Fatalf("OfficialContact = %q, want %q", got.OfficialContact, wantContact)
	}
}

// TestResolveStaffContactIdentity_Precedence pins the Sprint 5 resolution chain:
// initiator -> account assignee -> company default (if allowed) -> omit.
func TestResolveStaffContactIdentity_Precedence(t *testing.T) {
	const orgA int64 = 1
	base := models.CompanyIdentity{OfficialContact: "COMPANY-CONTACT", PrimaryCTA: "COMPANY-CTA"}

	initiatorProfile := models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391", PreferredCTA: "INBOX-ME"}
	assigneeProfile := models.StaffContactProfile{Telegram: "othersale", Zalo: "0900000000", PreferredCTA: "ASSIGNEE-CTA"}

	t.Run("initiator wins on unassigned account (reported bug)", func(t *testing.T) {
		f := newFakeContactDir()
		f.seedStaff(orgA, 11, true, initiatorProfile)
		assertResolvedContact(t, f, orgA, 11, 0, base, "Telegram @hairypotter98 · Zalo 0949716391", "INBOX-ME")
	})

	t.Run("initiator wins over a different assignee", func(t *testing.T) {
		f := newFakeContactDir()
		f.seedStaff(orgA, 11, true, initiatorProfile)
		f.seedStaff(orgA, 22, true, assigneeProfile)
		acc := f.seedAssignedAccount(100, orgA, 22)
		assertResolvedContact(t, f, orgA, 11, acc, base, "Telegram @hairypotter98 · Zalo 0949716391", "INBOX-ME")
	})

	t.Run("falls back to account assignee when initiator unusable", func(t *testing.T) {
		f := newFakeContactDir()
		f.seedStaff(orgA, 22, true, assigneeProfile)
		acc := f.seedAssignedAccount(100, orgA, 22)
		assertResolvedContact(t, f, orgA, 11, acc, base, "Telegram @othersale · Zalo 0900000000", "ASSIGNEE-CTA")
	})

	t.Run("company fallback when neither usable and fallback allowed (default)", func(t *testing.T) {
		f := newFakeContactDir()
		acc := f.seedAssignedAccount(100, orgA, 22)
		assertResolvedContact(t, f, orgA, 11, acc, base, "COMPANY-CONTACT", "COMPANY-CTA")
	})

	t.Run("omit when neither usable and fallback disabled", func(t *testing.T) {
		f := newFakeContactDir()
		f.disableCompanyFallback(orgA)
		acc := f.seedAssignedAccount(100, orgA, 22)
		assertResolvedOfficialContact(t, f, orgA, 11, acc, base, "")
	})

	t.Run("empty fields filtered and empty PreferredCTA keeps company CTA", func(t *testing.T) {
		f := newFakeContactDir()
		f.seedStaff(orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391"})
		assertResolvedContact(t, f, orgA, 11, 0, base, "Telegram @hairypotter98 · Zalo 0949716391", "COMPANY-CTA")
	})

	t.Run("tenant isolation: profile under another org is not leaked", func(t *testing.T) {
		f := newFakeContactDir()
		const orgB int64 = 2
		f.seedStaff(orgB, 11, true, initiatorProfile)
		assertResolvedOfficialContact(t, f, orgA, 11, 0, base, "COMPANY-CONTACT")
	})

	t.Run("inactive initiator profile falls through to assignee", func(t *testing.T) {
		f := newFakeContactDir()
		f.seedStaff(orgA, 11, false, initiatorProfile)
		f.seedStaff(orgA, 22, true, assigneeProfile)
		acc := f.seedAssignedAccount(100, orgA, 22)
		assertResolvedOfficialContact(t, f, orgA, 11, acc, base, "Telegram @othersale · Zalo 0900000000")
	})
}
