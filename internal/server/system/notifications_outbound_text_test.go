package system

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// buildOutboundUserText must branch on the verified/outcome classification and
// append the "tab kept open" suffix only for comment runs (and never for the
// submitted-unverified info case).
func TestBuildOutboundUserText(t *testing.T) {
	const tabOK = "Tab Facebook được giữ lại để bạn kiểm tra."
	const tabErr = "Tab Facebook được giữ lại để bạn kiểm tra lỗi."

	verified := buildOutboundUserText(true, models.VerifVerifiedSuccess, "comment", "Lead A", " (Account #1)", "", "comment")
	if !strings.Contains(verified, "✅ Đã") || !strings.Contains(verified, tabOK) {
		t.Fatalf("verified comment: want success + tab-kept suffix, got %q", verified)
	}

	submitted := buildOutboundUserText(false, models.VerifSubmittedUnverified, "comment", "Lead A", "", "", "comment")
	if !strings.Contains(submitted, "ℹ️") || !strings.Contains(submitted, "CHƯA xác minh") {
		t.Fatalf("submitted-unverified: want info copy, got %q", submitted)
	}
	if strings.Contains(submitted, "Tab Facebook") {
		t.Fatalf("submitted-unverified must NOT get a tab suffix, got %q", submitted)
	}

	failed := buildOutboundUserText(false, models.VerificationOutcome("blocked_by_guard"), "comment", "Lead A", "", "boom", "comment")
	if !strings.Contains(failed, "⚠️") || !strings.Contains(failed, "thất bại") || !strings.Contains(failed, tabErr) {
		t.Fatalf("failure comment: want failure + tab-error suffix, got %q", failed)
	}

	inProgress := buildOutboundUserText(false, "", "comment", "Lead A", "", "", "inbox")
	if !strings.Contains(inProgress, "Đang") || strings.Contains(inProgress, "Tab Facebook") {
		t.Fatalf("default/in-progress non-comment: want progress copy + no tab suffix, got %q", inProgress)
	}
}
