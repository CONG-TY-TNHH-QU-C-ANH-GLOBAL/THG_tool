package facebook

import (
	"strings"
	"unicode"
)

// A short Vietnamese noun can identify a catalog item when it is part of the
// same two-word product phrase in both the post and the start of the title.
// This keeps áo phông, ly sứ and cốc sứ without matching generic "in logo".
func sharedProductHeadPhrase(post, title string) bool {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for i := 0; i+1 < len(words) && i < 4; i++ {
		first, second := words[i], words[i+1]
		if !productPhraseWord(first) || !productPhraseWord(second) {
			continue
		}
		if containsLeadPhrase(post, first+" "+second) {
			return true
		}
	}
	return false
}

func productPhraseWord(word string) bool {
	return len([]rune(word)) >= 2 && !leadProductStopWords[word] && word != "in" && word != "theo" && word != "yêu" && word != "cầu"
}
