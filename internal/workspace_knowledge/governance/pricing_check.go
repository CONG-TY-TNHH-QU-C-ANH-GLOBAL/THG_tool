package governance

import (
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

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
