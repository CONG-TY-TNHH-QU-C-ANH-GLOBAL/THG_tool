package crawlingest

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/store"
)

// Org-intelligence keyword extraction: turns an org's business-profile context
// fields into a bounded keyword set used to bias crawl-result scoring. Pure aside
// from the read-only org-context lookups.

func orgIntelligenceKeywords(db *store.Store, orgID int64) []string {
	text := gatherOrgIntelText(db, orgID)
	if text == "" {
		return nil
	}
	return keywordsFromText(text)
}

// gatherOrgIntelText concatenates the org's business-profile context fields into
// one lowercased blob ("" when db/org is invalid or no field is set).
func gatherOrgIntelText(db *store.Store, orgID int64) string {
	if db == nil || orgID <= 0 {
		return ""
	}
	var combined strings.Builder
	for _, key := range []string{"business_profile", "business_industry", "services", "target_customers", "target_signals", "markets", "private_files_summary", "data_sources_summary"} {
		value, err := db.Leads().GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
		if err == nil && strings.TrimSpace(value) != "" {
			combined.WriteByte(' ')
			combined.WriteString(value)
		}
	}
	return strings.ToLower(combined.String())
}

// keywordsFromText returns up to 24 unique, non-stopword keyword tokens (>= 3
// runes) extracted from the lowercased blob.
func keywordsFromText(text string) []string {
	stop := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true, "that": true,
		"this": true, "you": true, "your": true, "are": true, "can": true, "will": true,
		"toi": true, "tôi": true, "cua": true, "của": true, "cho": true, "voi": true,
		"với": true, "cac": true, "các": true, "nhung": true, "những": true, "khach": true,
		"khách": true, "hang": true, "hàng": true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, 24)
	for _, token := range strings.FieldsFunc(text, func(r rune) bool { return !isKeywordRune(r) }) {
		token = strings.Trim(token, "._-+/:%&=?'\"")
		if len([]rune(token)) < 3 || stop[token] || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
		if len(out) >= 24 {
			break
		}
	}
	return out
}

// isKeywordRune reports whether r is kept inside a keyword token: letters,
// digits, selected punctuation, or non-ASCII (e.g. Vietnamese diacritics).
func isKeywordRune(r rune) bool {
	return r == '_' || r == '-' || r == '+' || r == '#' || r == '@' || r == '.' || r == '/' || r == ':' || r == '%' || r == '&' || r == '=' || r == '?' || r == '\'' || r == '"' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || r >= 128
}
