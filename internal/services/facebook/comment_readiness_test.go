package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/readiness"
)

// fakeCommentReadiness is a store-free CommentReadinessEvaluator: it returns a
// canned (reason, detail) so the gate can be exercised without a real store.
type fakeCommentReadiness struct{ reason, detail string }

func (f fakeCommentReadiness) EvaluateCommentReadiness(_ context.Context, _, _ int64, _ string, _ int64) (string, string) {
	return f.reason, f.detail
}

// TestCommentReadinessGate_Delegates pins the port wiring: a ready verdict lets
// the run proceed (not blocked, no message); any non-ready verdict blocks and
// the actionable detail flows through into the block message.
func TestCommentReadinessGate_Delegates(t *testing.T) {
	if msg, blocked := CommentReadinessGate(context.Background(), fakeCommentReadiness{reason: readiness.ReadinessReady}, 1, 2, "admin", 3); blocked || msg != "" {
		t.Fatalf("ready must not block, got blocked=%v msg=%q", blocked, msg)
	}
	msg, blocked := CommentReadinessGate(context.Background(), fakeCommentReadiness{reason: readiness.ReasonConnectorOffline, detail: "DETAIL-MARKER"}, 1, 2, "admin", 3)
	if !blocked {
		t.Fatalf("non-ready verdict must block the comment run")
	}
	if !strings.Contains(msg, "DETAIL-MARKER") {
		t.Fatalf("gate must carry the evaluator detail into the block message, got %q", msg)
	}
}

// TestCommentReadinessDecision asserts §5 block policy: a ready account does NOT
// block (the run proceeds to queue), and every non-ready verdict blocks with an
// actionable Vietnamese message that carries the shared preflight detail.
func TestCommentReadinessDecision(t *testing.T) {
	// Ready → not blocked, no message (the comment run continues to the queue).
	if msg, blocked := commentReadinessDecision(readiness.ReadinessReady, ""); blocked || msg != "" {
		t.Fatalf("ready must not block, got blocked=%v msg=%q", blocked, msg)
	}

	// Every non-ready reason must block and surface the actionable detail. The
	// extension reasons use the real connector reason-code values
	// (internal/store/connectors version_policy) as literals so this test stays
	// store-free; commentReadinessDecision blocks on ANY non-ready reason.
	cases := []struct{ name, reason, detail string }{
		{"no connector", readiness.ReasonConnectorOffline, "Mở Chrome profile đã pair account này"},
		{"identity unknown", readiness.ReasonActorIdentityUnknown, "chưa đọc được c_user"},
		{"actor mismatch", readiness.ReasonActorMismatchBlocked, "đăng nhập một Facebook KHÁC"},
		{"update required", "extension_update_required", "Cập nhật extension"},
		{"unsupported", "extension_unsupported", "không còn được hỗ trợ"},
		{"not owned", readiness.ReasonAccountNotOwned, "Bạn không sở hữu"},
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
