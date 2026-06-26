package governance

import (
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

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
