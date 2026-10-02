package facebook

import (
	"regexp"
	"strings"
)

var podReplyURL = regexp.MustCompile(`https?://[^\s<>]+`)
var podReplyMoney = regexp.MustCompile(`(?i)(?:[¥$₫]|\d[\d.,]*\s*(?:usd|vnd|cny|rmb|yuan|k)\b|\d[\d.,]*\s*(?:đồng|nghìn|triệu|đ)(?:\s|$|[.,;)]))`)

// The POD catalog facts contain a product name and link, but no verified price
// or freight. Reject any generated financial claim or unrelated link rather
// than asking a sale to check an invented number in a copy-ready draft.
func generatedPODReplyGrounded(reply, productURL string) bool {
	if podReplyMoney.MatchString(reply) {
		return false
	}
	for _, found := range podReplyURL.FindAllString(reply, -1) {
		if strings.TrimRight(found, ".,);") != productURL {
			return false
		}
	}
	return true
}
