package facebook

import "unicode"

// leadLooksEnglish reports a post written without Vietnamese or other
// non-ASCII letters, so a draft that cannot extract a product phrase still
// answers in the post's language.
func leadLooksEnglish(text string) bool {
	words := 0
	inWord := false
	for _, r := range text {
		if unicode.IsLetter(r) && r > unicode.MaxASCII {
			return false
		}
		isASCIILetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if isASCIILetter && !inWord {
			words++
		}
		inWord = isASCIILetter
	}
	return words >= 3
}

// asciiQuery reports a search phrase that is English even inside a Vietnamese
// post ("cần nhập hand massager"), so Elim returns titles in the same language.
func asciiQuery(query string) bool {
	if query == "" {
		return false
	}
	for _, r := range query {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// asksForWarehouse marks storage or fulfillment requests. They are not
// product sourcing requests even when they mention 1688 or bulk volume.
func asksForWarehouse(text string) bool {
	for _, term := range []string{"kho", "warehouse", "3pl", "fulfillment", "fulfilment", "fulfill", "storage"} {
		if containsLeadPhrase(text, term) {
			return true
		}
	}
	return false
}
