package governance

import (
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

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
