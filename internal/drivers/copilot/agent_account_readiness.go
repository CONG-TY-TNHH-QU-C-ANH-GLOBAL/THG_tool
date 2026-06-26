package copilot

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

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

// isRiskyDirectAccountAction reports whether an action is a Facebook WRITE action that must
// NOT silently first-ready-pick an account (P1.3D). These resolve their account from LIVE
// connector identity and fail closed downstream (guardFacebookWriteAccount), so picking an
// arbitrary ready account here would run a comment/post/message from the wrong identity.
// Broad read/crawl/search actions (scrape_group, scrape_comments, search_groups, recurring
// crawl) are NOT listed — they create no public side effect and keep their fallback.
func isRiskyDirectAccountAction(action string) bool {
	switch action {
	case "comment_single_post",
		"auto_comment", "comment_all_leads",
		"auto_inbox", "inbox_all_leads",
		"create_job_post", "post_to_profile":
		return true
	}
	return false
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
