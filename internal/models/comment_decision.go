package models

// Knowledge Intelligence Layer contracts (P2a — specs/COMMENT_INTELLIGENCE_PIPELINE.md
// §4). These are DOMAIN CONTRACTS (feedback_contracts_not_orm), not DB rows: the
// agent reasons over retrieved knowledge and emits a CommentDecision whose every
// concrete claim is GROUNDED in a real ingested asset. There is no Service-Offer /
// Proof / CTA authored type — capabilities/offers/proofs are reasoning OUTPUTS.

// Lead intent — the closed, domain-agnostic enum the reasoning layer assigns.
const (
	IntentServiceSeeking = "service_seeking"
	IntentProductSeeking = "product_seeking"
	IntentAmbiguous      = "ambiguous"
	IntentNonLead        = "non_lead"
)

// KnowledgeCandidate is one retrieved, grounding-eligible piece of knowledge
// (a KnowledgeOS asset or catalog SKU) produced by retrieval. It is the ONLY
// material the agent may select from — a selection that does not match a
// candidate by AssetID/SKU is an invented claim and is dropped.
type KnowledgeCandidate struct {
	AssetID   int64   `json:"asset_id"`
	SKU       string  `json:"sku,omitempty"`
	Kind      string  `json:"kind"` // asset type: POD_product | sales_playbook | faq | pricing_rule | cta | ...
	Title     string  `json:"title"`
	Summary   string  `json:"summary,omitempty"`
	PriceText string  `json:"price_text,omitempty"`
	ImageURL  string  `json:"image_url,omitempty"`
	Score     float64 `json:"score"`
}

// GroundedItem is the no-fabrication unit: an agent claim that points at a real
// source. SourceAssetID>0 OR SKU!="" is REQUIRED — an item with neither would be
// an invented claim and is never produced by grounding.
type GroundedItem struct {
	Label         string  `json:"label"`           // the agent's phrasing
	SourceAssetID int64   `json:"source_asset_id"` // the KnowledgeOS asset it is grounded in
	SKU           string  `json:"sku,omitempty"`   // catalog SKU when grounded in a product
	PriceText     string  `json:"price_text,omitempty"`
	ImageURL      string  `json:"image_url,omitempty"` // from the cited asset; never generated
	Score         float64 `json:"score"`
}

// Selection is the agent's grounded choices for one lead, split by role. Each
// slice contains only items that survived grounding.
type Selection struct {
	Capabilities []GroundedItem `json:"capabilities,omitempty"`
	Products     []GroundedItem `json:"products,omitempty"`
	Proofs       []GroundedItem `json:"proofs,omitempty"`
	CTA          *GroundedItem  `json:"cta,omitempty"`
}

// HasOffer reports whether any substantive offer survived grounding (a CTA alone
// is not an offer). Drives KnowledgeGap.
func (s Selection) HasOffer() bool {
	return len(s.Capabilities) > 0 || len(s.Products) > 0 || len(s.Proofs) > 0
}

// CommentDecision is the agent's explainable, grounded decision for one lead.
// It is produced by the Reasoning Layer and (in later phases) consumed by the
// comment generator, the Policy Gate, and the Agent Decision Inspector.
type CommentDecision struct {
	Intent       string    `json:"intent"`
	Confidence   float64   `json:"confidence"` // [0,1]
	Reasoning    string    `json:"reasoning"`  // short human-readable WHY
	Selected     Selection `json:"selected"`
	KnowledgeGap bool      `json:"knowledge_gap"` // true when nothing substantive could be grounded
	RetrievalID  string    `json:"retrieval_id,omitempty"`
}
