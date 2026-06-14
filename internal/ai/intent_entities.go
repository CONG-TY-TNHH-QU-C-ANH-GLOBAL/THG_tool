package ai

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/fburl"
)

// Copilot intent — entity extraction layer. Pulls the platform-aware features the
// classifier routes on (Facebook URLs, scope, crawl verb, item count) from the
// prompt. Facebook URL recognition is delegated to internal/fburl (the single,
// host-anchored source of truth). No DB / outbound / session access.

// firstFacebookURL returns the first host-anchored Facebook URL in the prompt
// (lookalike hosts like facebook.com.evil.com are rejected by fburl).
func firstFacebookURL(prompt string) string {
	if urls := fburl.ExtractFacebookURLs(prompt); len(urls) > 0 {
		return urls[0]
	}
	return ""
}

// isLikelyFacebookPostURL reports whether u points at a commentable/crawlable FB
// post. Post detection is delegated to fburl.LooksLikePostURL; videos/reels are a
// crawl-path extension (the direct-link comment flow validates posts via
// fburl.CanonicalizePostURL, which excludes them).
func isLikelyFacebookPostURL(u string) bool {
	if fburl.LooksLikePostURL(u) {
		return true
	}
	lower := strings.ToLower(u)
	return strings.Contains(lower, "/videos/") || strings.Contains(lower, "/reel/")
}

// extractMaxItemsFromPrompt parses an explicit count ("50 bài", "crawl 30"),
// clamped to [1,200]. 0 = none specified.
func extractMaxItemsFromPrompt(prompt string) int64 {
	folded := foldVietnameseForMatch(prompt)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(\d{1,3})\s*(?:bai|post|posts|lead|leads)`),
		regexp.MustCompile(`(?:lay|cao|crawl|quet|tim)\s*(\d{1,3})`),
	} {
		m := re.FindStringSubmatch(folded)
		if len(m) != 2 {
			continue
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n <= 0 {
			continue
		}
		if n > 200 {
			n = 200
		}
		return n
	}
	return 0
}

// extractIntentEntities computes the normalized feature set the classifier and
// the RouteDecision observability view consume. `folded` is the
// accent-folded prompt; `prompt` is the raw text (for URL extraction).
func extractIntentEntities(folded, prompt string) IntentEntities {
	urls := fburl.ExtractFacebookURLs(prompt)
	e := IntentEntities{
		FacebookURLs:     urls,
		HasSpecificScope: containsAnyFolded(folded, lexSpecificScope),
		HasBulkScope:     containsAnyFolded(folded, lexCommentBulkScope),
		HasCrawlVerb:     containsAnyFolded(folded, lexCrawlVerbs),
	}
	if len(urls) > 0 {
		if isLikelyFacebookPostURL(urls[0]) {
			e.HasPostURL = true
		} else {
			e.HasGroupURL = true
		}
	}
	return e
}

// promptKeywords extracts up to 8 meaningful search keywords from a prompt
// (URLs + stop words removed) — used to seed a group search when no FB URL is
// given.
func promptKeywords(prompt string) string {
	prompt = stripDashboardContext(prompt)
	prompt = regexp.MustCompile(`https?://\S+`).ReplaceAllString(prompt, " ")
	cleaner := strings.NewReplacer(
		"\n", " ", "\t", " ", ".", " ", ",", ",", ";", ",", ":", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "\"", " ", "'", " ",
	)
	prompt = cleaner.Replace(prompt)
	fields := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '/'
	})
	stop := map[string]bool{
		// Crawl-action verbs (already filtered upstream but kept for safety).
		"cào": true, "cao": true, "crawl": true, "scrape": true, "quét": true, "quet": true,
		"lấy": true, "lay": true, "fetch": true, "get": true,
		// First-person + intent verbs.
		"tôi": true, "toi": true, "mình": true, "minh": true, "tao": true,
		"cần": true, "can": true, "tìm": true, "tim": true, "muốn": true, "muon": true,
		"giúp": true, "giup": true,
		// Lead-domain words that ARE the goal, not the keywords.
		"tệp": true, "tep": true, "khách": true, "khach": true,
		"hàng": true, "hang": true, "lead": true, "leads": true,
		"bài": true, "post": true, "posts": true, "bai": true, "bài viết": true,
		// Source / location nouns.
		"nhóm": true, "nhom": true, "group": true, "groups": true,
		"page": true, "fanpage": true, "trang": true, "facebook": true, "fb": true,
		"profile": true, "url": true, "link": true,
		// Vietnamese function words.
		"có": true, "co": true, "không": true, "khong": true,
		"nhu": true, "cầu": true, "cau": true,
		"hoặc": true, "hoac": true, "từ": true, "tu": true, "đi": true,
		"di": true, "và": true, "va": true, "với": true, "voi": true,
		"cho": true, "tại": true, "tai": true, "ở": true, "trong": true, "ngoài": true, "ngoai": true,
		"của": true, "cua": true, "là": true, "la": true, "được": true, "duoc": true,
		"này": true, "nay": true, "đó": true, "do": true, "kia": true, "đây": true, "day": true,
		"cũng": true, "cung": true, "thì": true, "thi": true, "rằng": true, "rang": true,
		// Numbers spelled out / qty modifiers commonly typed in prompts.
		"số": true, "so": true, "lượng": true, "luong": true,
		// English filler.
		"the": true, "a": true, "an": true, "of": true, "to": true, "from": true,
		"for": true, "in": true, "on": true, "at": true, "with": true, "and": true, "or": true,
	}
	out := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, raw := range fields {
		for _, token := range strings.Fields(raw) {
			token = strings.Trim(token, " -_")
			if len([]rune(token)) < 3 || stop[token] || seen[token] {
				continue
			}
			seen[token] = true
			out = append(out, token)
			if len(out) >= 8 {
				return strings.Join(out, ", ")
			}
		}
	}
	return strings.Join(out, ", ")
}
