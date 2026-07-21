package readiness

import (
	"context"
	"fmt"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// Facebook Automation Reliability Track — PR-A (Mission Preflight).
// See specs/domains/facebook-sales-intelligence/features/account-safety/implementation/reliability-track.md.
//
// Promoted from internal/server/crawl (PR29C): account readiness is a
// platform-neutral primitive (connector online + live identity + supported
// extension + ownership), shared by crawl create-time preflight AND the comment
// run preflight, with future FB/IG/TikTok/YT runners reusing the same decision.
// It depends only on models + store + store/connectors — never on transport.

// Crawl-readiness reason codes — a CLOSED set. UI and tests branch on these
// exact strings; messages are actionable Vietnamese.
const (
	ReadinessReady                 = "ready"
	ReasonAccountNotSelected       = "account_not_selected"
	ReasonAccountNotOwned          = "account_not_owned"
	ReasonConnectorOffline         = "connector_offline"
	ReasonActorIdentityUnknown     = "actor_identity_unknown"
	ReasonActorMismatchBlocked     = "actor_mismatch_blocked"
	ReasonExtensionVersionOutdated = "extension_version_outdated"
)

// EvaluateCrawlAccountReadiness preflights a user-created crawl mission against
// the foundational invariant: the system must know the chosen account, its
// connector, the live Facebook identity, and eligibility BEFORE anything runs —
// and it must NEVER silently fall back to another / first-ready account.
//
// Returns (ReadinessReady, "") when the account may run a crawl now, otherwise a
// typed reason code + an actionable message. The connector-eligibility decision
// is delegated to the SHARED connectors.PickReadyConnector so create-time
// (here) and run-time (cmd/scraper pickOnlineConnectorForCrawl) never diverge.
func EvaluateCrawlAccountReadiness(ctx context.Context, db *store.Store, orgID, userID int64, role string, accountID int64) (string, string) {
	if accountID <= 0 {
		return ReasonAccountNotSelected, "Bạn chưa chọn account để crawl. Hãy chọn account sẽ chạy nhiệm vụ này."
	}
	acc, err := db.Identities().GetAccountForOrg(accountID, orgID)
	if err != nil || acc == nil {
		return ReasonAccountNotOwned, "Account không tồn tại trong tổ chức của bạn."
	}
	if userID > 0 && !models.IsAccountOwnerAllowed(acc, userID, role) {
		return ReasonAccountNotOwned, fmt.Sprintf("Bạn không sở hữu account #%d.", accountID)
	}
	// Persisted Verified-Actor block (P1b): a mis-logged account is off-limits
	// until an operator clears it.
	if states, serr := db.Coordination().AccountActorStatesForOrg(ctx, orgID); serr == nil && states[accountID].Blocked {
		return ReasonActorMismatchBlocked, "Account này đang bị chặn vì Facebook identity mismatch — admin cần gỡ chặn trước khi chạy."
	}

	conns, _ := db.Connectors().ListLocalConnectors(orgID)
	policy, _ := db.Connectors().GetExtensionPolicy()
	_, connReason := connectors.PickReadyConnector(conns, accountID, acc.FBUserID, policy)
	switch connReason {
	case connectors.ConnReady:
		return ReadinessReady, ""
	case connectors.ConnIdentityUnknown:
		return ReasonActorIdentityUnknown, "Chrome Extension của account này chưa xác định được danh tính Facebook (chưa đọc được c_user). Mở tab Facebook đã đăng nhập rồi thử lại."
	case connectors.ConnIdentityMismatch:
		return ReasonActorMismatchBlocked, "Chrome Extension đang đăng nhập một Facebook KHÁC với account này — hãy đăng nhập đúng tài khoản."
	case connectors.ConnExtensionUpdateRequired:
		return connectors.ConnExtensionUpdateRequired, "Automation đang tạm dừng vì extension của bạn đã cũ. Cập nhật extension để tiếp tục nhận task."
	case connectors.ConnExtensionUnsupported:
		return connectors.ConnExtensionUnsupported, "Phiên bản extension này không còn được hỗ trợ. Vui lòng cài phiên bản mới."
	default: // ConnOffline
		return ReasonConnectorOffline, fmt.Sprintf("Account #%d chưa có Chrome Extension online + đăng nhập Facebook. Mở Chrome profile đã pair account này, giữ một tab Facebook đã đăng nhập, rồi thử lại.", accountID)
	}
}
