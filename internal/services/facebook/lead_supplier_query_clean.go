package facebook

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var supplierEmailOrPhone = regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}|\+?\d[\d .-]{6,}\d`)
var supplierTrailingQuantity = regexp.MustCompile(`(?i)\s+\d{1,6}\s*(?:hộp|cái|chiếc|sản phẩm|pcs|units|boxes|pieces)(?:/tháng|/month|\b)`)
var commonVietnameseSurname = map[string]bool{
	"nguyễn": true, "trần": true, "lê": true, "phạm": true, "hoàng": true,
	"vũ": true, "võ": true, "đặng": true, "bùi": true, "đỗ": true,
	"hồ": true, "ngô": true, "dương": true, "đinh": true,
}
var supplierContextProperWords = map[string]bool{
	"mỹ": true, "us": true, "usa": true, "uk": true,
	"taobao": true, "tmall": true, "alibaba": true, "china": true,
	"europe": true, "european": true, "canada": true,
}

// Keep a product phrase when a customer adds a capitalized personal name after
// it. This reduces accidental disclosure; ambiguous lower-case text still
// needs a separate fail-closed check before calling the external API.
func trimSupplierProperNameTail(raw string) string {
	words := strings.Fields(raw)
	for i, word := range words {
		if i == 0 || !startsWithUppercaseLetter(word) {
			continue
		}
		if supplierContextProperWords[strings.ToLower(word)] {
			continue
		}
		capitalizedNext := i+1 < len(words) && startsWithUppercaseLetter(words[i+1])
		precededByLowercase := false
		for _, prior := range words[:i] {
			r, _ := utf8.DecodeRuneInString(prior)
			if unicode.IsLower(r) {
				precededByLowercase = true
				break
			}
		}
		if commonVietnameseSurname[strings.ToLower(word)] || (capitalizedNext && i >= 2 && precededByLowercase) || (i >= 3 && precededByLowercase) {
			return strings.Join(words[:i], " ")
		}
	}
	return raw
}

func startsWithUppercaseLetter(word string) bool {
	for _, r := range word {
		if unicode.IsLetter(r) {
			return unicode.IsUpper(r)
		}
	}
	return false
}

// Keep only product words before delivery instructions, sales copy or contact
// information. The returned text is the only text sent as an Elim search term.
func trimSupplierQueryContext(raw string) string {
	text := strings.TrimSpace(strings.ToLower(raw))
	cut := len(text)
	// The trailing space lets word markers such as " gửi " also match the last
	// word, e.g. "túi vải canvas gửi" left after "về mỹ" was removed upstream.
	padded := text + " "
	for _, marker := range []string{
		" liên hệ", " sđt", " số điện thoại", " zalo", " inbox", " ib ",
		" chị ", " anh ", " em ", " nhé ", " email", " phone", " contact", " gửi ", " giao ",
		" địa chỉ", " đ/c", " đc ", " quận ", " phường ", " huyện ", " đường ", " facebook", " fb ",
		" tới mỹ", " đến mỹ", " về mỹ", " sang mỹ", " đi mỹ",
		" to the us", " to us", " to the usa", " to usa", " to the uk", " to uk", " to united states",
		" loại ", " giá ", " khoảng ", " số lượng", " https://", " http://",
		" www.", "@", "\n", ",", ";",
	} {
		if at := strings.Index(padded, marker); at >= 0 && at < cut {
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
	for _, word := range strings.Fields(strings.ToLower(text)) {
		if commonVietnameseSurname[word] {
			return false
		}
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
