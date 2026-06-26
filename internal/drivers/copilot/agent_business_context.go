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

func inferBusinessCalibrationFromPrompt(prompt string) map[string]string {
	clean := strings.TrimSpace(regexp.MustCompile(`https?://\S+`).ReplaceAllString(stripDashboardContext(prompt), ""))
	out := map[string]string{}
	if clean == "" {
		return out
	}

	var contextLines []string
	for _, raw := range strings.Split(clean, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		folded := foldVietnameseForMatch(line)
		if strings.Contains(folded, "cao ") || strings.Contains(folded, "crawl") || strings.Contains(folded, "scrape") || strings.Contains(folded, "quet ") {
			continue
		}
		contextLines = append(contextLines, line)
		if out["business_name"] == "" && (strings.Contains(folded, "doanh nghiep") || strings.Contains(folded, "cong ty") || strings.Contains(folded, "brand")) {
			out["business_name"] = compactBusinessField(segmentAfterAny(line, []string{" la ", " ten la ", ":"}))
		}
		if out["services"] == "" && (strings.Contains(folded, "dich vu") || strings.Contains(folded, "mang kinh doanh") || strings.Contains(folded, "chuyen ho tro") || strings.Contains(folded, "offer")) {
			out["services"] = compactBusinessField(segmentAfterAny(line, []string{" chinh la ", " la ", ":"}))
		}
		if out["target_customers"] == "" && (strings.Contains(folded, roleKwKhachMua) || strings.Contains(folded, "can tim khach") || strings.Contains(folded, "seller") || strings.Contains(folded, "target")) {
			out["target_customers"] = compactBusinessField(line)
		}
		if out["target_signals"] == "" && strings.Contains(folded, "giu lai") {
			out["target_signals"] = compactBusinessField(segmentAfterAny(line, []string{" la ", ":"}))
		}
		if out["negative_signals"] == "" && (strings.Contains(folded, "loai bo") || strings.Contains(folded, "reject")) {
			out["negative_signals"] = compactBusinessField(segmentAfterAny(line, []string{" loai bo ", " la ", ":"}))
		}
		if out["markets"] == "" && (strings.Contains(folded, "thi truong") || strings.Contains(folded, "ship") || strings.Contains(folded, "my") || strings.Contains(folded, "trung quoc") || strings.Contains(folded, "viet nam")) {
			out["markets"] = compactBusinessField(line)
		}
	}
	if out["business_profile"] == "" {
		if len(contextLines) > 0 {
			out["business_profile"] = strings.Join(contextLines, "\n")
		} else {
			out["business_profile"] = clean
		}
	}
	if out["target_author_role"] == "" {
		folded := foldVietnameseForMatch(clean)
		switch {
		case strings.Contains(folded, roleKwUngVien) || strings.Contains(folded, "nhan su") || strings.Contains(folded, "tuyen"):
			out["target_author_role"] = "candidates"
		case strings.Contains(folded, "supplier") || strings.Contains(folded, "nha cung cap") || strings.Contains(folded, roleKwDoiTac):
			if strings.Contains(folded, roleKwKhachMua) || strings.Contains(folded, "tim khach") {
				out["target_author_role"] = "customers"
			} else {
				out["target_author_role"] = "suppliers"
			}
		default:
			out["target_author_role"] = "customers"
		}
	}
	return out
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
