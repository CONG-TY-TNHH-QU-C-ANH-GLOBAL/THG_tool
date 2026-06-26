package copilot

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode/utf8"
)

func orgContextKeysForPrompt() []string {
	return []string{
		"business_profile",
		"business_name",
		"business_industry",
		"services",
		"target_customers",
		"target_author_role",
		"target_signals",
		"negative_signals",
		"business_location",
		"markets",
		"business_usp",
		"tone",
		"approval_policy",
		"reject_rules",
		"private_files_summary",
		"data_sources_summary",
		"outbound_mode",
	}
}

func isBusinessContextPrompt(prompt string) bool {
	lower := strings.ToLower(stripDashboardContext(prompt))
	triggers := []string{
		"định vị doanh nghiệp", "dinh vi doanh nghiep", "business profile", "business context",
		"mình là", "minh la", "chúng tôi là", "chung toi la", "công ty", "cong ty",
		"doanh nghiệp", "doanh nghiep", "brand", "thương hiệu", "thuong hieu",
		"dịch vụ của tôi", "dich vu cua toi", "chúng tôi bán", "chung toi ban",
	}
	for _, trigger := range triggers {
		if strings.Contains(lower, trigger) {
			return true
		}
	}
	return false
}

func (a *Agent) captureBusinessCalibrationFromPrompt(orgID int64, userCtx map[string]string, prompt string) {
	if a == nil || a.db == nil || orgID <= 0 || !isBusinessContextPrompt(prompt) {
		return
	}
	inferred := inferBusinessCalibrationFromPrompt(prompt)
	for key, value := range inferred {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if current := strings.TrimSpace(userCtx[key]); current != "" && len([]rune(current)) > len([]rune(value))*2 {
			continue
		}
		if err := a.db.Leads().SetContext(fmt.Sprintf("org:%d:%s", orgID, key), value); err != nil {
			log.Printf("[Agent] save business calibration failed org=%d key=%s: %v", orgID, key, err)
			continue
		}
		userCtx[key] = value
		userCtx["org_"+key] = value
		if key == "business_profile" {
			userCtx["business_desc"] = value
		}
	}
}

// businessLineRules maps a context line to the business field it fills (set only
// when still empty and the folded line contains a trigger). markers == nil means
// "use the whole line"; otherwise the segment after the first marker (segmentAfterAny).
var businessLineRules = []struct {
	key      string
	triggers []string
	markers  []string
}{
	{"business_name", []string{"doanh nghiep", "cong ty", "brand"}, []string{" la ", " ten la ", ":"}},
	{"services", []string{"dich vu", "mang kinh doanh", "chuyen ho tro", "offer"}, []string{" chinh la ", " la ", ":"}},
	{"target_customers", []string{roleKwKhachMua, "can tim khach", "seller", "target"}, nil},
	{"target_signals", []string{"giu lai"}, []string{" la ", ":"}},
	{"negative_signals", []string{"loai bo", "reject"}, []string{" loai bo ", " la ", ":"}},
	{"markets", []string{"thi truong", "ship", "my", "trung quoc", "viet nam"}, nil},
}

func inferBusinessCalibrationFromPrompt(prompt string) map[string]string {
	clean := cleanBusinessPrompt(prompt)
	out := map[string]string{}
	if clean == "" {
		return out
	}
	contextLines := collectBusinessContextLines(clean, out)
	inferBusinessProfile(out, contextLines, clean)
	inferTargetAuthorRole(out, clean)
	return out
}

func cleanBusinessPrompt(prompt string) string {
	return strings.TrimSpace(regexp.MustCompile(`https?://\S+`).ReplaceAllString(stripDashboardContext(prompt), ""))
}

// collectBusinessContextLines walks the non-empty, non-crawl-command lines,
// filling per-line business fields in out and returning the kept lines.
func collectBusinessContextLines(clean string, out map[string]string) []string {
	var contextLines []string
	for _, raw := range strings.Split(clean, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		folded := foldVietnameseForMatch(line)
		if businessLineIsCrawlCommand(folded) {
			continue
		}
		contextLines = append(contextLines, line)
		inferBusinessLineFields(out, line, folded)
	}
	return contextLines
}

// businessLineIsCrawlCommand reports a crawl/scrape line (excluded from the profile).
func businessLineIsCrawlCommand(folded string) bool {
	return containsAnyFolded(folded, []string{"cao ", "crawl", "scrape", "quet "})
}

// inferBusinessLineFields fills any still-empty field whose trigger the line matches.
func inferBusinessLineFields(out map[string]string, line, folded string) {
	for _, rule := range businessLineRules {
		if out[rule.key] != "" || !containsAnyFolded(folded, rule.triggers) {
			continue
		}
		if rule.markers == nil {
			out[rule.key] = compactBusinessField(line)
			continue
		}
		out[rule.key] = compactBusinessField(segmentAfterAny(line, rule.markers))
	}
}

// inferBusinessProfile sets business_profile from the kept lines (or the whole
// cleaned prompt as a fallback) when it was not set explicitly.
func inferBusinessProfile(out map[string]string, contextLines []string, clean string) {
	if out["business_profile"] != "" {
		return
	}
	if len(contextLines) > 0 {
		out["business_profile"] = strings.Join(contextLines, "\n")
		return
	}
	out["business_profile"] = clean
}

// inferTargetAuthorRole defaults the audience role when not already inferred.
func inferTargetAuthorRole(out map[string]string, clean string) {
	if out["target_author_role"] != "" {
		return
	}
	folded := foldVietnameseForMatch(clean)
	switch {
	case containsAnyFolded(folded, []string{roleKwUngVien, "nhan su", "tuyen"}):
		out["target_author_role"] = "candidates"
	case containsAnyFolded(folded, []string{"supplier", "nha cung cap", roleKwDoiTac}):
		if containsAnyFolded(folded, []string{roleKwKhachMua, "tim khach"}) {
			out["target_author_role"] = "customers"
		} else {
			out["target_author_role"] = "suppliers"
		}
	default:
		out["target_author_role"] = "customers"
	}
}

func segmentAfterAny(line string, markers []string) string {
	folded := foldVietnameseForMatch(line)
	origRunes := []rune(line)
	for _, marker := range markers {
		idx := strings.LastIndex(folded, marker)
		if idx < 0 {
			continue
		}
		start := utf8.RuneCountInString(folded[:idx]) + utf8.RuneCountInString(marker)
		if start >= 0 && start < len(origRunes) {
			return strings.TrimSpace(string(origRunes[start:]))
		}
	}
	return line
}

func compactBusinessField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " .,:;|")
	if len([]rune(value)) > 500 {
		value = string([]rune(value)[:500])
	}
	return strings.TrimSpace(value)
}
