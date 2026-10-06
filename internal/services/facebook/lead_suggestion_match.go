package facebook

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// A retrieval hit can be broadly relevant to the workspace without matching the
// item in this post. Require a product term from the post before offering it.
func matchesLeadProduct(leadText, title string) bool {
	if sharedProductHeadPhrase(leadText, title) {
		return true
	}
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, word := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
			if leadProductStopWords[word] {
				continue
			}
			word = singularProductWord(word)
			if len([]rune(word)) >= 3 && !leadProductStopWords[word] {
				out[word] = true
			}
		}
		return out
	}
	post := words(leadText)
	matches := 0
	for word := range words(title) {
		if post[word] {
			matches++
			if matches >= 2 || leadProductSpecificTerms[word] {
				return true
			}
		}
	}
	return false
}

// Singularize common English plurals before comparing product names. This is
// deliberately narrow: stemming arbitrary words can create false matches.
func singularProductWord(word string) string {
	if strings.HasSuffix(word, "sses") || strings.HasSuffix(word, "xes") {
		return strings.TrimSuffix(word, "es")
	}
	if len(word) > 4 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
		return strings.TrimSuffix(word, "s")
	}
	return word
}

var leadProductSpecificTerms = map[string]bool{
	"hoodie": true, "stocking": true, "tshirt": true,
	"sweatshirt": true, "shirt": true, "thun": true,
}

var leadProductStopWords = map[string]bool{
	"sản": true, "phẩm": true, "hàng": true, "cần": true, "tìm": true,
	"giá": true, "cho": true, "với": true, "của": true, "bên": true,
	"mình": true, "shop": true, "bán": true, "đơn": true, "ship": true,
	"the": true, "and": true, "for": true, "product": true, "new": true,
	"personalized": true, "personalised": true, "custom": true,
	"christmas": true, "xmas": true, "etsy": true, "logo": true,
	"design": true, "cotton": true, "unisex": true, "premium": true,
	// Grade and gift qualifiers ("cao cấp", "quà tặng") are shared by unrelated items.
	"cao": true, "cấp": true, "quà": true, "tặng": true,
}

// Match whole Unicode words so "pod" cannot match "tripod", and "áo"
// cannot match "táo". Punctuation (including arrows and slashes) separates words.
func containsLeadPhrase(text, phrase string) bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	needle := strings.FieldsFunc(strings.ToLower(phrase), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for start := 0; start+len(needle) <= len(words); start++ {
		matched := true
		for i, word := range needle {
			if words[start+i] != word {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func matchingCandidates(leadText string, candidates []models.KnowledgeCandidate) []models.KnowledgeCandidate {
	matched := make([]models.KnowledgeCandidate, 0, len(candidates))
	supplierPhrase := supplierQuery(leadText)
	for _, candidate := range candidates {
		if candidate.Kind == string(assets.AssetSupplierProduct) && supplierPhrase != "" {
			if matchesSupplierQuery(supplierPhrase, candidate.Title) {
				matched = append(matched, candidate)
			}
			continue
		}
		if matchesLeadProduct(leadText, candidate.Title) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

// The post's buying intent decides which business path to offer. A matching
// catalog title alone does not make a bulk sourcing request a POD request.
func wantsBulkSourcing(text string) bool {
	text = strings.ToLower(text)
	if bulkQuantityPattern.MatchString(text) {
		return true
	}
	for _, phrase := range []string{"số lượng lớn", "sll", "nhập hàng", "lấy sỉ", "mua sỉ", "nguồn hàng", "giá sỉ", "1688", "taobao", "hộp/tháng", "cái/tháng", "sp/tháng", "dropshipping supplier", "dropshipping agent", "wholesale supplier", "bulk order"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

var bulkQuantityPattern = regexp.MustCompile(`\d{2,}\s*(hộp|cái|chiếc|sản phẩm|pcs|units|đôi|áo|shirts|hoodies)`)

func wantsPersonalizedPOD(text string) bool {
	for _, phrase := range []string{"in theo logo", "in logo", "in theo thiết kế", "in theo yêu cầu", "cá nhân hóa", "custom logo", "print logo", "logo printing", "custom print", "personalized", "print on demand", "pod"} {
		if containsLeadPhrase(text, phrase) {
			return true
		}
	}
	return false
}
func isUSDestination(text string) bool {
	if explicitUSDestination.MatchString(text) {
		return true
	}
	for _, term := range []string{"sang mỹ", "về mỹ", "đi mỹ", "tại mỹ", "qua mỹ", "giao tới mỹ", "gửi mỹ", "gửi đến mỹ", "gửi sang mỹ", "to usa", "to the usa", "to united states", "to the united states", " hoa kỳ", " u.s."} {
		if containsLeadPhrase(text, term) {
			return true
		}
	}
	return false
}

var explicitUSDestination = regexp.MustCompile(`\b(?:to|ship to|deliver to)\s+(?:the\s+)?US\b`)

var leadQuantityPattern = regexp.MustCompile(`(\d{1,6})\s*(hộp|cái|chiếc|sản phẩm|pcs|units|đôi|boxes|pieces|áo|shirts|hoodies)`)

func leadQuantity(text string) int {
	if match := leadQuantityPattern.FindStringSubmatch(strings.ToLower(text)); len(match) > 1 {
		quantity, err := strconv.Atoi(match[1])
		if err == nil && quantity > 0 {
			return quantity
		}
	}
	for _, phrase := range []string{"một hộp", "một cái", "một chiếc", "one box", "one piece"} {
		if strings.Contains(strings.ToLower(text), phrase) {
			return 1
		}
	}
	return 0
}

func leadDestinationCountry(text string) string {
	if isUSDestination(text) {
		return "US"
	}
	for _, term := range []string{"to uae", "to the uae", "to united arab emirates", "ship to uae", "gửi đến uae", "đi uae", "sang uae", "về uae",
		// "supplier in UAE" names where the seller is, so only "based in" counts.
		"to dubai", "to abu dhabi", "based in uae", "based in dubai"} {
		if containsLeadPhrase(text, term) {
			return "AE"
		}
	}
	for _, term := range []string{"sang nước anh", "về nước anh", "đi nước anh", "vương quốc anh", "to uk", "to the uk", "to gb", "united kingdom"} {
		if containsLeadPhrase(text, term) {
			return "GB"
		}
	}
	return ""
}

func isOrdinaryApparel(name string) bool {
	for _, blocked := range []string{"thuốc", "thực phẩm", "bổ sung", "pin", "mỹ phẩm", "chất lỏng", "supplement", "battery", "cosmetic", "tất cả", "sưởi", "điện", "usb", "sạc", "led", "heated", "electric", "rechargeable", "quạt", "đèn", "phát sáng", "fan", "light", "glow"} {
		if containsLeadPhrase(name, blocked) {
			return false
		}
	}
	for _, term := range []string{"áo", "hoodie", "shirt", "stocking", "tất", "vớ", "quần", "socks", "dress"} {
		if containsLeadPhrase(name, term) {
			return true
		}
	}
	return false
}
