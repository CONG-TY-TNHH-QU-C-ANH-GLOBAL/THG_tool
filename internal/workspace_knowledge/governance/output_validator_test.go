package governance

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func mkAsset(id int64, typ assets.AssetType, state assets.AssetState, title, desc string, payload string) *assets.Asset {
	return &assets.Asset{
		ID:          id,
		OrgID:       7,
		Type:        typ,
		Title:       title,
		Description: desc,
		Payload:     json.RawMessage(payload),
		State:       state,
	}
}

// Empty output is always rejected. Defensive — an empty comment
// queued for send would land in a customer thread as nothing.
func TestValidate_EmptyOutputRejected(t *testing.T) {
	v := ValidateOutput("", nil)
	if v.Allow {
		t.Error("empty output should be rejected")
	}
	if v.Reasons[0].Code != CodeEmptyOutput {
		t.Errorf("wrong code: %v", v.Reasons[0].Code)
	}
}

// Whitespace-only is empty.
func TestValidate_WhitespaceOnlyRejected(t *testing.T) {
	v := ValidateOutput("   \n\t  ", nil)
	if v.Allow {
		t.Error("whitespace-only output should be rejected")
	}
}

// Excessively long output is rejected.
func TestValidate_ExcessiveLengthRejected(t *testing.T) {
	long := strings.Repeat("blah ", 400) // 2000 chars
	v := ValidateOutput(long, nil)
	if v.Allow {
		t.Error("excessive length should be rejected")
	}
}

// FABRICATED PRICING — the critical case for goal G4.
// LLM invents a price that doesn't appear in any retrieved asset.
func TestValidate_FabricatedPricingRejected(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateApproved,
			"Cat Tee", "premium 6.1oz",
			`{"price":"$18.50 wholesale"}`),
	}
	// Hallucinated price: $11.99 isn't in the asset.
	out := "Bên mình có cat tee chỉ $11.99 thôi, inbox mình nhé."
	v := ValidateOutput(out, retrieved)
	if v.Allow {
		t.Error("fabricated price $11.99 should be rejected")
	}
	if v.Reasons[0].Code != CodeFabricatedPricing {
		t.Errorf("wrong code: %v", v.Reasons[0].Code)
	}
}

// Pricing that IS in the catalog passes.
func TestValidate_GroundedPricingAllowed(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateApproved,
			"Cat Tee", "premium 6.1oz",
			`{"price":"$18.50 wholesale"}`),
	}
	out := "Cat tee giá sỉ $18.50, fulfillment 3-7 ngày. Inbox mình nhé."
	v := ValidateOutput(out, retrieved)
	if !v.Allow {
		t.Errorf("grounded price should pass; reasons: %+v", v.Reasons)
	}
}

// FAKE GUARANTEE — most-common LLM fabrication failure mode.
func TestValidate_FakeGuaranteeRejected(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateApproved,
			"Cat Tee", "premium quality", "{}"),
	}
	cases := []string{
		"Cat tee with lifetime warranty, inbox mình",
		"Cat tee — money-back guarantee on all orders",
		"We offer best price guaranteed on all POD products",
		"100% satisfaction on every order",
	}
	for _, c := range cases {
		v := ValidateOutput(c, retrieved)
		if v.Allow {
			t.Errorf("fake guarantee %q should be rejected", c)
		}
	}
}

// If a guarantee phrase IS in an approved asset, allow it.
func TestValidate_GroundedGuaranteeAllowed(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetShippingPolicy, assets.StateApproved,
			"Returns Policy",
			"We offer a 100% satisfaction guarantee on all POD products.",
			"{}"),
	}
	out := "Cat tee with 100% satisfaction, inbox mình."
	v := ValidateOutput(out, retrieved)
	if !v.Allow {
		t.Errorf("grounded guarantee should pass; reasons: %+v", v.Reasons)
	}
}

// HALLUCINATED SHIPPING — when no shipping asset was retrieved,
// shipping claims are forbidden.
func TestValidate_HallucinatedShippingRejected(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateApproved,
			"Cat Tee", "premium", "{}"),
	}
	cases := []string{
		"Cat tee with free shipping worldwide",
		"Cat tee, next day delivery available",
		"Cat tee — 30-day shipping to your door",
	}
	for _, c := range cases {
		v := ValidateOutput(c, retrieved)
		if v.Allow {
			t.Errorf("hallucinated shipping %q should be rejected", c)
		}
	}
}

// If a shipping_policy asset was retrieved, shipping claims pass.
func TestValidate_GroundedShippingAllowed(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateApproved, "Cat Tee", "premium", "{}"),
		mkAsset(2, assets.AssetShippingPolicy, assets.StateApproved,
			"Shipping SLA",
			"Production 3-7 days, US transit 5-10 days.",
			"{}"),
	}
	out := "Cat tee shipping 5-10 days to US."
	v := ValidateOutput(out, retrieved)
	if !v.Allow {
		t.Errorf("grounded shipping should pass; reasons: %+v", v.Reasons)
	}
}

// BANNED CLAIM DIRECT MATCH — if the LLM output literally contains
// the org's banned phrase, hard reject.
func TestValidate_BannedClaimDirectMatch(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetBannedClaim, assets.StateApproved,
			"best price guaranteed",
			"Cannot prove. Block at runtime.",
			"{}"),
	}
	out := "Our products are best price guaranteed, inbox mình!"
	v := ValidateOutput(out, retrieved)
	if v.Allow {
		t.Errorf("banned claim should be rejected")
	}
	// At least one reason should be CodeBannedClaimMatch.
	found := false
	for _, r := range v.Reasons {
		if r.Code == CodeBannedClaimMatch {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CodeBannedClaimMatch in reasons; got %+v", v.Reasons)
	}
}

// Hidden-state assets MUST NOT be treated as ground truth.
// Otherwise an operator who hides an asset still has its content
// "validating" hallucinated claims.
func TestValidate_HiddenAssetsDoNotGround(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateHidden,
			"Cat Tee", "premium 6.1oz",
			`{"price":"$11.99 wholesale"}`),
	}
	// Output references $11.99 — which exists in a HIDDEN asset.
	// Hidden assets should NOT ground claims; this must be rejected.
	out := "Cat tee giá $11.99, inbox."
	v := ValidateOutput(out, retrieved)
	if v.Allow {
		t.Error("hidden asset must not ground a price claim")
	}
}

// Banned-claim assets MUST NOT ground their own claims either.
func TestValidate_BannedAssetsDoNotGround(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetBannedClaim, assets.StateApproved,
			"best price guaranteed",
			"Sometimes lifetime warranty is also banned.",
			"{}"),
	}
	out := "We offer lifetime warranty on every order."
	v := ValidateOutput(out, retrieved)
	if v.Allow {
		t.Error("banned_claim asset must not ground a guarantee phrase")
	}
}

// Pure-text comment with no risky claims passes cleanly.
func TestValidate_CleanOutputAllowed(t *testing.T) {
	retrieved := []*assets.Asset{
		mkAsset(1, assets.AssetPODProduct, assets.StateApproved,
			"Cat Tee", "premium cotton", "{}"),
	}
	out := "Cat tee unisex heavyweight 6.1oz cotton. Inbox mình nhé!"
	v := ValidateOutput(out, retrieved)
	if !v.Allow {
		t.Errorf("clean output should pass; reasons: %+v", v.Reasons)
	}
}
