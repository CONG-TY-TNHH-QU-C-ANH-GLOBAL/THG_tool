package models

import "testing"

func companyID() CompanyIdentity {
	return CompanyIdentity{
		CompanyName:     "THG Fulfill",
		Website:         "https://www.thgfulfill.com/vi",
		OfficialContact: "Telegram @company",
		PrimaryCTA:      "Inbox mình nhé",
	}
}

// PR-5 core guarantee: two salespeople produce two DIFFERENT contact
// lines from the same company identity — comments stop reusing one
// global handle.
func TestApplyStaffContact_TwoStaffDifferentLines(t *testing.T) {
	a := &StaffContactProfile{UserID: 1, Active: true, Telegram: "saleA", Zalo: "0901111111", PreferredCTA: "Inbox mình để mình khảo sát mẫu."}
	b := &StaffContactProfile{UserID: 2, Active: true, Zalo: "0902222222"}

	idA := ApplyStaffContact(a, companyID(), true)
	idB := ApplyStaffContact(b, companyID(), true)

	if idA.OfficialContact == idB.OfficialContact {
		t.Fatalf("two staff produced the same contact line: %q", idA.OfficialContact)
	}
	if idA.OfficialContact != "Telegram @saleA · Zalo 0901111111" {
		t.Errorf("staff A line = %q", idA.OfficialContact)
	}
	if idB.OfficialContact != "Zalo 0902222222" {
		t.Errorf("staff B line = %q", idB.OfficialContact)
	}
	// Staff A's preferred CTA overrides the org default; B keeps it.
	if idA.PrimaryCTA != "Inbox mình để mình khảo sát mẫu." || idB.PrimaryCTA != "Inbox mình nhé" {
		t.Errorf("CTA resolution wrong: A=%q B=%q", idA.PrimaryCTA, idB.PrimaryCTA)
	}
	// Brand truth is untouched — only the contact line is per-staff.
	if idA.Website != idB.Website || idA.CompanyName != "THG Fulfill" {
		t.Errorf("company identity mutated: %+v", idA)
	}
}

// Resolution chain tail: missing/inactive staff contact → company
// fallback when allowed; omitted entirely when not. Never invented.
func TestApplyStaffContact_FallbackAndOmit(t *testing.T) {
	if id := ApplyStaffContact(nil, companyID(), true); id.OfficialContact != "Telegram @company" {
		t.Errorf("nil staff + fallback → company contact, got %q", id.OfficialContact)
	}
	if id := ApplyStaffContact(nil, companyID(), false); id.OfficialContact != "" {
		t.Errorf("nil staff + no fallback → omitted, got %q", id.OfficialContact)
	}
	inactive := &StaffContactProfile{UserID: 3, Active: false, Telegram: "ghost"}
	if id := ApplyStaffContact(inactive, companyID(), true); id.OfficialContact != "Telegram @company" {
		t.Errorf("inactive staff → company fallback, got %q", id.OfficialContact)
	}
	empty := &StaffContactProfile{UserID: 4, Active: true}
	if id := ApplyStaffContact(empty, companyID(), false); id.OfficialContact != "" {
		t.Errorf("empty staff + no fallback → omitted, got %q", id.OfficialContact)
	}
}

func TestStaffContactLine_Composition(t *testing.T) {
	p := &StaffContactProfile{Telegram: "@x", Zalo: "09", Phone: "08", Email: "e@x.com"}
	if got := p.ContactLine(); got != "Telegram @x · Zalo 09 · SĐT 08" {
		t.Errorf("full line = %q (email only used when nothing else)", got)
	}
	onlyEmail := &StaffContactProfile{Email: "e@x.com"}
	if got := onlyEmail.ContactLine(); got != "Email e@x.com" {
		t.Errorf("email-only line = %q", got)
	}
	if (&StaffContactProfile{}).ContactLine() != "" {
		t.Errorf("empty profile must produce empty line")
	}
}
