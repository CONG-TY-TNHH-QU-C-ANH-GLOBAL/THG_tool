package governance

import (
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Direct seam tests for the per-check helpers now that each lives in its
// own file (banned_claim_check.go / guarantee_check.go / shipping_check.go
// / pricing_check.go). ValidateOutput-level behavior is covered by
// output_validator_test.go; these pin the extracted predicates in isolation.

func TestBannedClaimReasons_MatchesTitleAndDescription(t *testing.T) {
	retrieved := []*assets.Asset{
		nil,
		{Type: assets.AssetBannedClaim, Title: "Best Price Guaranteed", Description: ""},
		{Type: assets.AssetPODProduct, Title: "Cat Tee", Description: "soft"},
	}
	reasons := bannedClaimReasons("we offer best price guaranteed today", retrieved)
	if len(reasons) != 1 || reasons[0].Code != CodeBannedClaimMatch {
		t.Fatalf("reasons = %+v, want 1 banned_claim_match", reasons)
	}
	if got := bannedClaimReasons("nothing forbidden here", retrieved); len(got) != 0 {
		t.Errorf("clean output produced %d reasons, want 0", len(got))
	}
}

func TestGuaranteeCheck_GroundingGate(t *testing.T) {
	if !findGuaranteePhrase("comes with a lifetime warranty") {
		t.Fatal("findGuaranteePhrase missed 'lifetime warranty'")
	}
	approved := []*assets.Asset{{Type: assets.AssetPODProduct, Title: "Policy", Description: "lifetime warranty included"}}
	if !approvedAssetContains(approved, "lifetime warranty", guaranteeWhitelistPhrases) {
		t.Error("approved asset with the phrase should ground it")
	}
	if approvedAssetContains(nil, "lifetime warranty", guaranteeWhitelistPhrases) {
		t.Error("no assets should not ground a guarantee")
	}
}

func TestShippingCheck_RequiresShippingAsset(t *testing.T) {
	if !findShippingClaim("free shipping worldwide") {
		t.Fatal("findShippingClaim missed 'free shipping worldwide'")
	}
	withPolicy := []*assets.Asset{{Type: assets.AssetShippingPolicy, Title: "Ship", Description: "5-10 days"}}
	if !approvedAssetMentionsShipping(withPolicy, "free shipping") {
		t.Error("a shipping_policy asset should ground a shipping claim")
	}
	if approvedAssetMentionsShipping(nil, "free shipping") {
		t.Error("no retrieved shipping context should not ground the claim")
	}
}

func TestPricingCheck_GroundedVsFabricated(t *testing.T) {
	retrieved := []*assets.Asset{{Type: assets.AssetPODProduct, Title: "Tee", Description: "wholesale $18.50"}}
	if r := fabricatedPriceReasons("ours is $18.50 each", retrieved); len(r) != 0 {
		t.Errorf("grounded price flagged: %+v", r)
	}
	r := fabricatedPriceReasons("special $9.99 deal", retrieved)
	if len(r) != 1 || r[0].Code != CodeFabricatedPricing {
		t.Fatalf("fabricated price not flagged: %+v", r)
	}
}
