package facebook

import (
	"regexp"
	"strings"
)

var englishSupplierFor = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:a|an|the)\s+)?(?:(?:european|chinese|china|eu|1688|taobao)\s+)?(?:(?:reliable|trusted|dropshipping|wholesale)\s+){0,2}(?:suppliers?|manufacturers?|vendors?|factory|factories|wholesalers?)\s+(?:for|of)\s+(?:(?:this|the|a|an)\s+)?(?:(?:exact|same|similar)\s+)?([a-z][a-z0-9 -]+)`)
var englishProductNeed = regexp.MustCompile(`(?i)\b(?:looking\s+for|need|seeking|sourcing)\s+(?:(?:this|the|a|an)\s+)?(?:(?:exact|same|similar)\s+)?([a-z][a-z0-9 -]+)`)
var englishSupplierIntent = regexp.MustCompile(`(?i)\b(?:looking\s+for|needs?|seeking|searching\s+for|sourcing|anyone\s+(?:know|have|recommend)|recommend)\s+(?:(?:a|an|the|any)\s+)?(?:[a-z-]+\s+){0,4}(?:suppliers?|agents?|manufacturers?|vendors?|factory|factories|wholesalers?)\b`)
var englishSupplierNeeded = regexp.MustCompile(`(?i)\b(?:suppliers?|manufacturers?|vendors?|sourcing\s+agents?)\s+needed\b`)

// An unspecified supplier request deserves a short question, but cannot be
// turned into a marketplace search for an arbitrary product.
func asksForSupplierHelp(text string) bool {
	return englishSupplierIntent.MatchString(text) || englishSupplierNeeded.MatchString(text)
}

// Words naming who supplies rather than what is supplied.
var supplierRoleWords = []string{"supplier", "agent", "shipping", "vendor", "manufacturer", "factor", "wholesaler", "forwarder", "freight", "courier", "logistics"}

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
	for _, role := range supplierRoleWords {
		if strings.Contains(phrase, role) {
			return ""
		}
	}
	if asksForWarehouse(phrase) {
		return ""
	}
	if len(words) == 0 || len(words) > 10 || !safeSupplierQuery(phrase) {
		return ""
	}
	return strings.Join(words, " ")
}
