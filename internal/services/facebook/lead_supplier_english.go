package facebook

import (
	"regexp"
	"strings"
)

var englishSupplierFor = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:a|an|the)\s+)?(?:(?:european|chinese|china|eu|1688|taobao)\s+)?(?:(?:reliable|trusted|dropshipping|wholesale)\s+){0,2}supplier\s+for\s+(?:(?:this|the|a|an)\s+)?(?:(?:exact|same|similar)\s+)?([a-z][a-z0-9 -]+)`)
var englishProductNeed = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:this|the|a|an)\s+)?(?:(?:exact|same|similar)\s+)?([a-z][a-z0-9 -]+)`)
var englishSupplierIntent = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:a|an|the)\s+)?(?:[a-z-]+\s+){0,4}(?:supplier|agent)\b`)

// An unspecified supplier request deserves a short question, but cannot be
// turned into a marketplace search for an arbitrary product.
func asksForSupplierHelp(text string) bool {
	return englishSupplierIntent.MatchString(text)
}

// supplierEnglishQuery handles explicit English product requests without
// treating "looking for a supplier" as a product. It never uses the image
// alone to claim an exact model match.
func supplierEnglishQuery(raw string) string {
	text := raw
	if len(text) > 3000 {
		text = text[:3000]
	}
	var phrase string
	if match := englishSupplierFor.FindStringSubmatchIndex(text); len(match) >= 4 {
		phrase = text[match[2]:match[3]]
	} else if match := englishProductNeed.FindStringSubmatchIndex(text); len(match) >= 4 {
		phrase = text[match[2]:match[3]]
	}
	phrase = strings.ToLower(trimSupplierProperNameTail(phrase))
	for _, marker := range []string{" or ", " with ", " shipped", " shipping", " ship ", " to ", " from ", " in ", " for ", " model", " https", ".", ",", ";", "\n"} {
		if at := strings.Index(phrase, marker); at >= 0 {
			phrase = phrase[:at]
		}
	}
	phrase = strings.TrimSpace(phrase)
	phrase = trimSupplierQueryContext(phrase)
	words := strings.Fields(phrase)
	if len(words) > 0 {
		switch words[len(words)-1] {
		case "product", "products", "item", "items", "goods":
			return ""
		}
	}
	if strings.Contains(phrase, "supplier") || strings.Contains(phrase, "agent") || strings.Contains(phrase, "shipping") || asksForWarehouse(phrase) {
		return ""
	}
	if len(words) == 0 || len(words) > 10 || !safeSupplierQuery(phrase) {
		return ""
	}
	return strings.Join(words, " ")
}
