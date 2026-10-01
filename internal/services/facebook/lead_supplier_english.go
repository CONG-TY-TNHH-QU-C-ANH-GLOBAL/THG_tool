package facebook

import (
	"regexp"
	"strings"
)

var englishSupplierFor = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:a|an|the)\s+)?(?:(?:european|chinese|china|eu|1688|taobao)\s+)?(?:(?:dropshipping|wholesale)\s+)?supplier\s+for\s+(?:(?:this|the|a|an)\s+)?(?:(?:exact|same|similar)\s+)?([a-z][a-z0-9 -]+)`)
var englishProductNeed = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:this|the|a|an)\s+)?(?:(?:exact|same|similar)\s+)?([a-z][a-z0-9 -]+)`)

// supplierEnglishQuery handles explicit English product requests without
// treating "looking for a supplier" as a product. It never uses the image
// alone to claim an exact model match.
func supplierEnglishQuery(raw string) string {
	text := strings.ToLower(raw)
	if len(text) > 3000 {
		text = text[:3000]
	}
	var phrase string
	if match := englishSupplierFor.FindStringSubmatch(text); len(match) > 1 {
		phrase = match[1]
	} else if match := englishProductNeed.FindStringSubmatch(text); len(match) > 1 {
		phrase = match[1]
	}
	for _, marker := range []string{" or ", " with ", " to ", " from ", " in ", " for ", " model", " https", ".", ",", ";", "\n"} {
		if at := strings.Index(phrase, marker); at >= 0 {
			phrase = phrase[:at]
		}
	}
	phrase = strings.TrimSpace(phrase)
	if strings.Contains(phrase, "supplier") || strings.Contains(phrase, "agent") || strings.Contains(phrase, "shipping") {
		return ""
	}
	words := strings.Fields(phrase)
	if len(words) < 2 || len(words) > 10 {
		return ""
	}
	return strings.Join(words, " ")
}
