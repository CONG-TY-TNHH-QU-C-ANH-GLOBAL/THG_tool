package ai

import "strings"

// Copilot intent — text normalization layer. Pure string functions shared by the
// router, the self-sufficiency gates, and business-context inference. No DB /
// outbound / session access. (Relocated from agent_action_router.go /
// agent_preflight.go / agent_request.go — same package, behavior unchanged.)

// stripDashboardContext drops the appended "Dashboard context:" block so the
// classifier sees only the user's own words, and trims surrounding whitespace.
func stripDashboardContext(prompt string) string {
	marker := "\n\nDashboard context:"
	if idx := strings.Index(prompt, marker); idx >= 0 {
		return strings.TrimSpace(prompt[:idx])
	}
	return strings.TrimSpace(prompt)
}

// foldVietnameseForMatch lowercases and strips Vietnamese diacritics so a single
// ASCII needle ("binh luan") matches accented input ("bình luận").
func foldVietnameseForMatch(value string) string {
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

// containsAnyFolded reports whether the folded value contains any of the needles
// (each needle is folded too, so callers pass plain ASCII lexicon entries).
func containsAnyFolded(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, foldVietnameseForMatch(needle)) {
			return true
		}
	}
	return false
}
