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
		{
			ExternalID:  "shopify_cat_tee_001",
			Type:        assets.AssetPODProduct,
			Title:       "Custom Cat Tee — Unisex Heavyweight",
			Description: "Premium ring-spun cotton, 6.1oz. POD with 7-day production.",
			Tags:        []string{"cat", "tee", "unisex", "heavyweight", "pod"},
			Payload:     map[string]any{"price": "$18.50 wholesale"},
		},
		{
			ExternalID:  "shopify_cat_mug_002",
			Type:        assets.AssetPODProduct,
			Title:       "Cat Lover Ceramic Mug 11oz",
			Description: "Dishwasher safe, full-wrap sublimation print.",
			Tags:        []string{"cat", "mug", "ceramic", "pet"},
			Payload:     map[string]any{"price": "$7.20 wholesale"},
		},
		{
			ExternalID:  "shopify_cat_hoodie_003",
			Type:        assets.AssetPODProduct,
			Title:       "Cat Mom Oversized Hoodie — Streetwear",
			Description: "50/50 blend, baggy fit. DTG print on heavyweight blanks.",
			Tags:        []string{"cat", "hoodie", "oversized", "streetwear", "mom"},
			Payload:     map[string]any{"price": "$22.00 wholesale"},
		},
		// --- Dog-niche products ---
		{
			ExternalID:  "shopify_dog_hoodie_004",
			Type:        assets.AssetPODProduct,
			Title:       "Custom Dog Dad Pullover Hoodie",
			Description: "Soft-hand DTG print on midweight cotton-poly hoodie.",
			Tags:        []string{"dog", "hoodie", "dad", "pullover", "pod"},
			Payload:     map[string]any{"price": "$22.00 wholesale"},
		},
		{
			ExternalID:  "shopify_dog_bandana_005",
			Type:        assets.AssetPODProduct,
			Title:       "Personalised Dog Bandana — Cotton Twill",
			Description: "Custom pet name + tag-friendly cotton twill bandana.",
			Tags:        []string{"dog", "bandana", "pet", "personalised"},
			Payload:     map[string]any{"price": "$6.50 wholesale"},
		},
		// --- Aesthetic / niche-style products ---
		{
			ExternalID:  "csv_anime_tee_006",
			Type:        assets.AssetPODProduct,
			Title:       "Anime Gothic Oversized Tee — Black",
			Description: "Dark anime-inspired streetwear; oversized fit, edgy designs.",
			Tags:        []string{"anime", "gothic", "oversized", "tee", "streetwear"},
			Payload:     map[string]any{"price": "$24.00 wholesale"},
		},
		{
			ExternalID:  "csv_kawaii_hoodie_007",
			Type:        assets.AssetPODProduct,
			Title:       "Kawaii Pastel Hoodie — Y2K Aesthetic",
			Description: "Soft pastel colourways, Japanese-inspired graphics.",
			Tags:        []string{"anime", "kawaii", "hoodie", "pastel"},
			Payload:     map[string]any{"price": "$26.00 wholesale"},
		},
		// --- Shipping / policy assets ---
		{
			ExternalID:  "notion_shipping_sla_008",
			Type:        assets.AssetShippingPolicy,
			Title:       "Shipping & Production SLA",
			Description: "Production 3-7 days. US transit 5-10 days. EU transit 8-14 days. Tracking included.",
			Tags:        []string{"shipping", "sla", "us", "eu", "fulfillment"},
		},
		{
			ExternalID:  "notion_returns_009",
			Type:        assets.AssetShippingPolicy,
			Title:       "Returns & Refunds Policy v3",
			Description: "90-day defect warranty. POD products are made-to-order; non-defect returns not accepted.",
			Tags:        []string{"returns", "refunds", "policy", "warranty"},
		},
		{
			ExternalID:  "sheet_pricing_010",
			Type:        assets.AssetPricingRule,
			Title:       "Wholesale Pricing Tier",
			Description: "MOQ 20 = 5% off. MOQ 100 = 15% off. MOQ 500 = 25% off plus dedicated rep.",
			Tags:        []string{"pricing", "wholesale", "moq", "tier"},
		},
		// --- CTAs ---
		{
			ExternalID:  "notion_cta_vi_dm_011",
			Type:        assets.AssetCTA,
			Title:       "Soft CTA — Vietnamese DM Invite",
			Description: "Inbox mình nhé, mình gửi catalog chi tiết và giá sỉ riêng.",
			Tags:        []string{"cta", "vi", "inbox", "dm"},
		},
		{
			ExternalID:  "notion_cta_en_dm_012",
			Type:        assets.AssetCTA,
			Title:       "Soft CTA — English DM Invite",
			Description: "DM me for the full catalog and wholesale pricing.",
			Tags:        []string{"cta", "en", "dm"},
		},
		// --- Compliance asset — banned claim ---
		// Banned: must NEVER surface in retrieval results regardless
		// of query. The soak verifies this.
		{
			ExternalID:  "compliance_banned_001",
			Type:        assets.AssetBannedClaim,
			Title:       "Banned: best price guaranteed",
			Description: "Cannot prove. FTC Section 5 risk. Block in all outbound text.",
			Tags:        []string{"compliance", "banned", "guaranteed"},
		},
		// --- Operator-pinned CTA ---
		// This CTA is pinned + zero-score on most queries — soak
		// must surface it via the pin path, not the semantic path.
		{
			ExternalID:  "notion_pinned_cta_013",
			Type:        assets.AssetCTA,
			Title:       "PINNED CTA — Wholesale enquiry",
			Description: "For wholesale enquiries, message us with your MOQ + target market.",
			Tags:        []string{"cta", "wholesale", "pinned"},
			Pinned:      true,
		},
		// --- Boosted seasonal product ---
		{
			ExternalID:  "csv_seasonal_014",
			Type:        assets.AssetPODProduct,
			Title:       "Christmas Cat Sweater — Pre-order",
			Description: "Q4 seasonal. 45-day lead time. Pre-orders open Aug.",
			Tags:        []string{"cat", "seasonal", "christmas", "preorder"},
			Payload:     map[string]any{"price": "$21.00 wholesale"},
			Boost:       60,
		},
		// --- Hidden asset (must never appear in approved-only results) ---
		{
			ExternalID:  "csv_hidden_015",
			Type:        assets.AssetPODProduct,
			Title:       "Generic Plain Tote — Low Margin",
			Description: "No design. Considered low-margin filler SKU.",
			Tags:        []string{"tote", "plain"},
			State:       assets.StateHidden,
		},
		// --- Pending asset (not yet approved by operator) ---
		{
			ExternalID:  "csv_pending_016",
			Type:        assets.AssetPODProduct,
			Title:       "Halloween Bat Tee — Awaiting approval",
			Description: "Operator has not reviewed this CSV row yet.",
			Tags:        []string{"halloween", "tee", "seasonal"},
			State:       assets.StatePending,
		},
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
