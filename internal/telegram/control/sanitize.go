package control

import "strings"

// excerptSpam: tokens that are channel/source noise, not real post content. A summary made only of
// these (e.g. the "Facebook Facebook Facebook…" garbage) is rejected → the renderer uses a fallback.
var excerptSpam = map[string]bool{
	"facebook": true, "taobao": true, "1688": true, "group": true, "groups": true,
	"nhóm": true, "nhom": true, "page": true, "trang": true, "post": true, "posts": true,
	"bài": true, "bai": true, "viết": true, "viet": true,
}

// SanitizeExcerpt cleans a crawled post excerpt for display: collapse all whitespace, drop
// consecutive repeated tokens, reject spam-only/too-short content, and cap length. Returns "" when
// there is no usable content (the caller shows a fallback). Never returns secrets — input is post
// text only.
func SanitizeExcerpt(text string) string {
	fields := strings.Fields(strings.TrimSpace(text)) // collapses spaces, tabs, newlines
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, 0, len(fields))
	prevLow := ""
	meaningful := 0
	for _, w := range fields {
		low := strings.ToLower(w)
		if low == prevLow {
			continue // drop a token identical to the one before it (de-spams repeats)
		}
		prevLow = low
		out = append(out, w)
		if !excerptSpam[strings.Trim(low, ".,!?…\"'()[]:;-")] {
			meaningful++
		}
	}
	s := strings.Join(out, " ")
	if meaningful == 0 || len([]rune(s)) < 6 {
		return "" // only spam tokens, or too short to be useful
	}
	if r := []rune(s); len(r) > 300 {
		s = strings.TrimSpace(string(r[:300])) + "…"
	}
	return s
}
