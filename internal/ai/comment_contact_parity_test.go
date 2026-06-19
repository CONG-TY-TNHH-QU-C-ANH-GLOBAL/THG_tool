package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Case 3 — buildContactRule branches. When a Website is configured the rule MUST
// demand it exactly once and independently of the contact line; when empty it MUST
// forbid any URL (no fabrication). Pins the new MUST-include wording directly.
func TestBuildContactRule_WebsiteBranches(t *testing.T) {
	withWeb := buildContactRule(models.CompanyIdentity{
		CompanyName:     "THG Fulfill",
		Website:         "https://thgfulfill.com/vi",
		OfficialContact: "Telegram @hairypotter98",
	})
	for _, want := range []string{"MUST include the Website", "EXACTLY ONCE", "EVEN WHEN an Official contact is also present", "ONLY URL"} {
		if !strings.Contains(withWeb, want) {
			t.Fatalf("website rule missing %q: %q", want, withWeb)
		}
	}
	// The website demand must stand on its own — it must NOT be gated behind the
	// contact line (regression guard for the live-path bug where website dropped
	// once a staff contact was present).
	if !strings.Contains(withWeb, "include it EVEN WHEN an Official contact is also present") {
		t.Fatalf("website rule must be independent of the contact line: %q", withWeb)
	}

	noWeb := buildContactRule(models.CompanyIdentity{CompanyName: "THG Fulfill"})
	if !strings.Contains(noWeb, "do NOT include any URL") {
		t.Fatalf("no-website rule must forbid URLs: %q", noWeb)
	}
	if strings.Contains(noWeb, "MUST include the Website") {
		t.Fatalf("no-website rule must not demand a website: %q", noWeb)
	}
}

// Case 2b — the resolved staff CTA is RENDERED in the grounded prompt via ctaSuffix.
func TestCtaSuffix_RendersResolvedCTA(t *testing.T) {
	if got := ctaSuffix("Nhắn Telegram mình nhé"); !strings.Contains(got, "Nhắn Telegram mình nhé") {
		t.Fatalf("ctaSuffix dropped the resolved CTA: %q", got)
	}
	// Empty CTA degrades to a generic suffix, never a fabricated contact.
	if got := ctaSuffix(""); strings.Contains(got, "Telegram") {
		t.Fatalf("empty CTA must not invent a channel: %q", got)
	}
}

// Case 7 — NO DOUBLE CTA in the live grounded prompt. The staff CTA is single-
// sourced through ctaSuffix (rule 6); buildCompanyBlock does NOT render the CTA. So
// the exact CTA label must appear EXACTLY ONCE in the rendered prompt.
func TestBuildGroundedCommentPrompt_StaffCTARenderedExactlyOnce(t *testing.T) {
	const cta = "Nhắn Telegram @hairypotter98 nhé"
	identity := models.CompanyIdentity{
		CompanyName:     "THG Fulfill",
		Website:         "https://thgfulfill.com/vi",
		OfficialContact: "Telegram @hairypotter98 · Zalo 0949716391",
		PrimaryCTA:      cta,
		ServiceSummary:  "US fulfillment",
	}
	d := &models.CommentDecision{
		Intent:   models.IntentServiceSeeking,
		Selected: models.Selection{Capabilities: []models.GroundedItem{{Label: "US fulfillment cho TikTok Shop"}}},
	}
	prompt := buildGroundedCommentPrompt("ai làm fulfill US không", "An", nil, d, identity)

	if n := strings.Count(prompt, cta); n != 1 {
		t.Fatalf("staff CTA label must appear exactly once (ctaSuffix only), got %d:\n%s", n, prompt)
	}
	// The website + contact are present for brand trust, and the MUST-include rule fires.
	if !strings.Contains(prompt, "https://thgfulfill.com/vi") {
		t.Fatalf("grounded prompt missing company website:\n%s", prompt)
	}
	if !strings.Contains(prompt, "MUST include the Website") {
		t.Fatalf("grounded prompt missing website MUST-include rule:\n%s", prompt)
	}
}
