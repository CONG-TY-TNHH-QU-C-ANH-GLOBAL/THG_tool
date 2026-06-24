package facebook

import (
	"fmt"
	"strings"
)

// Facebook outbound-result presentation: pure copy formatters for the operator/
// customer-facing summary of a queued FB outbound run. Moved verbatim out of
// cmd/scraper/outbound_actions.go (PR29F harvest) so the FB-specific result copy
// lives with the FB service. Pure functions — no store, no queue, no action
// ledger; they only render strings from already-computed counts/reason codes.

// FriendlySkipReasons summarizes skip reason codes for a customer-facing message.
// Each reason renders as "<friendly> [<raw_code>] ×<n>" so the exact gate stays
// unambiguous for forensics; an unknown code degrades to "cần kiểm tra" (never
// crashes) and an empty map degrades honestly.
func FriendlySkipReasons(reasons map[string]int) string {
	if len(reasons) == 0 {
		return "không đủ điều kiện"
	}
	label := map[string]string{
		"no_target_url":                  "thiếu link bài viết",
		"missing_target_url":             "thiếu link bài viết",
		"empty_content":                  "không soạn được nội dung",
		"generation_failed":              "lỗi soạn nội dung",
		"comment_quality_invalid":        "không đạt kiểm tra chất lượng",
		"comment_quality_empty":          "nội dung rỗng sau khi xử lý",
		"comment_quality_too_long":       "nội dung quá dài (vượt giới hạn)",
		"comment_quality_placeholder":    "còn chứa tên giữ chỗ (anonymous participant)",
		"comment_quality_duplicate_text": "nội dung bị lặp",
		"comment_multiple_urls":          "comment có nhiều liên kết",
		"comment_unsupported_contact":    "comment có liên hệ chưa xác minh",
		"account_cooldown_active":        "tài khoản đang nghỉ an toàn",
		"daily_limit_exceeded":           "đã đạt giới hạn hôm nay",
		"risk_ceiling_exceeded":          "tài khoản đang ở chế độ bảo vệ",
		"actor_mismatch_blocked":         "tài khoản đăng nhập nhầm Facebook",
		// Coordination / dedup guards — the common "0 queued" causes.
		"outbound_cooldown_active":       "đã gửi tới lead này gần đây (chờ hết 24h)",
		"duplicate_outbound_target_race": "đã có comment đang xếp hàng cho bài này",
		"awaiting_reply_cooldown":        "đang chờ lead phản hồi lần trước",
		"lead_replied":                   "lead đã trả lời — không gửi thêm",
		"conversation_closed":            "hội thoại với lead đã đóng",
		// Multi-actor coverage gate (brand coverage, not spam).
		"already_commented_by_this_actor": "tài khoản này đã comment lead này",
		"single_actor_policy":             "chính sách chỉ 1 tài khoản/lead",
		"coverage_full":                   "lead đã đủ số tài khoản tiếp cận",
		"coverage_gap_too_soon":           "chưa đủ giãn cách giữa các lượt comment",
		"action_policy_missing":           "workspace chưa bật chính sách hành động",
		// Target-URL resolution (resolveOutboundTargetURL) — the common skip for a
		// fresh lead whose crawled URL is not a direct commentable post permalink.
		"missing_post_permalink": "lead chưa có link bài viết comment được (URL không phải permalink bài post)",
		"missing_target":         "lead thiếu link nguồn",
		"unrouted_source_type":   "loại nguồn của lead không hỗ trợ comment",
	}
	parts := make([]string, 0, len(reasons))
	for code, n := range reasons {
		name := label[code]
		if name == "" {
			name = "cần kiểm tra"
		}
		// Incident forensics: keep the raw code in brackets so the exact skip gate is
		// unambiguous in the copilot message (no guessing which guard fired).
		parts = append(parts, fmt.Sprintf("%s [%s] ×%d", name, code, n))
	}
	return strings.Join(parts, ", ")
}

// FormatOutboundNotification renders the operator-facing run notification. The
// DEFAULT (non-auto) mode says "drafts waiting for approval" (approval-required
// is the visible default); approved_auto flips only the state clause; the channel
// label varies by msgType and an unknown msgType falls back to "outbound".
func FormatOutboundNotification(orgID, accountID int64, msgType string, queued, skipped int, mode string) string {
	label := "outbound"
	switch msgType {
	case "comment":
		label = "Facebook comments"
	case "inbox":
		label = "Facebook inbox"
	case "group_post":
		label = "Facebook posting"
	case "profile_post":
		label = "Facebook profile posting"
	}
	state := "drafts waiting for approval"
	if mode == "approved_auto" {
		state = "approved for Chrome Extension execution"
	}
	return fmt.Sprintf("[THG Agent] %s queued: %d (%s). Org #%d, account #%d, skipped %d by guardrails.", label, queued, state, orgID, accountID, skipped)
}
