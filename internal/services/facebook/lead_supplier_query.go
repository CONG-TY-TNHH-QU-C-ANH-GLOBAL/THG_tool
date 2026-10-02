package facebook

import (
	"regexp"
	"strings"
	"unicode"
)

var supplierLeadingNumber = regexp.MustCompile(`^\d{1,6}\s+`)
var supplierLeadingQuantity = regexp.MustCompile(`^\d{1,6}\s*(hộp|cái|chiếc|sản phẩm|pcs|units|đôi|boxes|pieces)(?:\s+|$)`)

func supplierQueryLanguage(text string) string {
	if supplierEnglishQuery(text) != "" {
		return "en"
	}
	for _, phrase := range []string{"print logo", "logo printing", "custom logo", "custom print"} {
		if containsLeadPhrase(text, phrase) {
			return "en"
		}
	}
	if leadLooksEnglish(text) {
		return "en"
	}
	return "vi"
}

// supplierQuery extracts a product phrase from buying or POD requests. Vague
// logistics posts have no query, so they never consume marketplace quota.
func supplierQuery(raw string) string {
	text := strings.ToLower(raw)
	if len([]rune(text)) > 1500 {
		text = string([]rune(text)[:1500])
	}
	for _, marker := range []string{"cần nhập ", "muốn nhập ", "cần tìm nguồn ", "cần nguồn ", "tìm nguồn ", "cần mua ", "muốn mua ", "nhập ", "mua "} {
		if at := strings.Index(text, marker); at >= 0 {
			// Preserve case until a possible personal-name tail has been removed.
			text = strings.ToLower(trimSupplierProperNameTail(raw[at+len(marker):]))
			goto cut
		}
	}
	if query := podSupplierQuery(raw); query != "" {
		return query
	}
	return supplierEnglishQuery(raw)
cut:
	for _, marker := range []string{" từ 1688", " từ taobao", " về mỹ", " sang mỹ", " đi mỹ", " ship ", " khoảng ", " số lượng ", " in logo", " in theo", " mẫu này", " https://", " http://", "\n", ".", ",", ";"} {
		if at := strings.Index(text, marker); at >= 0 {
			text = text[:at]
		}
	}
	text = strings.TrimSpace(text)
	text = trimSupplierQueryContext(text)
	text = supplierLeadingQuantity.ReplaceAllString(text, "")
	text = supplierLeadingNumber.ReplaceAllString(text, "")
	for _, prefix := range []string{"một số lượng lớn ", "số lượng lớn ", "sản phẩm ", "hàng "} {
		text = strings.TrimPrefix(text, prefix)
	}
	words := strings.Fields(text)
	if len(words) == 0 || len(words) > 12 || !safeSupplierQuery(text) {
		return ""
	}
	return strings.Join(words, " ")
}

// podSupplierQuery searches for a named blank product when a printing request
// has no catalog match. An unspecified design or product never triggers Elim.
func podSupplierQuery(raw string) string {
	if !wantsPersonalizedPOD(raw) {
		return ""
	}
	text := strings.ToLower(raw)
	for _, marker := range []string{"logo lên ", "logo trên ", "thiết kế lên ", "yêu cầu lên ", "logo on ", "printing on ", "print on "} {
		if at := strings.Index(text, marker); at >= 0 {
			return cleanPODSupplierPhrase(strings.ToLower(trimSupplierProperNameTail(raw[at+len(marker):])))
		}
	}
	for _, marker := range []string{" in logo", " in theo thiết kế", " in theo yêu cầu", " with custom logo"} {
		if at := strings.Index(text, marker); at >= 0 {
			return cleanPODSupplierPhrase(strings.ToLower(trimSupplierProperNameTail(raw[:at])))
		}
	}
	return ""
}

func cleanPODSupplierPhrase(text string) string {
	for _, marker := range []string{" đi mỹ", " sang mỹ", " về mỹ", " đi nước", " ship ", " từ ", " số lượng", " khoảng ", " theo ", " để ", " https://", "\n", ".", ",", ";"} {
		if at := strings.Index(text, marker); at >= 0 {
			text = text[:at]
		}
	}
	text = strings.TrimSpace(text)
	text = trimSupplierQueryContext(text)
	for _, prefix := range []string{"cần ", "muốn ", "tìm ", "lên ", "trên ", "cho "} {
		text = strings.TrimPrefix(text, prefix)
	}
	text = supplierLeadingQuantity.ReplaceAllString(text, "")
	text = supplierLeadingNumber.ReplaceAllString(text, "")
	words := strings.Fields(text)
	if len(words) == 0 || len(words) > 8 {
		return ""
	}
	query := strings.Join(words, " ")
	if !safeSupplierQuery(query) {
		return ""
	}
	return query
}

// A marketplace title must retain the phrase's first and last specific words
// and cover at least 80% of its identity words. This tolerates minor listing
// wording changes while protecting qualifiers such as the target animal.
func matchesSupplierQuery(query, title string) bool {
	need := supplierIdentitySequence(query)
	if len(need) == 0 {
		return false
	}
	if len(need) == 1 {
		if !leadProductSpecificTerms[need[0]] {
			return false
		}
	}
	have := supplierIdentityWords(title)
	if !have[need[0]] || !have[need[len(need)-1]] {
		return false
	}
	matched := 0
	for _, word := range need {
		if have[word] {
			matched++
		}
	}
	return matched*5 >= len(need)*4
}

func supplierIdentityWords(text string) map[string]bool {
	words := make(map[string]bool)
	for _, word := range supplierIdentitySequence(text) {
		words[word] = true
	}
	return words
}

func supplierIdentitySequence(text string) []string {
	words := make([]string, 0, 8)
	seen := make(map[string]bool)
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		word = singularProductWord(word)
		switch word {
		case "massager", "massage":
			word = "massage"
		case "handheld":
			word = "hand"
		}
		if len([]rune(word)) >= 2 && !leadProductStopWords[word] && word != "viên" && word != "mẫu" && !allDigits(word) && !seen[word] {
			words = append(words, word)
			seen[word] = true
		}
	}
	return words
}

func allDigits(text string) bool {
	for _, r := range text {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
