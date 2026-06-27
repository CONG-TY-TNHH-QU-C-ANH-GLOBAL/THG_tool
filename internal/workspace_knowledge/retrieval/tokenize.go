package retrieval

import "strings"

// Tokenize is the shared word-set extractor for lexical searchers.
// Lower-cases, splits on non-alphanumeric, drops tokens shorter than
// 2 chars. Returns a set so callers can intersect cheaply.
//
// Deliberately NOT removing stopwords — short search queries
// ("cat tee POD") lose more signal than they gain from stopword
// removal. If a future searcher wants stemming, add a new helper
// rather than mutating this — score semantics across naive + hybrid
// MUST stay stable so the Operator Replay surface remains
// reproducible.
func Tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() >= 2 {
			out[cur.String()] = struct{}{}
		}
		cur.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
