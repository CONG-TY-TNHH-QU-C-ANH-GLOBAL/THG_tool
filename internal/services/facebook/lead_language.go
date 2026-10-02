package facebook

import (
	"strings"
	"unicode"
)

// leadLooksEnglish reports a post written without Vietnamese or other
// non-ASCII letters, so a draft that cannot extract a product phrase still
// answers in the post's language.
func leadLooksEnglish(text string) bool {
	if looksLikeUnaccentedVietnamese(text) {
		return false
	}
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

// ASCII alone does not mean English: Facebook posts often omit Vietnamese
// accents. Use only phrases that are distinctive in this sales context.
func looksLikeUnaccentedVietnamese(text string) bool {
	for _, phrase := range []string{"can nhap", "muon nhap", "can mua", "muon mua", "tim nguon", "so luong", "bao gia", "gui ve", "ben minh", "cho minh", "nhap hang", "ao hoodie", "ao thun"} {
		if containsLeadPhrase(text, phrase) {
			return true
		}
	}
	return false
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
	return !looksLikeUnaccentedVietnamese(query) && !strings.HasPrefix(strings.ToLower(query), "ao ")
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
