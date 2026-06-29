// Package textnorm provides Vietnamese-aware text normalization for prompt / keyword
// matching: diacritic folding and folded substring matching. It is generic, pure
// (stdlib `strings` only), domain-free, and a neutral leaf — copilot intent
// classification, self-sufficiency gates, and business-context inference all depend on
// it without depending on each other. Extracted from copilot/intent_normalize.go under
// ARCHCP3 so the eventual copilot/intent subpackage exposes only real intent API, not
// generic text helpers.
package textnorm

import "strings"

// Fold lowercases and strips Vietnamese diacritics so a single ASCII needle
// ("binh luan") matches accented input ("bình luận").
func Fold(value string) string {
	value = strings.ToLower(value)
	return strings.Map(func(r rune) rune {
		switch r {
		case 'à', 'á', 'ạ', 'ả', 'ã', 'â', 'ầ', 'ấ', 'ậ', 'ẩ', 'ẫ', 'ă', 'ằ', 'ắ', 'ặ', 'ẳ', 'ẵ':
			return 'a'
		case 'è', 'é', 'ẹ', 'ẻ', 'ẽ', 'ê', 'ề', 'ế', 'ệ', 'ể', 'ễ':
			return 'e'
		case 'ì', 'í', 'ị', 'ỉ', 'ĩ':
			return 'i'
		case 'ò', 'ó', 'ọ', 'ỏ', 'õ', 'ô', 'ồ', 'ố', 'ộ', 'ổ', 'ỗ', 'ơ', 'ờ', 'ớ', 'ợ', 'ở', 'ỡ':
			return 'o'
		case 'ù', 'ú', 'ụ', 'ủ', 'ũ', 'ư', 'ừ', 'ứ', 'ự', 'ử', 'ữ':
			return 'u'
		case 'ỳ', 'ý', 'ỵ', 'ỷ', 'ỹ':
			return 'y'
		case 'đ':
			return 'd'
		default:
			return r
		}
	}, value)
}

// ContainsAny reports whether the folded value contains any of the needles (each
// needle is folded too, so callers pass plain ASCII lexicon entries).
func ContainsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, Fold(needle)) {
			return true
		}
	}
	return false
}
