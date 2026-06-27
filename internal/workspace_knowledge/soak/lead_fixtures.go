package soak

// LeadPrompt is one realistic lead the harness will query against.
// IntentTag is the GROUND TRUTH — which clusters the prompt SHOULD
// activate. The soak harness uses it to compute precision proxy:
// "did retrieval surface assets that share clusters with the
// intent?". Without ground truth, quality metrics are meaningless.
type LeadPrompt struct {
	Lang       string   // "vi" or "en"
	Text       string   // the lead body the LLM would see in production
	IntentTags []string // expected cluster activations (e.g. ["cat", "shirt", "pod"])
	// MinScore is the lowest acceptable best-hit score for this
	// prompt. Soak fails when actual best score < MinScore. Set
	// conservatively (0.30) — anything below means retrieval missed.
	MinScore float64
}

// RealisticLeads returns lead prompts with intent ground-truth. The
// soak harness queries each prompt and measures precision @ k=5
// against the IntentTags — what fraction of returned assets share
// at least one expected cluster.
//
// Mix of:
//   - Direct keyword matches (should be easy)
//   - Synonym / cross-token matches (where semantic should shine —
//     ClusteredEmbedder simulates this via cluster overlap)
//   - Vietnamese prompts (UTF-8 + diacritic-folded matching)
//   - Out-of-niche prompts (should produce empty / low-confidence)
//   - Compliance-risk prompts (banned claim must NOT surface)
func RealisticLeads() []LeadPrompt {
	return []LeadPrompt{
		// Direct-match prompts
		{
			Lang:       "en",
			Text:       "Looking for custom cat tee POD with US shipping",
			IntentTags: []string{"cat", "shirt", "pod", "us", "fulfillment"},
			MinScore:   0.30,
		},
		{
			Lang:       "en",
			Text:       "Need dog hoodie supplier MOQ 50",
			IntentTags: []string{"dog", "hoodie", "wholesale"},
			MinScore:   0.30,
		},
		// Semantic / synonym prompts — these are where ClusteredEmbedder
		// shines: "anime gothic" maps to cluster anime+gothic; "edgy
		// streetwear" assets also activate gothic via term overlap.
		{
			Lang:       "en",
			Text:       "Need supplier for oversized gothic anime tees in US wholesale",
			IntentTags: []string{"anime", "gothic", "oversized", "shirt", "us", "wholesale"},
			MinScore:   0.30,
		},
		{
			Lang:       "en",
			Text:       "Looking for kawaii pastel hoodies for streetwear brand",
			IntentTags: []string{"anime", "hoodie"},
			MinScore:   0.30,
		},
		// Vietnamese prompts
		{
			Lang:       "vi",
			Text:       "Cần fulfill áo thun mèo cho thị trường Mỹ, giá sỉ",
			IntentTags: []string{"cat", "shirt", "fulfillment", "us", "wholesale"},
			MinScore:   0.25,
		},
		{
			Lang:       "vi",
			Text:       "Tìm xưởng POD chó hoodie cho team marketing US",
			IntentTags: []string{"dog", "hoodie", "pod", "us"},
			MinScore:   0.25,
		},
		// Policy-seeking prompts
		{
			Lang:       "en",
			Text:       "What is your shipping time to Germany and returns policy?",
			IntentTags: []string{"shipping", "policy", "eu"},
			MinScore:   0.20,
		},
		// Pure CTA-trigger prompt (low keyword overlap, soaks pin/boost path)
		{
			Lang:       "en",
			Text:       "Interested in wholesale enquiry",
			IntentTags: []string{"wholesale", "cta"},
			MinScore:   0.15, // looser — pin should surface even without text overlap
		},
		// Compliance-risk prompt: the banned claim MUST NOT surface
		// in any retrieval result.
		//
		// IntentTags is intentionally EMPTY: no approved asset in
		// the catalog can legitimately answer this prompt — the only
		// "best price guaranteed" asset is banned and correctly
		// filtered upstream. The soak's success criterion is therefore
		// SAFETY (no leak) rather than relevance. Surfacing unrelated
		// assets is tolerable; surfacing the banned one is a hard
		// fail caught by the ComplianceLeaks assertion.
		{
			Lang:       "en",
			Text:       "Are your products best price guaranteed?",
			IntentTags: []string{},
			MinScore:   0.0,
		},
		// Out-of-domain prompt: should produce near-empty result.
		// Validates that retrieval doesn't hallucinate matches.
		{
			Lang:       "en",
			Text:       "Recommend a good Italian restaurant in Hanoi",
			IntentTags: []string{}, // no expected cluster
			MinScore:   0.0,
		},
	}
}
