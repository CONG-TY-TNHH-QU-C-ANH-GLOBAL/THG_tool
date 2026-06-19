package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// PR-2 GenerateCommentV2 Depth Upgrade — the prompt builder is pure + testable.
func TestBuildGroundedCommentPrompt_ProductSeekingCitesSKUAndPrice(t *testing.T) {
	d := &models.CommentDecision{
		Intent: models.IntentProductSeeking,
		Selected: models.Selection{
			Products: []models.GroundedItem{{Label: "Áo thun cotton", SKU: "TS-001", PriceText: "120k"}},
		},
	}
	p := buildGroundedCommentPrompt("cần tìm xưởng áo thun", "An", nil, d, ResolveCompanyIdentity(nil, d.Selected.CTA))
	for _, want := range []string{"SKU TS-001", "giá 120k", "PRODUCT-SEEKING"} {
		if !strings.Contains(p, want) {
			t.Fatalf("product prompt missing %q:\n%s", want, p)
		}
	}
}

func TestBuildGroundedCommentPrompt_ServiceSeekingUsesCapabilityNotSKU(t *testing.T) {
	d := &models.CommentDecision{
		Intent: models.IntentServiceSeeking,
		Selected: models.Selection{
			Capabilities: []models.GroundedItem{{Label: "US fulfillment cho TikTok Shop"}},
			CTA:          &models.GroundedItem{Label: "Inbox mình nhé"},
		},
	}
	p := buildGroundedCommentPrompt("ai làm fulfill US không", "", nil, d, ResolveCompanyIdentity(nil, d.Selected.CTA))
	if !strings.Contains(p, "SERVICE-SEEKING") {
		t.Fatalf("service prompt missing intent rule:\n%s", p)
	}
	if !strings.Contains(p, "US fulfillment cho TikTok Shop") {
		t.Fatalf("service prompt missing capability:\n%s", p)
	}
	// Anonymous author → no salutation rule.
	if !strings.Contains(p, "anonymous") && !strings.Contains(p, "do NOT use any name") {
		t.Fatalf("anonymous author rule missing:\n%s", p)
	}
}
