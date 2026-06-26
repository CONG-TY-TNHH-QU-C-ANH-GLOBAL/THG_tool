// Package governance implements the THREE governance layers
// mandated by goal G4:
//
//	Layer 1 — Retrieval filter      (filter unsafe assets at search time)
//	Layer 2 — Prompt constraint     (constrain what the LLM is told to do)
//	Layer 3 — Output validation     (validate generated text BEFORE send)
//
// THIS FILE implements Layer 3. Layers 1 + 2 live elsewhere:
//   - L1 is the banned_claim filter in retrieval/hybrid + retrieval/pgvector.
//   - L2 is the prompt-assembly format in workspace_knowledge/assembly
//     (sectioned PRODUCTS / POLICIES / CTAs blocks bound the LLM to
//     reasoning over retrieved facts rather than free-form synthesis).
//
// Layer 3 is the LAST LINE OF DEFENCE: even if the LLM hallucinates
// despite L1 + L2, the output validator rejects the message before it
// reaches Facebook / the customer.
//
// The validator answers ONE question per submission: "is this output
// safe to send?" It does NOT correct, rewrite, or score quality. It
// is a pure gate.
//
// Failure modes the validator MUST block (per goal G4):
//
//  1. Fabricated pricing — output claims a price not present in the
//     org's catalog. E.g. LLM invents "$15.99 wholesale" when the
//     real wholesale is $18.50.
//  2. Fake guarantees — "lifetime warranty", "money-back guarantee",
//     "best price guaranteed", unless those exact claims are an
//     approved asset in the org's knowledge base.
//  3. Hallucinated shipping claims — "free shipping worldwide", "next
//     day delivery", "30-day shipping" when the org's shipping policy
//     specifies different terms.
//  4. Hallucinated banned-claim phrases — direct match against the
//     org's banned_claim assets.
//
// Design contract:
//
//   - The validator takes the RETRIEVED ASSETS (the same set the LLM
//     saw) as ground truth. Any factual claim in the output that
//     doesn't trace back to an asset is suspect.
//   - The validator is CONSERVATIVE: it blocks when uncertain.
//     Production caller's recourse is to mark the message as draft
//     (human-review) rather than auto-send.
//   - The validator produces a structured Verdict so the Operator
//     Replay surface can show WHICH check failed and WHY.
//
// Each failure-mode check lives in its own file: banned_claim_check.go,
// guarantee_check.go, shipping_check.go, pricing_check.go. This file
// owns the verdict types and the ValidateOutput orchestrator only.
package governance

import (
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// ValidationVerdict is the output of [ValidateOutput]. It is
// safe-to-marshal and the Operator Replay UI renders it directly.
type ValidationVerdict struct {
	// Allow reports whether the output may be sent. False means the
	// caller MUST drop the message (or queue for human review). When
	// false, Reasons explains why.
	Allow bool `json:"allow"`

	// Reasons is the list of validator findings. Empty when Allow=true.
	// Each Reason carries a machine-readable Code so dashboards can
	// aggregate ("how many fabricated-pricing rejections this week?").
	Reasons []ValidationReason `json:"reasons,omitempty"`
}

// ValidationReason is one finding. Code maps to one of the
// constants below; Snippet captures up to 100 chars of the offending
// substring; Detail is operator-facing context.
type ValidationReason struct {
	Code    ValidationCode `json:"code"`
	Snippet string         `json:"snippet,omitempty"`
	Detail  string         `json:"detail,omitempty"`
}

// ValidationCode enumerates the failure taxonomies the validator
// reports. Stable across releases — dashboards aggregate on these.
type ValidationCode string

const (
	CodeFabricatedPricing ValidationCode = "fabricated_pricing"
	CodeFakeGuarantee     ValidationCode = "fake_guarantee"
	CodeHallucinatedShip  ValidationCode = "hallucinated_shipping"
	CodeBannedClaimMatch  ValidationCode = "banned_claim_match"
	CodeEmptyOutput       ValidationCode = "empty_output"
	CodeExcessiveLength   ValidationCode = "excessive_length"
)

// ValidateOutput is the Layer-3 governance check.
//
// generatedText is the LLM's draft output (comment, inbox message,
// post body). retrievedAssets is the set of assets the LLM saw —
// the validator uses these as ground truth.
//
// Returns Allow=true with no reasons when the output is safe.
// Returns Allow=false with structured reasons otherwise.
//
// This function is PURE. Same inputs always produce the same
// verdict — important for replay determinism.
func ValidateOutput(generatedText string, retrievedAssets []*assets.Asset) ValidationVerdict {
	v := ValidationVerdict{Allow: true}

	trimmed := strings.TrimSpace(generatedText)
	if trimmed == "" {
		v.Allow = false
		v.Reasons = append(v.Reasons, ValidationReason{
			Code:   CodeEmptyOutput,
			Detail: "generated output is empty or whitespace only",
		})
		return v
	}

	// Length guard. 1500 chars is well above any sane comment / DM;
	// beyond that we're probably looking at LLM run-on or smuggled
	// payload.
	if len(trimmed) > 1500 {
		v.Allow = false
		v.Reasons = append(v.Reasons, ValidationReason{
			Code:    CodeExcessiveLength,
			Detail:  "output exceeds 1500-character ceiling",
			Snippet: trimmed[:100] + "…",
		})
	}

	lower := strings.ToLower(trimmed)

	// Check 1: banned-claim direct match. If ANY of the org's
	// banned-claim assets appears literally in the output, hard fail.
	if reasons := bannedClaimReasons(lower, retrievedAssets); len(reasons) > 0 {
		v.Allow = false
		v.Reasons = append(v.Reasons, reasons...)
	}

	// Check 2: fake-guarantee phrases. These are NEVER safe unless an
	// approved asset says them verbatim. Conservative: block-by-default,
	// allow only if a matching approved asset exists.
	if findGuaranteePhrase(lower) {
		if !approvedAssetContains(retrievedAssets, lower, guaranteeWhitelistPhrases) {
			v.Allow = false
			v.Reasons = append(v.Reasons, ValidationReason{
				Code:   CodeFakeGuarantee,
				Detail: "output contains a guarantee phrase that is not backed by an approved asset",
			})
		}
	}

	// Check 3: hallucinated shipping claims. Same pattern — these are
	// VERY frequently fabricated by LLMs (especially "free shipping
	// worldwide") because they're plausible-sounding and the model
	// has no incentive to be conservative.
	if findShippingClaim(lower) {
		if !approvedAssetMentionsShipping(retrievedAssets, lower) {
			v.Allow = false
			v.Reasons = append(v.Reasons, ValidationReason{
				Code:   CodeHallucinatedShip,
				Detail: "output contains a shipping claim not supported by retrieved shipping_policy assets",
			})
		}
	}

	// Check 4: fabricated pricing. Look for dollar/percent/currency
	// figures in the output; assert each one appears in one of the
	// retrieved assets' price-bearing fields.
	if reasons := fabricatedPriceReasons(generatedText, retrievedAssets); len(reasons) > 0 {
		v.Allow = false
		v.Reasons = append(v.Reasons, reasons...)
	}

	return v
}
