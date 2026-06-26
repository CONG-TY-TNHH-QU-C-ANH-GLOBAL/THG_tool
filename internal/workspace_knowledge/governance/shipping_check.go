package governance

import (
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

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
