package main

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/server/crawl"
	"github.com/thg/scraper/internal/store/connectors"
)

// TestCommentReadinessDecision asserts §5 block policy: a ready account does NOT
// block (the run proceeds to queue), and every non-ready verdict blocks with an
// actionable Vietnamese message that carries the shared preflight detail.
func TestCommentReadinessDecision(t *testing.T) {
	// Ready → not blocked, no message (the comment run continues to the queue).
	if msg, blocked := commentReadinessDecision(crawl.ReadinessReady, ""); blocked || msg != "" {
		t.Fatalf("ready must not block, got blocked=%v msg=%q", blocked, msg)
	}

	// Every non-ready reason must block and surface the actionable detail.
	cases := []struct{ name, reason, detail string }{
		{"no connector", crawl.ReasonConnectorOffline, "Mở Chrome profile đã pair account này"},
		{"identity unknown", crawl.ReasonActorIdentityUnknown, "chưa đọc được c_user"},
		{"actor mismatch", crawl.ReasonActorMismatchBlocked, "đăng nhập một Facebook KHÁC"},
		{"update required", connectors.ConnExtensionUpdateRequired, "Cập nhật extension"},
		{"unsupported", connectors.ConnExtensionUnsupported, "không còn được hỗ trợ"},
		{"not owned", crawl.ReasonAccountNotOwned, "Bạn không sở hữu"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg, blocked := commentReadinessDecision(c.reason, c.detail)
			if !blocked {
				t.Fatalf("reason %q must block the comment run", c.reason)
			}
			if !strings.Contains(msg, "Facebook") || !strings.Contains(msg, "comment") {
				t.Errorf("block message must name Facebook + comment context, got %q", msg)
			}
			if !strings.Contains(msg, c.detail) {
				t.Errorf("block message must carry the actionable detail %q, got %q", c.detail, msg)
			}
		})
	}
}

// TestCommentReadinessBlockNoDetail: with no detail, the headline still tells the
// operator to connect Facebook before running comment (never implies posting).
func TestCommentReadinessBlockNoDetail(t *testing.T) {
	msg := commentReadinessBlock("   ")
	if strings.Contains(msg, "Chi tiết") {
		t.Errorf("empty detail must not append a 'Chi tiết' clause, got %q", msg)
	}
	if !strings.Contains(msg, "kết nối Facebook") {
		t.Errorf("headline must instruct to connect Facebook, got %q", msg)
	}
}
