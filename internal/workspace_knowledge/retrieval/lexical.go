package retrieval

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// NormalizeLexical folds Latin accents (including Vietnamese đ), case and
// separators. Other scripts keep their letters and combining marks. Use this
// explicitly for v2 search; Tokenize retains the v1 replay semantics.
func NormalizeLexical(text string) string {
	var out strings.Builder
	latin := false
	for _, r := range norm.NFD.String(strings.ToLower(text)) {
		if unicode.IsMark(r) {
			if !latin {
				out.WriteRune(r)
			}
			continue
		}
		latin = unicode.In(r, unicode.Latin)
		if r == 'đ' {
			r = 'd'
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte(' ')
		}
	}
	return norm.NFC.String(strings.Join(strings.Fields(out.String()), " "))
}

// LexicalTokens returns whole normalized words, never fragments of accented
// words. Length is measured in runes. It does not translate or segment CJK.
func LexicalTokens(text string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, word := range strings.Fields(NormalizeLexical(text)) {
		if utf8.RuneCountInString(word) >= 2 {
			out[word] = struct{}{}
		}
	}
	return out
}
