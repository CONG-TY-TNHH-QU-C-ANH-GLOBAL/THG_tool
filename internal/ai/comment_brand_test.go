package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestResolveCompanyIdentity_FromProfileAndCTA(t *testing.T) {
	p := &BusinessProfile{Name: "THG Fulfill", Website: "https://thgfulfill.com", OfficialContact: "t.me/thgfulfill", PrimaryCTA: "inbox khảo sát", Services: "US fulfillment + sourcing"}
	id := ResolveCompanyIdentity(p, nil)
	if id.CompanyName != "THG Fulfill" || id.Website != "https://thgfulfill.com" || id.PrimaryCTA != "inbox khảo sát" || id.ServiceSummary != "US fulfillment + sourcing" {
		t.Fatalf("identity not resolved: %+v", id)
	}
	// A grounded per-lead CTA overrides the org default.
	id2 := ResolveCompanyIdentity(p, &models.GroundedItem{Label: "inbox gửi mẫu"})
	if id2.PrimaryCTA != "inbox gửi mẫu" {
		t.Fatalf("grounded CTA should override, got %q", id2.PrimaryCTA)
	}
}

func TestGenerateCommentPrompt_BrandTrust(t *testing.T) {
	// Service lead: prompt carries brand + service + contact policy, no SKU push.
	p := &BusinessProfile{Name: "THG Fulfill", Website: "https://thgfulfill.com", Services: "US fulfillment"}
	d := &models.CommentDecision{Intent: models.IntentServiceSeeking, Selected: models.Selection{Capabilities: []models.GroundedItem{{Label: "US fulfillment cho TikTok Shop"}}}}
	prompt := buildGroundedCommentPrompt("ai làm fulfill US", "An", p, d)
	for _, want := range []string{"THG Fulfill", "thgfulfill.com", "CONTACT POLICY", "SERVICE-SEEKING"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("brand prompt missing %q:\n%s", want, prompt)
		}
	}
	// Website MISSING → the prompt must forbid any URL (no fabrication).
	noWeb := buildGroundedCommentPrompt("ai làm fulfill US", "An", &BusinessProfile{Name: "THG Fulfill", Services: "US fulfillment"}, d)
	if !strings.Contains(noWeb, "do NOT include any URL") {
		t.Fatalf("no-website prompt must forbid URLs:\n%s", noWeb)
	}
}

func TestScreenCommentContacts(t *testing.T) {
	grounded := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com", OfficialContact: "t.me/thgfulfill 0901234567"}
	noContact := models.CompanyIdentity{CompanyName: "THG Fulfill"}

	cases := []struct {
		name     string
		text     string
		id       models.CompanyIdentity
		wantOK   bool
		wantCode string
	}{
		{"no contact at all is fine", "Bên em hỗ trợ sourcing VN/TQ, inbox em nhé.", grounded, true, ""},
		{"grounded website ok", "Tham khảo tại thgfulfill.com nhé.", grounded, true, ""},
		{"grounded website with scheme ok", "Xem https://www.thgfulfill.com/ nhé.", grounded, true, ""},
		{"non-grounded website rejected", "Xem shopee.vn/abc nhé.", grounded, false, "comment_unsupported_contact"},
		{"two URLs rejected", "Xem thgfulfill.com và fb.com/abc nhé.", grounded, false, "comment_multiple_urls"},
		{"website when none grounded rejected", "Xem thgfulfill.com nhé.", noContact, false, "comment_unsupported_contact"},
		{"fabricated email rejected", "Email fake@gmail.com nhé.", grounded, false, "comment_unsupported_contact"},
		{"fabricated phone rejected", "Gọi 0987654321 nhé.", grounded, false, "comment_unsupported_contact"},
		{"grounded phone ok", "Gọi 0901234567 nhé.", grounded, true, ""},
	}
	for _, c := range cases {
		ok, code := ScreenCommentContacts(c.text, c.id)
		if ok != c.wantOK || code != c.wantCode {
			t.Fatalf("%s: got ok=%v code=%q, want ok=%v code=%q", c.name, ok, code, c.wantOK, c.wantCode)
		}
	}
}

// The default comment prompt must surface a configured website + official contact
// (the founder's "comment thiếu website/contact" fix), and never invent one.
func TestCompanyBlockAndContactRule_IncludeConfiguredWebsiteContact(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com", OfficialContact: "t.me/thgfulfill", ServiceSummary: "US fulfillment"}
	block := buildCompanyBlock(id)
	if !strings.Contains(block, "https://thgfulfill.com") || !strings.Contains(block, "t.me/thgfulfill") {
		t.Fatalf("company block must list the website + contact, got: %q", block)
	}
	rule := buildContactRule(id)
	if !strings.Contains(strings.ToLower(rule), "include") {
		t.Errorf("contact rule should instruct INCLUDING the website/contact, got: %q", rule)
	}

	// When nothing is configured, the block names no website/contact and the rule
	// forbids inventing a URL (no fabrication).
	empty := buildCompanyBlock(models.CompanyIdentity{})
	if strings.Contains(empty, "http") {
		t.Errorf("empty identity must not produce a URL, got: %q", empty)
	}
	if !strings.Contains(buildContactRule(models.CompanyIdentity{}), "do NOT include any URL") {
		t.Error("no-website rule must forbid URLs")
	}
}
