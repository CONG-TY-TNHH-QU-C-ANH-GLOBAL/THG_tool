package models

import (
	"regexp"
	"strings"
)

// Content-accurate coverage derivation (spec: MULTI_ACTOR_COVERAGE_POLICY). Instead
// of inferring website/CTA usage from a touch count, we read the ACTUAL prior comment
// texts: a website is "used" only if a comment actually cited it, a hard CTA only if
// a comment actually carried one. Angle tags are classified from the text so a later
// actor can pick a different angle.

// DetectWebsiteUsed reports whether any prior comment text cites the verified website.
func DetectWebsiteUsed(contents []string, website string) bool {
	w := normalizeURLForMatch(website)
	if w == "" {
		return false
	}
	for _, c := range contents {
		if strings.Contains(strings.ToLower(c), w) {
			return true
		}
	}
	return false
}

var ctaKeywords = []string{
	"inbox", "nhắn tin", "nhan tin", "liên hệ", "lien he", "ib mình", "ib minh",
	"ib nhé", "ib nhe", "ib nha", "zalo", "telegram", "dm mình", "dm minh",
	"kết bạn", "ket ban", "để lại sđt", "de lai sdt",
}
var reCTAContact = regexp.MustCompile(`(?i)@[a-z0-9_]{4,}|t\.me/|(?:\+?84|0)(?:[\s.\-]?\d){8,10}`)

// DetectDirectCTAUsed reports whether any prior comment used a hard inbox/contact CTA.
func DetectDirectCTAUsed(contents []string) bool {
	for _, c := range contents {
		lc := strings.ToLower(c)
		for _, kw := range ctaKeywords {
			if strings.Contains(lc, kw) {
				return true
			}
		}
		if reCTAContact.MatchString(c) {
			return true
		}
	}
	return false
}

// angleOrder keeps ClassifyAngles deterministic; angleKeywords are matched (lowercased).
var angleOrder = []string{"price", "speed", "sourcing", "fulfillment", "quality", "experience"}
var angleKeywords = map[string][]string{
	"price":       {"giá", "rẻ", "chi phí", "cost", "cạnh tranh", "base cost"},
	"speed":       {"nhanh", "ship", "vận chuyển", "giao hàng", "đi hàng", "fast", "delivery"},
	"sourcing":    {"nguồn", "sourcing", "tìm nguồn", "nhà cung cấp", "supplier", "1688", "taobao"},
	"fulfillment": {"fulfill", "kho", "warehouse", "đóng gói", "pick pack", "kết nối"},
	"quality":     {"chất lượng", "quality", "kiểm hàng", "qc", "mẫu"},
	"experience":  {"kinh nghiệm", "từng làm", "đã làm", "experience", "case", "end-to-end"},
}

// ClassifyAngles returns the distinct angle tags present across the comment texts, in
// a stable order — the "used angles" a later actor must avoid.
func ClassifyAngles(contents []string) []string {
	joined := strings.ToLower(strings.Join(contents, " \n "))
	if strings.TrimSpace(joined) == "" {
		return nil
	}
	var out []string
	for _, tag := range angleOrder {
		for _, kw := range angleKeywords[tag] {
			if strings.Contains(joined, kw) {
				out = append(out, tag)
				break
			}
		}
	}
	return out
}
