package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func argBool(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes" || s == "auto"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func argInt64(args map[string]any, key string) int64 {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

func maxItemsFromPrompt(prompt string) int {
	prompt = strings.ToLower(strings.TrimSpace(prompt))
	if prompt == "" {
		return 0
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(\d{1,3})\s*(?:bai|bài|post|posts)`),
		regexp.MustCompile(`(?:lay|lấy|cao|cào|crawl)\s*(\d{1,3})`),
	} {
		m := re.FindStringSubmatch(prompt)
		if len(m) != 2 {
			continue
		}
		n, _ := strconv.Atoi(m[1])
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func orgContext(db *store.Store, orgID int64, key string) string {
	value, _ := db.GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
	return strings.TrimSpace(value)
}

func businessContextForOrg(db *store.Store, orgID int64) string {
	parts := []string{}
	for _, item := range []struct {
		label string
		key   string
	}{
		{"Business profile", "business_profile"},
		{"Sales voice", "sales_voice"},
		{"Sales voice memory", "sales_voice_summary"},
		{"Comment style rules", "comment_style_rules"},
		{"Inbox style rules", "inbox_style_rules"},
		{"CTA rules", "cta_rules"},
		{"Forbidden phrases", "forbidden_phrases"},
		{"Pricing summary", "pricing_summary"},
		{"Sales examples", "sales_examples_summary"},
		{"Target customers", "target_customers"},
		{"Target signals", "target_signals"},
		{"Reject rules", "reject_rules"},
		{"Private files", "private_files_summary"},
		{"Connected data sources", "data_sources_summary"},
	} {
		if value := orgContext(db, orgID, item.key); value != "" {
			parts = append(parts, item.label+":\n"+value)
		}
	}
	if price := strings.TrimSpace(db.GetPriceListText()); price != "" {
		parts = append(parts, price)
	}
	if len(parts) == 0 {
		return "Business context is not configured yet. Avoid making claims about prices, inventory, guarantees, or policies."
	}
	return strings.Join(parts, "\n\n")
}

func splitKeywords(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func promptKeywordFallback(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = regexp.MustCompile(`https?://\S+`).ReplaceAllString(raw, " ")
	replacer := strings.NewReplacer("\n", " ", "\t", " ", ".", " ", ";", ",", ":", " ", "(", " ", ")", " ")
	raw = replacer.Replace(strings.ToLower(raw))
	stop := map[string]bool{
		"cào": true, "cao": true, "crawl": true, "scrape": true, "tôi": true, "toi": true,
		"cần": true, "can": true, "tìm": true, "tim": true, "tệp": true, "tep": true,
		"khách": true, "khach": true, "nhu": true, "cầu": true, "cau": true, "và": true, "va": true,
		"hoặc": true, "hoac": true, "the": true, "from": true, "with": true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, 8)
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '|' || r == '/' }) {
		for _, part := range strings.Fields(token) {
			part = strings.Trim(part, " -_")
			if len([]rune(part)) < 3 || stop[part] || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
			if len(out) >= 8 {
				return strings.Join(out, ", ")
			}
		}
	}
	return strings.Join(out, ", ")
}

func sourceTypeFromURL(u string) string {
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com"):
		if strings.Contains(lower, "/posts/") || strings.Contains(lower, "story_fbid") || strings.Contains(lower, "/permalink/") {
			return "facebook_post"
		}
		return "facebook_group"
	default:
		return "web_url"
	}
}

func detectPlatformFromURL(u string) models.Platform {
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com"):
		return models.PlatformFacebook
	case strings.Contains(lower, "tiktok.com"):
		return models.PlatformTikTok
	case strings.Contains(lower, "zalo"):
		return models.PlatformZalo
	default:
		return models.PlatformFacebook
	}
}
