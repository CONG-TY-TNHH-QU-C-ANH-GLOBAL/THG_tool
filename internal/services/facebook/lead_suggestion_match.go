package facebook

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/thg/scraper/internal/models"
)

// A retrieval hit can be broadly relevant to the workspace without matching the
// item in this post. Require a product term from the post before offering it.
func matchesLeadProduct(leadText, title string) bool {
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, word := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
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

var leadProductSpecificTerms = map[string]bool{
	"hoodie": true, "stocking": true, "tshirt": true,
	"sweatshirt": true, "shirt": true, "thun": true,
}

var leadProductStopWords = map[string]bool{
	"sản": true, "phẩm": true, "hàng": true, "cần": true, "tìm": true,
	"giá": true, "cho": true, "với": true, "của": true, "bên": true,
	"mình": true, "shop": true, "bán": true, "đơn": true, "ship": true,
	"the": true, "and": true, "for": true, "product": true, "new": true,
}

func matchingCandidates(leadText string, candidates []models.KnowledgeCandidate) []models.KnowledgeCandidate {
	matched := make([]models.KnowledgeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
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
	text = strings.ToLower(text)
	for _, phrase := range []string{"in theo logo", "in logo", "in theo thiết kế", "in theo yêu cầu", "cá nhân hóa", "custom logo", "personalized", "print on demand", "pod"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
func isUSDestination(text string) bool {
	text = strings.ToLower(text)
	for _, term := range []string{"sang mỹ", "về mỹ", "đi mỹ", "tại mỹ", "qua mỹ", "to us", "to usa", " united states", " hoa kỳ", " u.s."} {
		if strings.Contains(" "+text, term) {
			return true
		}
	}
	return false
}

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
	text = strings.ToLower(text)
	for _, term := range []string{"sang anh", "về anh", "đi anh", "to uk", "to gb", " united kingdom"} {
		if strings.Contains(" "+text, term) {
			return "GB"
		}
	}
	return ""
}

func isOrdinaryApparel(name string) bool {
	name = strings.ToLower(name)
	for _, blocked := range []string{"thuốc", "thực phẩm", "bổ sung", "pin", "mỹ phẩm", "chất lỏng", "supplement", "battery", "cosmetic"} {
		if strings.Contains(name, blocked) {
			return false
		}
	}
	for _, term := range []string{"áo", "hoodie", "shirt", "stocking", "tất", "vớ", "quần", "socks", "dress"} {
		if strings.Contains(name, term) {
			return true
		}
	}
	return false
}
