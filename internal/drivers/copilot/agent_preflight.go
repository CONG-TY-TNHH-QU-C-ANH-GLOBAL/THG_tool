package copilot

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/thg/scraper/internal/models"
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

func facebookBrowserPreflight(accounts []models.Account, selectedAccountID int64) (bool, string) {
	if selectedAccountID > 0 {
		for _, acc := range accounts {
			if acc.ID != selectedAccountID {
				continue
			}
			if accountReadyForFacebookAutomation(acc) {
				return true, ""
			}
			return false, browserNotReadyMessage(&acc)
		}
		return false, browserNotReadyMessage(nil)
	}
	for _, acc := range accounts {
		if accountReadyForFacebookAutomation(acc) {
			return true, ""
		}
	}
	return false, browserNotReadyMessage(nil)
}

func accountReadyForFacebookAutomation(acc models.Account) bool {
	return acc.Platform == models.PlatformFacebook &&
		acc.BrowserLoggedIn &&
		acc.Status == models.AccountActive &&
		strings.TrimSpace(acc.FBUserID) != ""
}

func pickReadyFacebookAccountID(accounts []models.Account) int64 {
	for _, acc := range accounts {
		if accountReadyForFacebookAutomation(acc) {
			return acc.ID
		}
	}
	return 0
}

// businessCalibrationPreflight is a no-op kept for the two legacy call sites.
// Crawl is no longer blocked by missing profile — instead, mergeEphemeralCrawlTargeting
// derives target_author_role, target_signals, and negative_signals from the user's
// prompt and feeds them into the in-memory userContext for the request scope only.
func businessCalibrationPreflight(_ map[string]string, _ string) (bool, string) {
	return true, ""
}

// mergeEphemeralCrawlTargeting fills userCtx with prompt-derived crawl targeting
// (target_author_role, target_signals, negative_signals) when the user's prompt
// contains crawl intent. The merged values are scoped to the current request only —
// they are NOT persisted to the database. Empty inferred values do not overwrite
// existing profile values, so a configured profile still falls through.
func mergeEphemeralCrawlTargeting(userCtx map[string]string, prompt string) {
	if userCtx == nil {
		return
	}
	inferred := inferCrawlTargetingFromPrompt(prompt)
	for key, value := range inferred {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		userCtx[key] = value
		userCtx["org_"+key] = value
	}
}

// inferCrawlTargetingFromPrompt extracts a minimal Market Signal Gate from the
// user's current prompt: target audience role plus a handful of positive and
// negative phrases. Returns an empty map when the prompt is empty.
func inferCrawlTargetingFromPrompt(prompt string) map[string]string {
	out := map[string]string{}
	clean := strings.TrimSpace(regexp.MustCompile(`https?://\S+`).ReplaceAllString(stripDashboardContext(prompt), ""))
	if clean == "" {
		return out
	}
	folded := foldVietnameseForMatch(clean)

	role := "customers"
	switch {
	case containsAnyFolded(folded, []string{"tuyen ", "tuyen dung", "nhan su", "ung vien", "tim viec", "can viec", "san sang lam"}):
		role = "candidates"
	case containsAnyFolded(folded, []string{"supplier", "nha cung cap", "nguon hang", "factory"}) && !containsAnyFolded(folded, []string{"tim khach", "khach mua", "tim buyer"}):
		role = "suppliers"
	case containsAnyFolded(folded, []string{"doi tac", "partner", "reseller", "agency hop tac"}):
		role = "partners"
	}
	out["target_author_role"] = role

	if positives := extractCrawlPositiveSignals(folded, role); len(positives) > 0 {
		out["target_signals"] = strings.Join(positives, ", ")
	}

	if negatives := defaultCrawlNegativeSignals(role); len(negatives) > 0 {
		out["negative_signals"] = strings.Join(negatives, ", ")
	}

	return out
}

// extractCrawlPositiveSignals returns folded phrases the prompt mentions that
// buyer/candidate/supplier posts would also use. Output is intentionally small —
// the LLM classifier (UniversalClassify) does the heavy lifting; we only nudge
// the SignalGate toward the right intent.
func extractCrawlPositiveSignals(folded, role string) []string {
	var pool []string
	switch role {
	case "candidates":
		pool = []string{
			"tim viec", "can viec", "ung vien", "ho so", "cv",
			"remote ok", "san sang lam", "co kinh nghiem", "freelance",
		}
	case "suppliers":
		pool = []string{
			"nhan in pod", "nhan order", "nhan lam", "studio", "factory",
			"san xuat", "in theo yeu cau", "fulfillment", "warehouse",
		}
	case "partners":
		pool = []string{
			"hop tac", "doi tac", "reseller", "agency", "share doanh thu",
		}
	default:
		pool = []string{
			"tim supplier", "can supplier", "tim nha cung cap", "tim nguon hang",
			"tim hang", "can bao gia", "looking for supplier", "need supplier",
			"can tu van", "can tim", "ai co", "ai biet",
			"pod", "dropship", "fulfillment",
			"ship my", "ship usa", "ship eu", "ship sang my",
			"vn sang my", "tq sang my", "viet nam sang my", "trung quoc sang my",
		}
	}
	seen := map[string]bool{}
	var result []string
	for _, kw := range pool {
		if seen[kw] {
			continue
		}
		if strings.Contains(folded, kw) {
			seen[kw] = true
			result = append(result, kw)
		}
	}
	return result
}

// defaultCrawlNegativeSignals returns a baseline list of phrases to reject when
// targeting buyers/candidates — these are the most common provider/spam markers.
func defaultCrawlNegativeSignals(role string) []string {
	switch role {
	case "candidates":
		return []string{
			"tuyen ctv mlm", "lam giau nhanh", "lam viec tai nha 0 von",
			"co hoi kiem tien khong gioi han", "spam link",
		}
	case "suppliers":
		return nil
	case "partners":
		return nil
	default:
		return []string{
			"nhan lam pod", "nhan order pod", "shop pod nhan",
			"studio nhan in", "agency nhan", "fulfillment service offered",
			"chuyen nhan in pod", "xuong in pod nhan",
		}
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
		if out["target_customers"] == "" && (strings.Contains(folded, "khach mua") || strings.Contains(folded, "can tim khach") || strings.Contains(folded, "seller") || strings.Contains(folded, "target")) {
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
		case strings.Contains(folded, "ung vien") || strings.Contains(folded, "nhan su") || strings.Contains(folded, "tuyen"):
			out["target_author_role"] = "candidates"
		case strings.Contains(folded, "supplier") || strings.Contains(folded, "nha cung cap") || strings.Contains(folded, "doi tac"):
			if strings.Contains(folded, "khach mua") || strings.Contains(folded, "tim khach") {
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

// foldVietnameseForMatch moved to intent_normalize.go (Copilot intent layer).

func browserNotReadyMessage(acc *models.Account) string {
	target := "Workspace chưa có Facebook session sẵn sàng."
	if acc != nil {
		target = fmt.Sprintf("Facebook account %q chưa sẵn sàng để chạy automation.", acc.Name)
	}
	return target + `

THG chỉ chạy crawl/comment/inbox khi Browser đã xác nhận đúng Facebook session thật của workspace. Cách này tránh chạy nhầm tài khoản và giữ dữ liệu theo đúng organization.

Vào tab Browser và hoàn tất 3 bước:
1. Cài và ghép THG Chrome Extension với workspace.
2. Mở tab Facebook đã đăng nhập trong chính Chrome đó.
3. Chờ Browser chuyển sang trạng thái Facebook Extension ready.

Khi Browser đã sẵn sàng, gửi lại prompt này. Agent sẽ dùng đúng account đã xác thực để thu dữ liệu thật, phân loại leads và lưu kết quả về workspace.`
}
