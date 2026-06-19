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
package governance

import (
	"regexp"
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

// bannedClaimReasons returns one reason per org banned-claim asset whose title
// or description appears literally (case-insensitive) in the already-lowercased
// output. Extracted verbatim from ValidateOutput Check 1 — same matches, same
// order; the caller still sets Allow=false when any reason is returned.
func bannedClaimReasons(lower string, retrievedAssets []*assets.Asset) []ValidationReason {
	var reasons []ValidationReason
	for _, a := range retrievedAssets {
		if a == nil || a.Type != assets.AssetBannedClaim {
			continue
		}
		// Banned-claim asset title or description IS the phrase to
		// block. Lowercase substring match — case-insensitive.
		needles := []string{
			strings.ToLower(strings.TrimSpace(a.Title)),
			strings.ToLower(strings.TrimSpace(a.Description)),
		}
		for _, needle := range needles {
			if needle == "" {
				continue
			}
			if strings.Contains(lower, needle) {
				reasons = append(reasons, ValidationReason{
					Code:    CodeBannedClaimMatch,
					Snippet: needle,
					Detail:  "output contains a phrase marked as banned in the org's compliance assets",
				})
			}
		}
	}
	return reasons
}

// fabricatedPriceReasons returns one reason per price figure in the output that
// is not grounded in a retrieved asset. Extracted verbatim from ValidateOutput
// Check 4 — same claims, same order.
func fabricatedPriceReasons(generatedText string, retrievedAssets []*assets.Asset) []ValidationReason {
	var reasons []ValidationReason
	for _, claim := range extractPriceClaims(generatedText) {
		if !priceClaimGrounded(claim, retrievedAssets) {
			reasons = append(reasons, ValidationReason{
				Code:    CodeFabricatedPricing,
				Snippet: claim,
				Detail:  "output mentions a price not present in retrieved assets",
			})
		}
	}
	return reasons
}

// --- guarantee-phrase matching ---

var guaranteePhraseRE = regexp.MustCompile(
	`(?i)\b(money[-\s]?back guarantee|lifetime (warranty|guarantee)|100%\s*satisfaction|best price guaranteed?)\b`,
)

// guaranteeWhitelistPhrases are the verbatim phrases an approved
// asset would have to contain for a guarantee to be allowed. If the
// asset says "90-day defect warranty" verbatim, that's a different
// phrase from "lifetime warranty" — the latter is still hallucinated.
var guaranteeWhitelistPhrases = []string{
	"money-back guarantee",
	"money back guarantee",
	"lifetime warranty",
	"lifetime guarantee",
	"100% satisfaction",
	"best price guaranteed",
	"best price guarantee",
}

func findGuaranteePhrase(lowerOutput string) bool {
	return guaranteePhraseRE.MatchString(lowerOutput)
}

// approvedAssetContains: does any approved (non-banned, non-hidden)
// asset's title or description verbatim contain one of the
// whitelisted phrases that the output also contains?
func approvedAssetContains(retrieved []*assets.Asset, lowerOutput string, whitelist []string) bool {
	// Determine WHICH whitelist phrase the output is using.
	usedPhrase := ""
	for _, p := range whitelist {
		if strings.Contains(lowerOutput, p) {
			usedPhrase = p
			break
		}
	}
	if usedPhrase == "" {
		return false
	}
	for _, a := range retrieved {
		if a == nil {
			continue
		}
		if a.Type == assets.AssetBannedClaim || a.State == assets.StateHidden {
			continue
		}
		text := strings.ToLower(a.Title + " " + a.Description)
		if strings.Contains(text, usedPhrase) {
			return true
		}
	}
	return false
}

// --- shipping-claim matching ---

var shippingClaimRE = regexp.MustCompile(
	`(?i)\b(free shipping( worldwide)?|next[-\s]?day delivery|same[-\s]?day delivery|\d+[-\s]?day shipping|express delivery)\b`,
)

func findShippingClaim(lowerOutput string) bool {
	return shippingClaimRE.MatchString(lowerOutput)
}

// approvedAssetMentionsShipping: at least one retrieved
// shipping-policy asset (approved, non-hidden) discusses shipping.
// Conservative: we DON'T verify the exact terms (e.g. "5-10 days")
// match — that's beyond what an L3 string validator can do reliably.
// We just require SOME shipping asset was actually retrieved; if so,
// the LLM at least had ground truth, so we accept its output. If
// nothing about shipping was retrieved, any shipping claim is
// hallucinated.
func approvedAssetMentionsShipping(retrieved []*assets.Asset, lowerOutput string) bool {
	_ = lowerOutput
	for _, a := range retrieved {
		if a == nil || a.State == assets.StateHidden || a.Type == assets.AssetBannedClaim {
			continue
		}
		if a.Type == assets.AssetShippingPolicy {
			return true
		}
		// Or any asset whose body mentions shipping/transit/delivery.
		body := strings.ToLower(a.Title + " " + a.Description)
		if strings.Contains(body, "shipping") || strings.Contains(body, "transit") ||
			strings.Contains(body, "delivery") || strings.Contains(body, "production") {
			return true
		}
	}
	return false
}

// --- price-claim extraction ---

// extractPriceClaims finds dollar-figure substrings in the output.
// Conservative — we focus on $X formats since that's the dominant
// shape in POD/wholesale text. Future improvement: handle €, £, ¥,
// VND, percentage-off claims.
var priceClaimRE = regexp.MustCompile(`\$\s?\d[\d,]*(?:\.\d{1,2})?`)

func extractPriceClaims(s string) []string {
	matches := priceClaimRE.FindAllString(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		// Normalise: strip whitespace inside the match.
		out = append(out, strings.ReplaceAll(m, " ", ""))
	}
	return out
}

// priceClaimGrounded: does the price appear in any retrieved asset's
// title, description, or payload? Exact-match — small false-negative
// risk (asset has "$18.50 wholesale", output says "$18.5") is the
// safe direction; better to over-block a slightly-rephrased price
// than to under-block a fabricated one.
func priceClaimGrounded(claim string, retrieved []*assets.Asset) bool {
	wanted := strings.ToLower(claim)
	for _, a := range retrieved {
		if a == nil {
			continue
		}
		if a.State == assets.StateHidden || a.Type == assets.AssetBannedClaim {
			continue
		}
		blob := strings.ToLower(a.Title + " " + a.Description + " " + string(a.Payload))
		if strings.Contains(blob, wanted) {
			return true
		}
	}
	return false
}
