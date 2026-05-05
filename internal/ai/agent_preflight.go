package ai

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

func businessCalibrationPreflight(userCtx map[string]string, prompt string) (bool, string) {
	profile := ProfileFromContext(userCtx)
	if profile.IsConfigured() {
		return true, ""
	}
	if isBusinessContextPrompt(prompt) {
		return true, ""
	}
	return false, `Mình chưa chạy crawl ngay vì workspace chưa có định vị doanh nghiệp đủ rõ.

Để Market Signal Gate lọc đúng tệp và không đổ dữ liệu rác vào dashboard, hãy cấu hình trước phần Định vị doanh nghiệp trong Data Private, hoặc trả lời trực tiếp theo 5 ý ngắn:

1. Doanh nghiệp/brand của bạn là ai?
2. Bạn đang bán sản phẩm, dịch vụ hoặc offer gì?
3. Tệp cần tìm là ai: khách mua dịch vụ, supplier, partner, ứng viên hay nhóm khác?
4. Những tín hiệu nào phải giữ lại? Ví dụ: “cần báo giá”, “looking for supplier”, “tìm fulfillment”.
5. Những tín hiệu nào phải loại bỏ? Ví dụ: bài quảng cáo dịch vụ, tuyển CTV, spam link, đối thủ tự bán.

Sau khi lưu định vị, gửi lại prompt. Lúc đó agent sẽ dùng đúng Facebook session của workspace để crawl, lọc theo ngữ cảnh doanh nghiệp, phân loại hot/warm/cold và chỉ lưu leads đủ điều kiện.`
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
		if err := a.db.SetContext(fmt.Sprintf("org:%d:%s", orgID, key), value); err != nil {
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

func foldVietnameseForMatch(value string) string {
	value = strings.ToLower(value)
	return strings.Map(func(r rune) rune {
		switch r {
		case 'à', 'á', 'ạ', 'ả', 'ã', 'â', 'ầ', 'ấ', 'ậ', 'ẩ', 'ẫ', 'ă', 'ằ', 'ắ', 'ặ', 'ẳ', 'ẵ':
			return 'a'
		case 'è', 'é', 'ẹ', 'ẻ', 'ẽ', 'ê', 'ề', 'ế', 'ệ', 'ể', 'ễ':
			return 'e'
		case 'ì', 'í', 'ị', 'ỉ', 'ĩ':
			return 'i'
		case 'ò', 'ó', 'ọ', 'ỏ', 'õ', 'ô', 'ồ', 'ố', 'ộ', 'ổ', 'ỗ', 'ơ', 'ờ', 'ớ', 'ợ', 'ở', 'ỡ':
			return 'o'
		case 'ù', 'ú', 'ụ', 'ủ', 'ũ', 'ư', 'ừ', 'ứ', 'ự', 'ử', 'ữ':
			return 'u'
		case 'ỳ', 'ý', 'ỵ', 'ỷ', 'ỹ':
			return 'y'
		case 'đ':
			return 'd'
		default:
			return r
		}
	}, value)
}

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
