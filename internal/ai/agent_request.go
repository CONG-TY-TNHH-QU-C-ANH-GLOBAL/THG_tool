package ai

import (
	"regexp"
	"strconv"
	"strings"
)

func wantsAutoOutbound(prompt string) bool {
	lower := strings.ToLower(prompt)
	triggers := []string{
		"gửi luôn", "gui luon", "chạy luôn", "chay luon", "tự động", "tu dong",
		"không cần duyệt", "khong can duyet", "auto", "automation hết", "automation het",
		"comment lên", "comment len", "inbox leads", "inbox tất cả", "inbox tat ca",
		"post lên", "post len", "đăng lên", "dang len", "posting",
	}
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// shouldAutoOutbound combines per-prompt opt-in with the org-level
// outbound_mode policy. The store layer is the final guardrail (it
// downgrades to draft when the org has not opted in), so it is safe to
// upgrade here when either signal is present. This fixes the case where
// admin sets outbound_mode=auto on the policy panel but the agent kept
// queuing drafts because the user prompt did not contain magic words like
// "tự động" / "auto".
func (a *Agent) shouldAutoOutbound(prompt string, orgID int64) bool {
	if wantsAutoOutbound(prompt) {
		return true
	}
	if a.db != nil && orgID > 0 && a.db.IsAutoOutboundEnabledForOrg(orgID) {
		return true
	}
	return false
}

func stripDashboardContext(prompt string) string {
	marker := "\n\nDashboard context:"
	if idx := strings.Index(prompt, marker); idx >= 0 {
		return strings.TrimSpace(prompt[:idx])
	}
	return strings.TrimSpace(prompt)
}

func extractDashboardAccountID(prompt string) int64 {
	re := regexp.MustCompile(`account_id\s*=\s*(\d+)`)
	m := re.FindStringSubmatch(prompt)
	if len(m) < 2 {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

func requiresFacebookBrowser(prompt string) bool {
	lower := strings.ToLower(stripDashboardContext(prompt))
	if strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com") {
		return true
	}
	triggers := []string{
		"cào", "cao ", "crawl", "scrape", "quét", "quet ",
		"tìm tệp", "tim tep", "tệp khách", "tep khach", "tìm khách", "tim khach",
		"lead", "leads", "group", "nhóm", "nhom",
		"comment", "bình luận", "binh luan", "inbox", "messenger",
		"đăng bài", "dang bai", "posting", "post lên", "post len",
	}
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

func facebookScopePreflight(prompt string) (bool, string) {
	prompt = strings.TrimSpace(stripDashboardContext(prompt))
	if prompt == "" {
		return false, facebookScopeGuardMessage()
	}
	if isBusinessContextPrompt(prompt) || requiresFacebookBrowser(prompt) || isFacebookConversationPrompt(prompt) {
		return true, ""
	}
	return false, facebookScopeGuardMessage()
}

func isFacebookConversationPrompt(prompt string) bool {
	lower := strings.ToLower(stripDashboardContext(prompt))
	folded := foldVietnameseForMatch(lower)
	if strings.Contains(lower, "facebook") || strings.Contains(lower, "meta") || strings.Contains(lower, "fb ") || strings.Contains(lower, "fb.") {
		return true
	}
	triggers := []string{
		"fanpage", "page", "profile", "personal profile", "group", "groups", "messenger",
		"reels", "story", "post", "posting", "comment", "comments", "inbox", "dm",
		"lead", "leads", "seller", "sellers", "outreach", "campaign", "content plan",
		"browser", "workspace", "session", "chrome extension", "browser gateway", "chrome", "cookie",
		"checkpoint", "captcha", "login", "automation", "crawler", "scraper", "crawl",
		"telegram", "tele", "data private", "market signal", "customer segment",
		"fanpage", "trang ca nhan", "trang cá nhân", "nhom", "nhóm", "bai viet", "bài viết",
		"binh luan", "bình luận", "tin nhan", "tin nhắn", "khach hang", "khách hàng",
		"tep khach", "tệp khách", "tim khach", "tìm khách", "chot sale", "chốt sale",
		"tuong tac", "tương tác", "nguon", "nguồn", "crawl", "cao", "cào",
	}
	for _, trigger := range triggers {
		if strings.Contains(lower, trigger) || strings.Contains(folded, foldVietnameseForMatch(trigger)) {
			return true
		}
	}
	return false
}
