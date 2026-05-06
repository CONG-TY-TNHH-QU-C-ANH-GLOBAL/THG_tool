package prompt

import "strings"

// SanitizeText strips control characters and trims runaway lengths before
// user-controlled text is embedded into prompts or tool arguments.
func SanitizeText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	count := 0
	for _, r := range value {
		if maxRunes > 0 && count >= maxRunes {
			b.WriteRune('\u2026')
			break
		}
		switch {
		case r == '\n', r == '\t':
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7f:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
		count++
	}
	return b.String()
}
