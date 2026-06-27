package system

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// buildOutboundUserText composes the operator-facing result line for an outbound
// attempt from its verified/outcome classification. Pure — no DB. msgType drives
// the "tab kept open" suffix for comment runs. Extracted verbatim from
// NotifyOutboundStatusDetail; behavior (copy, emoji, branch order) is unchanged.
func buildOutboundUserText(verified bool, outcome models.VerificationOutcome, typeVi, target, acctPart, detail, msgType string) string {
	var userText string
	switch {
	case verified:
		userText = fmt.Sprintf("%s ✅ Đã %s lead \"%s\"%s.", notifierPrefix, typeVi, target, acctPart)
	case outcome == models.VerifSubmittedUnverified || strings.Contains(strings.ToLower(string(outcome)), "optimistic"):
		// Submitted ≠ Verified: clicked send but no verified proof yet. Info, NOT success.
		userText = fmt.Sprintf("%s ℹ️ Đã gửi %s cho lead \"%s\"%s nhưng CHƯA xác minh được comment đã xuất hiện trên Facebook.", notifierPrefix, typeVi, target, acctPart)
	case outcome != "": // a terminal verification outcome that is not success → failure
		userText = fmt.Sprintf("%s ⚠️ %s thất bại cho lead \"%s\"%s — %s.", notifierPrefix, capitalizeFirst(typeVi), target, acctPart, friendlyOutboundReason(detail, string(outcome)))
	default:
		userText = fmt.Sprintf("%s Đang %s lead \"%s\"%s.", notifierPrefix, typeVi, target, acctPart)
	}
	// Window Respect (PR-2): comment tabs are kept open — tell the operator so they
	// know they can go inspect the result/error on the live page.
	if msgType == "comment" {
		if verified {
			userText += " Tab Facebook được giữ lại để bạn kiểm tra."
		} else if outcome != "" && outcome != models.VerifSubmittedUnverified {
			userText += " Tab Facebook được giữ lại để bạn kiểm tra lỗi."
		}
	}
	return userText
}
