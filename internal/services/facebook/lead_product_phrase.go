package facebook

import (
	"strings"
	"unicode"
)

// A short Vietnamese noun can identify a catalog item when it is part of the
// same two-word product phrase in both the post and the start of the title.
// This keeps áo phông, ly sứ and cốc sứ without matching generic "in logo".
// Only the title's opening pair names the product: a later pair such as
// "cao cấp" in "Áo thun cao cấp" is a qualifier shared by unrelated items.
func sharedProductHeadPhrase(post, title string) bool {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) < 2 || !productPhraseWord(words[0]) || !productPhraseWord(words[1]) {
		return false
	}
	return containsLeadPhrase(post, words[0]+" "+words[1])
}

func productPhraseWord(word string) bool {
	return len([]rune(word)) >= 2 && !leadProductStopWords[word] && word != "in" && word != "theo" && word != "yêu" && word != "cầu"
}
