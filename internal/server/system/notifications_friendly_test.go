package system

import (
	"strings"
	"testing"
)

// Failure reasons must read as plain Vietnamese — never the raw code — and unknown
// codes fall back without leaking the code (#8).
func TestFriendlyOutboundReason(t *testing.T) {
	cases := map[string]string{
		"target_not_reached":               "không mở được đúng bài viết Facebook",
		"context_drift":                    "Facebook chuyển trang trước khi gửi comment",
		"connector_offline":                "Chrome profile chưa kết nối",
		"actor_mismatch_blocked":           "đăng nhập nhầm Facebook",
		"comment_quality_invalid":          "comment không đạt kiểm tra chất lượng",
		"comment_required_website_missing": "comment thiếu/sai thông tin liên hệ theo chính sách",
	}
	for code, want := range cases {
		if got := friendlyOutboundReason(code, code); got != want {
			t.Errorf("friendlyOutboundReason(%q) = %q, want %q", code, got, want)
		}
	}
	// Unknown code → friendly fallback, no raw code leakage.
	got := friendlyOutboundReason("some_future_code_42", "some_future_code_42")
	if strings.Contains(got, "_") || strings.Contains(got, "future") {
		t.Errorf("unknown reason leaked the raw code: %q", got)
	}
}

func TestOutboundTypeVi(t *testing.T) {
	if outboundTypeVi("comment") != "comment" || outboundTypeVi("inbox") != "nhắn tin" || outboundTypeVi("group_post") != "đăng bài" {
		t.Errorf("outboundTypeVi mapping wrong")
	}
}

func TestCapitalizeFirst_Multibyte(t *testing.T) {
	if got := capitalizeFirst("đăng bài"); got != "Đăng bài" {
		t.Errorf("capitalizeFirst('đăng bài') = %q, want 'Đăng bài'", got)
	}
	if got := capitalizeFirst("comment"); got != "Comment" {
		t.Errorf("capitalizeFirst('comment') = %q, want 'Comment'", got)
	}
}
