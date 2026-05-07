package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func deterministicFacebookAction(prompt string, orgID, accountID int64) (string, map[string]any, bool) {
	lower := strings.ToLower(stripDashboardContext(prompt))
	folded := foldVietnameseForMatch(lower)
	args := map[string]any{}
	if orgID > 0 {
		args["org_id"] = orgID
	}
	if accountID > 0 {
		args["account_id"] = accountID
	}
	if maxItems := extractMaxItemsFromPrompt(prompt); maxItems > 0 {
		args["max_items"] = maxItems
	}

	if containsAnyFolded(folded, []string{"inbox", "messenger", "nhan tin", "tin nhan", "dm "}) &&
		containsAnyFolded(folded, []string{"lead", "leads", "tep khach", "khach hang", "tat ca", "all"}) {
		return "inbox_all_leads", args, true
	}
	if containsAnyFolded(folded, []string{"comment", "binh luan"}) &&
		containsAnyFolded(folded, []string{"lead", "leads", "tep khach", "khach hang", "tat ca", "all"}) {
		return "comment_all_leads", args, true
	}
	if containsAnyFolded(folded, []string{"dang bai", "posting", "post len", "tao bai"}) {
		args["content"] = strings.TrimSpace(stripDashboardContext(prompt))
		if u := firstFacebookURL(prompt); u != "" {
			args["group_url"] = u
		}
		return "create_job_post", args, true
	}
	if u := firstFacebookURL(prompt); u != "" && containsAnyFolded(folded, []string{"cao", "crawl", "scrape", "quet", "tim", "phan tich", "loc lead", "lead"}) {
		if isLikelyFacebookPostURL(u) && containsAnyFolded(folded, []string{"comment", "binh luan"}) {
			args["post_url"] = u
			return "scrape_comments", args, true
		}
		args["url"] = u
		return "scrape_group", args, true
	}
	if firstFacebookURL(prompt) == "" && containsAnyFolded(folded, []string{"cao", "crawl", "scrape", "quet", "tim tep", "tim khach", "lead", "leads"}) {
		query := promptKeywords(prompt)
		if query == "" {
			query = strings.TrimSpace(stripDashboardContext(prompt))
		}
		if query != "" {
			args["query"] = query
			return "search_groups", args, true
		}
	}
	return "", nil, false
}

func containsAnyFolded(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, foldVietnameseForMatch(needle)) {
			return true
		}
	}
	return false
}

func firstFacebookURL(prompt string) string {
	for _, raw := range regexp.MustCompile(`https?://[^\s]+`).FindAllString(prompt, -1) {
		u := strings.TrimRight(raw, ".,);]")
		lower := strings.ToLower(u)
		if strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com") {
			return u
		}
	}
	return ""
}

func isLikelyFacebookPostURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, "/posts/") ||
		strings.Contains(lower, "/permalink/") ||
		strings.Contains(lower, "story_fbid=") ||
		strings.Contains(lower, "/videos/") ||
		strings.Contains(lower, "/reel/")
}

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

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func argMissing(args map[string]any, key string) bool {
	if args == nil {
		return true
	}
	v, ok := args[key]
	if !ok || v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case float64:
		return t == 0
	case int:
		return t == 0
	case int64:
		return t == 0
	default:
		return false
	}
}

func argStringFromMap(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func isCrawlerTool(name string) bool {
	switch name {
	case "scrape_group", "scrape_comments", "search_groups":
		return true
	default:
		return false
	}
}

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
