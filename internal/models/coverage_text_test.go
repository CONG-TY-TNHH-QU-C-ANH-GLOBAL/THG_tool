package models

import (
	"slices"
	"testing"
	"time"
)

func TestContentAccurateCoverage(t *testing.T) {
	t0 := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	touched := []LeadEngagement{{AccountID: 1, Outcome: "succeeded", PerformedAt: t0}}
	web := "https://thgfulfill.com"

	// 1. Org touched but the comment has NO website → website_already_used=false.
	st1 := ProjectLeadCoverage(touched, false, []string{"Bên mình hỗ trợ sourcing nhé"}, web)
	if st1.OrgTouchCount != 1 || st1.WebsiteAlreadyUsed {
		t.Errorf("touched-without-website must be website_already_used=false, got %+v", st1)
	}

	// 2. A comment that cites the verified website → website_already_used=true.
	st2 := ProjectLeadCoverage(touched, false, []string{"Xem web thgfulfill.com nhé"}, web)
	if !st2.WebsiteAlreadyUsed {
		t.Error("comment citing the website must set website_already_used=true")
	}

	// 3. A comment with an inbox/contact CTA → direct_cta_already_used=true.
	st3 := ProjectLeadCoverage(touched, false, []string{"Cần thì inbox mình nhé"}, web)
	if !st3.DirectCTAAlreadyUsed {
		t.Error("comment with inbox CTA must set direct_cta_already_used=true")
	}
	if DetectDirectCTAUsed([]string{"Liên hệ Zalo 0367689834"}) != true || DetectDirectCTAUsed([]string{"Telegram @Moonzzz03"}) != true {
		t.Error("Zalo phone + Telegram handle must count as a direct CTA")
	}

	pol := DefaultCoveragePolicy()
	// 4. A later actor gets no_link ONLY when website_already_used=true.
	if DeriveActorPersona(st1, pol, "", "").LinkPolicy != LinkMayIncludeWebsite {
		t.Error("no prior website → later actor may still include the website")
	}
	if DeriveActorPersona(st2, pol, "", "").LinkPolicy != LinkNoLink {
		t.Error("prior website → later actor must be no_link")
	}

	// 5. A later actor must not reuse an angle already present in earlier comments.
	st5 := ProjectLeadCoverage(touched, false, []string{"Giá rất cạnh tranh, base cost rẻ"}, web)
	if !slices.Contains(st5.UsedAngles, "price") {
		t.Fatalf("price angle should be classified, got %v", st5.UsedAngles)
	}
	if !slices.Contains(DeriveActorPersona(st5, pol, "", "").ForbiddenRepeatedPhrases, "price") {
		t.Error("later actor must be told to avoid the used 'price' angle")
	}
}
