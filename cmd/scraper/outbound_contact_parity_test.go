package main

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
)

// companyProfile is the org BusinessProfile carrying brand WEBSITE + a company
// hotline/email as OfficialContact — the data a staff swap must NOT erase (website)
// and must override (OfficialContact). resolveCommentIdentity takes the profile as a
// param, so we ground it with a real struct instead of round-tripping the store.
func companyProfile() *ai.BusinessProfile {
	return &ai.BusinessProfile{
		Name:            "THG Fulfill",
		Website:         "https://thgfulfill.com/vi",
		OfficialContact: "Hotline 1900-1234 · cs@thgfulfill.com",
		PrimaryCTA:      "inbox khảo sát",
		Services:        "US fulfillment + sourcing",
	}
}

// Case 1 — Website PRESERVED when the staff contact wins. The initiator has a
// usable Telegram+Zalo line (no website on staff), the company profile carries the
// website + a company hotline. The resolved identity must swap OfficialContact to
// the staff line, KEEP the company website, and NOT leak the company hotline/email.
func TestResolveCommentIdentity_WebsitePreservedWhenStaffWins(t *testing.T) {
	const orgA int64 = 1
	db := newContactDB(t)
	seedStaff(t, db, orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391"})

	id := resolveCommentIdentity(db, orgA, 11, 0, companyProfile(), nil)

	if id.OfficialContact != "Telegram @hairypotter98 · Zalo 0949716391" {
		t.Fatalf("OfficialContact = %q, want staff line", id.OfficialContact)
	}
	if id.Website != "https://thgfulfill.com/vi" {
		t.Fatalf("Website = %q, want company website PRESERVED through staff swap", id.Website)
	}
	if strings.Contains(id.OfficialContact, "1900-1234") || strings.Contains(id.OfficialContact, "cs@thgfulfill.com") {
		t.Fatalf("company hotline/email leaked into OfficialContact: %q", id.OfficialContact)
	}
}

// Case 2a — staff PreferredCTA wins and lands on the resolved identity.
func TestResolveCommentIdentity_StaffCTAWins(t *testing.T) {
	const orgA int64 = 1
	db := newContactDB(t)
	seedStaff(t, db, orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", PreferredCTA: "Nhắn Telegram mình nhé"})

	id := resolveCommentIdentity(db, orgA, 11, 0, companyProfile(), nil)
	if id.PrimaryCTA != "Nhắn Telegram mình nhé" {
		t.Fatalf("PrimaryCTA = %q, want staff CTA", id.PrimaryCTA)
	}
}

// Case 4 — LIVE-PATH PARITY (the core bug). The identity the live path builds
// (groundedCTA != nil) must carry the SAME staff OfficialContact + SAME company
// Website as the normal path (groundedCTA == nil) for identical inputs. Only the
// PrimaryCTA may differ (staff CTA still wins here, so even that is identical).
func TestResolveCommentIdentity_LivePathParity(t *testing.T) {
	const orgA int64 = 1
	db := newContactDB(t)
	seedStaff(t, db, orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391", PreferredCTA: "INBOX-STAFF"})
	prof := companyProfile()

	normal := resolveCommentIdentity(db, orgA, 11, 0, prof, nil)
	live := resolveCommentIdentity(db, orgA, 11, 0, prof, &models.GroundedItem{Label: "xem mẫu áo POD"})

	if normal.OfficialContact != live.OfficialContact {
		t.Fatalf("OfficialContact diverged: normal=%q live=%q", normal.OfficialContact, live.OfficialContact)
	}
	if normal.Website != live.Website || live.Website != "https://thgfulfill.com/vi" {
		t.Fatalf("Website diverged: normal=%q live=%q", normal.Website, live.Website)
	}
	// Staff CTA outranks BOTH the company default and the per-lead grounded label,
	// so the CTA is identical too.
	if normal.PrimaryCTA != "INBOX-STAFF" || live.PrimaryCTA != "INBOX-STAFF" {
		t.Fatalf("staff CTA should win on both paths: normal=%q live=%q", normal.PrimaryCTA, live.PrimaryCTA)
	}
}

// Case 4b — when NO staff CTA is set, the live path keeps the per-lead grounded
// label as PrimaryCTA while the normal path keeps the company CTA; OfficialContact +
// Website still match across both.
func TestResolveCommentIdentity_LivePathGroundedCTAWhenNoStaffCTA(t *testing.T) {
	const orgA int64 = 1
	db := newContactDB(t)
	seedStaff(t, db, orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391"}) // no PreferredCTA
	prof := companyProfile()

	normal := resolveCommentIdentity(db, orgA, 11, 0, prof, nil)
	live := resolveCommentIdentity(db, orgA, 11, 0, prof, &models.GroundedItem{Label: "xem mẫu áo POD"})

	if normal.OfficialContact != live.OfficialContact {
		t.Fatalf("OfficialContact diverged: normal=%q live=%q", normal.OfficialContact, live.OfficialContact)
	}
	if normal.Website != live.Website {
		t.Fatalf("Website diverged: normal=%q live=%q", normal.Website, live.Website)
	}
	if normal.PrimaryCTA != "inbox khảo sát" {
		t.Fatalf("normal PrimaryCTA = %q, want company CTA", normal.PrimaryCTA)
	}
	if live.PrimaryCTA != "xem mẫu áo POD" {
		t.Fatalf("live PrimaryCTA = %q, want per-lead grounded label", live.PrimaryCTA)
	}
}

// Case 5 — no invented data: a staff profile with empty Phone/Email never surfaces
// those channels through resolveCommentIdentity (only the filled Telegram/Zalo).
func TestResolveCommentIdentity_NoInventedContactChannels(t *testing.T) {
	const orgA int64 = 1
	db := newContactDB(t)
	seedStaff(t, db, orgA, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391"})

	id := resolveCommentIdentity(db, orgA, 11, 0, companyProfile(), nil)
	if strings.Contains(id.OfficialContact, "SĐT") || strings.Contains(id.OfficialContact, "Email") {
		t.Fatalf("empty staff phone/email must not appear: %q", id.OfficialContact)
	}
}

// Case 6 — tenant isolation: an initiator staff profile that lives under org B is
// NOT used when resolving for org A; the resolver falls back to the company contact.
func TestResolveCommentIdentity_TenantIsolation(t *testing.T) {
	const orgA, orgB int64 = 1, 2
	db := newContactDB(t)
	seedStaff(t, db, orgB, 11, true, models.StaffContactProfile{Telegram: "hairypotter98", Zalo: "0949716391"})

	id := resolveCommentIdentity(db, orgA, 11, 0, companyProfile(), nil)
	if id.OfficialContact != "Hotline 1900-1234 · cs@thgfulfill.com" {
		t.Fatalf("cross-org leak: OfficialContact = %q, want company hotline", id.OfficialContact)
	}
	if id.Website != "https://thgfulfill.com/vi" {
		t.Fatalf("Website = %q, want company website", id.Website)
	}
}
