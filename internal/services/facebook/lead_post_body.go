package facebook

import (
	"regexp"
	"strings"
)

// The Chrome extension sends the whole article innerText: post body, then
// Facebook UI labels, reaction counts and comments. Comments from other
// sellers ("we have a warehouse", "need an agent? DM me") must not decide
// what the poster wants, so suggestions read only the text before these labels.
var leadPostUIBoundary = regexp.MustCompile(`(?i)\s(?:xem bản dịch|see translation|xem bản gốc|see original|tất cả cảm xúc|all reactions|phù hợp nhất|most relevant|viết bình luận|write a comment)\b`)

// A comment starts with reaction/comment counts, the commenter's name and
// "· 9 phút · Theo dõi" ("· 9m · Follow"). Vietnamese posts have no
// "Xem bản dịch" label, so this header is their only boundary.
var leadCommentHeader = regexp.MustCompile(`(?i)\s(?:\d+\s+){1,3}(?:\S+\s+){1,5}·\s*\d+\s*(?:phút|giờ|ngày|tuần|năm|min|mins|h|hr|hrs|d|w|y)\s*·\s*(?:theo dõi|follow)`)

func leadPostBody(text string) string {
	cut := len(text)
	for _, boundary := range []*regexp.Regexp{leadPostUIBoundary, leadCommentHeader} {
		if at := boundary.FindStringIndex(text); at != nil && at[0] < cut {
			cut = at[0]
		}
	}
	if body := strings.TrimSpace(text[:cut]); cut < len(text) && len([]rune(body)) >= 20 {
		return body
	}
	return text
}

var vietnameseSupplierIntent = []string{
	"tìm nhà cung cấp", "cần nhà cung cấp", "tìm ncc", "cần ncc", "tìm xưởng", "cần xưởng",
	"tìm agent", "cần agent", "tìm supplier", "cần supplier", "tìm đơn vị cung cấp",
	"tim nha cung cap", "can nha cung cap", "tim xuong", "can xuong",
}

// asksForSupplier covers supplier requests in either language. Language
// selection keeps using asksForSupplierHelp, which is English only.
func asksForSupplier(text string) bool {
	if asksForSupplierHelp(text) {
		return true
	}
	for _, phrase := range vietnameseSupplierIntent {
		if containsLeadPhrase(text, phrase) {
			return true
		}
	}
	return false
}
