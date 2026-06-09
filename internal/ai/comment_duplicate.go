package ai

import "strings"

// DetectRepeatedText reports whether content is the SAME block repeated back-to-back
// (A+A) — with or without a separator between the copies. A doubled comment must
// never enter the outbox (incident PR-1: defense-in-depth behind the extension's
// composer guard). Whitespace-insensitive, rune-safe.
func DetectRepeatedText(content string) bool {
	norm := strings.Join(strings.Fields(content), " ")
	r := []rune(norm)
	n := len(r)
	if n < 12 { // too short to judge a meaningful repeat
		return false
	}
	// A+A and A+<sep>+A both split into two equal halves at (or one rune past) the
	// midpoint, once surrounding whitespace is trimmed.
	for _, mid := range []int{n / 2, (n + 1) / 2} {
		if mid <= 0 || mid >= n {
			continue
		}
		left := strings.TrimSpace(string(r[:mid]))
		right := strings.TrimSpace(string(r[mid:]))
		if len([]rune(left)) >= 6 && left == right {
			return true
		}
	}
	return false
}
