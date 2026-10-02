package facebook

import (
	"regexp"
	"strings"
)

var supplierEmailOrPhone = regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}|\+?\d[\d .-]{6,}\d`)
var supplierTrailingQuantity = regexp.MustCompile(`(?i)\s+\d{1,6}\s*(?:hộp|cái|chiếc|sản phẩm|pcs|units|boxes|pieces)(?:/tháng|/month|\b)`)

// Keep only product words before delivery instructions, sales copy or contact
// information. The returned text is the only text sent as an Elim search term.
func trimSupplierQueryContext(raw string) string {
	text := strings.TrimSpace(strings.ToLower(raw))
	cut := len(text)
	for _, marker := range []string{
		" liên hệ", " sđt", " số điện thoại", " zalo", " inbox", " ib ",
		" chị ", " anh ", " email", " phone", " contact", " gửi ", " giao ",
		" tới mỹ", " đến mỹ", " về mỹ", " sang mỹ", " đi mỹ",
		" to the us", " to us", " to the usa", " to usa", " to the uk", " to uk", " to united states",
		" loại ", " giá ", " khoảng ", " số lượng", " https://", " http://",
		" www.", "@", "\n", ",", ";",
	} {
		if at := strings.Index(text, marker); at >= 0 && at < cut {
			cut = at
		}
	}
	for _, pattern := range []*regexp.Regexp{supplierEmailOrPhone, supplierTrailingQuantity} {
		if at := pattern.FindStringIndex(text); at != nil && at[0] < cut {
			cut = at[0]
		}
	}
	return strings.TrimSpace(text[:cut])
}

func safeSupplierQuery(text string) bool {
	if text == "" || strings.ContainsAny(text, "@:/\\") || supplierEmailOrPhone.MatchString(text) {
		return false
	}
	words := supplierIdentityWords(text)
	if len(words) >= 2 {
		return true
	}
	for word := range words {
		return leadProductSpecificTerms[word]
	}
	return false
}
