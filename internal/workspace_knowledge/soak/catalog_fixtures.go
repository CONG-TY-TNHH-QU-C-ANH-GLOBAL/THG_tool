package soak

import (
	"encoding/json"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// CatalogFixture is one asset the soak harness will load into the
// store. Mirrors the operator-facing UpsertKnowledgeAsset shape but
// stripped of the runtime fields (CreatedAt, Metrics, etc.) the
// store fills automatically.
type CatalogFixture struct {
	ExternalID  string
	Type        assets.AssetType
	Title       string
	Description string
	Tags        []string
	Payload     map[string]any
	// State defaults to approved when empty so retrieval reads it.
	// Set explicitly to pending/hidden when the fixture is meant to
	// test those cases.
	State assets.AssetState
	// Pinned + Boost let fixtures exercise operator-intent paths.
	Pinned bool
	Boost  int
}

// textAsset builds a non-product fixture (policy / CTA / banned claim, or a
// payload-less product). Collapses the repeated literal shape so the catalog
// list stays DRY; values are unchanged.
func textAsset(id string, typ assets.AssetType, title, desc string, tags ...string) CatalogFixture {
	return CatalogFixture{ExternalID: id, Type: typ, Title: title, Description: desc, Tags: tags}
}

// podProduct builds a POD-product fixture with a wholesale-price payload. Reuses
// textAsset so the shared field shape lives in exactly one place.
func podProduct(id, title, desc, price string, tags ...string) CatalogFixture {
	c := textAsset(id, assets.AssetPODProduct, title, desc, tags...)
	c.Payload = map[string]any{"price": price}
	return c
}

// Fixture modifiers — value receivers return a modified copy so they chain off a
// builder inside the slice literal (e.g. textAsset(...).pinned()).
func (c CatalogFixture) pinned() CatalogFixture                       { c.Pinned = true; return c }
func (c CatalogFixture) boosted(b int) CatalogFixture                 { c.Boost = b; return c }
func (c CatalogFixture) withState(s assets.AssetState) CatalogFixture { c.State = s; return c }

// RealisticCatalog returns a curated POD/fulfillment catalog
// matching the actual workspace this system serves. ~25 assets
// spanning products, policies, CTAs, and one banned claim.
//
// The catalog is HAND-TUNED so the soak harness exercises:
//   - Multi-token products (cat tee, dog hoodie)
//   - Policy assets (shipping SLA, refund policy)
//   - CTA assets (Vietnamese DM invite)
//   - Banned-claim governance (this one MUST never surface in any
//     retrieval result — soak verifies)
//   - Operator-pinned asset (must survive even with weak score)
//   - Boosted asset (must rank competitively)
//   - Hidden asset (must NEVER appear in approved-only queries)
func RealisticCatalog() []CatalogFixture {
	return []CatalogFixture{
		// --- Cat-niche products ---
		podProduct("shopify_cat_tee_001", "Custom Cat Tee — Unisex Heavyweight", "Premium ring-spun cotton, 6.1oz. POD with 7-day production.", "$18.50 wholesale", "cat", "tee", "unisex", "heavyweight", "pod"),
		podProduct("shopify_cat_mug_002", "Cat Lover Ceramic Mug 11oz", "Dishwasher safe, full-wrap sublimation print.", "$7.20 wholesale", "cat", "mug", "ceramic", "pet"),
		podProduct("shopify_cat_hoodie_003", "Cat Mom Oversized Hoodie — Streetwear", "50/50 blend, baggy fit. DTG print on heavyweight blanks.", "$22.00 wholesale", "cat", "hoodie", "oversized", "streetwear", "mom"),
		// --- Dog-niche products ---
		podProduct("shopify_dog_hoodie_004", "Custom Dog Dad Pullover Hoodie", "Soft-hand DTG print on midweight cotton-poly hoodie.", "$22.00 wholesale", "dog", "hoodie", "dad", "pullover", "pod"),
		podProduct("shopify_dog_bandana_005", "Personalised Dog Bandana — Cotton Twill", "Custom pet name + tag-friendly cotton twill bandana.", "$6.50 wholesale", "dog", "bandana", "pet", "personalised"),
		// --- Aesthetic / niche-style products ---
		podProduct("csv_anime_tee_006", "Anime Gothic Oversized Tee — Black", "Dark anime-inspired streetwear; oversized fit, edgy designs.", "$24.00 wholesale", "anime", "gothic", "oversized", "tee", "streetwear"),
		podProduct("csv_kawaii_hoodie_007", "Kawaii Pastel Hoodie — Y2K Aesthetic", "Soft pastel colourways, Japanese-inspired graphics.", "$26.00 wholesale", "anime", "kawaii", "hoodie", "pastel"),
		// --- Shipping / policy assets ---
		textAsset("notion_shipping_sla_008", assets.AssetShippingPolicy, "Shipping & Production SLA", "Production 3-7 days. US transit 5-10 days. EU transit 8-14 days. Tracking included.", "shipping", "sla", "us", "eu", "fulfillment"),
		textAsset("notion_returns_009", assets.AssetShippingPolicy, "Returns & Refunds Policy v3", "90-day defect warranty. POD products are made-to-order; non-defect returns not accepted.", "returns", "refunds", "policy", "warranty"),
		textAsset("sheet_pricing_010", assets.AssetPricingRule, "Wholesale Pricing Tier", "MOQ 20 = 5% off. MOQ 100 = 15% off. MOQ 500 = 25% off plus dedicated rep.", "pricing", "wholesale", "moq", "tier"),
		// --- CTAs ---
		textAsset("notion_cta_vi_dm_011", assets.AssetCTA, "Soft CTA — Vietnamese DM Invite", "Inbox mình nhé, mình gửi catalog chi tiết và giá sỉ riêng.", "cta", "vi", "inbox", "dm"),
		textAsset("notion_cta_en_dm_012", assets.AssetCTA, "Soft CTA — English DM Invite", "DM me for the full catalog and wholesale pricing.", "cta", "en", "dm"),
		// --- Compliance asset — banned claim. Banned: must NEVER surface in
		// retrieval results regardless of query. The soak verifies this.
		textAsset("compliance_banned_001", assets.AssetBannedClaim, "Banned: best price guaranteed", "Cannot prove. FTC Section 5 risk. Block in all outbound text.", "compliance", "banned", "guaranteed"),
		// --- Operator-pinned CTA. Pinned + zero-score on most queries — soak
		// must surface it via the pin path, not the semantic path.
		textAsset("notion_pinned_cta_013", assets.AssetCTA, "PINNED CTA — Wholesale enquiry", "For wholesale enquiries, message us with your MOQ + target market.", "cta", "wholesale", "pinned").pinned(),
		// --- Boosted seasonal product ---
		podProduct("csv_seasonal_014", "Christmas Cat Sweater — Pre-order", "Q4 seasonal. 45-day lead time. Pre-orders open Aug.", "$21.00 wholesale", "cat", "seasonal", "christmas", "preorder").boosted(60),
		// --- Hidden asset (must never appear in approved-only results) ---
		textAsset("csv_hidden_015", assets.AssetPODProduct, "Generic Plain Tote — Low Margin", "No design. Considered low-margin filler SKU.", "tote", "plain").withState(assets.StateHidden),
		// --- Pending asset (not yet approved by operator) ---
		textAsset("csv_pending_016", assets.AssetPODProduct, "Halloween Bat Tee — Awaiting approval", "Operator has not reviewed this CSV row yet.", "halloween", "tee", "seasonal").withState(assets.StatePending),
	}
}

// PayloadJSON serialises a fixture's payload map to JSON bytes,
// matching the on-the-wire shape UpsertKnowledgeAsset expects.
func (c CatalogFixture) PayloadJSON() json.RawMessage {
	if len(c.Payload) == 0 {
		return json.RawMessage(`{}`)
	}
	b, _ := json.Marshal(c.Payload)
	return b
}
