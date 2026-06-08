package crawl

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Facebook Automation Reliability Track — PR-A (Mission Preflight).
// See specs/FACEBOOK_AUTOMATION_RELIABILITY_TRACK.md.

// minCrawlExtensionVersion is the lowest Chrome-extension version allowed to run
// a crawl. Bump when an automation-breaking fix ships.
const minCrawlExtensionVersion = "0.5.26"

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
// typed reason code + an actionable message. Mirrors the run-time connector
// checks (pickOnlineConnectorForCrawl) so create-time and run-time agree.
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
	expectedFB := strings.TrimSpace(acc.FBUserID)
	sawAssignedConnector := false
	for i := range conns {
		c := conns[i]
		if c.AssignedAccountID > 0 && c.AssignedAccountID != accountID {
			continue // a connector bound to a different account
		}
		sawAssignedConnector = true
		if !c.Online {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(c.StreamStatus), browsergateway.StreamFacebookLoggedIn) {
			continue
		}
		// Online + logged in: now identity must be known and correct.
		connFB := strings.TrimSpace(c.FBUserID)
		if connFB == "" {
			return ReasonActorIdentityUnknown, "Chrome Extension của account này chưa xác định được danh tính Facebook (chưa đọc được c_user). Mở tab Facebook đã đăng nhập rồi thử lại."
		}
		if expectedFB != "" && connFB != expectedFB {
			return ReasonActorMismatchBlocked, "Chrome Extension đang đăng nhập một Facebook KHÁC với account này — hãy đăng nhập đúng tài khoản."
		}
		if !extensionVersionAtLeast(c.Version, minCrawlExtensionVersion) {
			return ReasonExtensionVersionOutdated, fmt.Sprintf("Chrome Extension đã cũ (%s) — hãy cập nhật lên phiên bản ≥ %s.", textOrUnknown(c.Version), minCrawlExtensionVersion)
		}
		return ReadinessReady, ""
	}
	if sawAssignedConnector {
		return ReasonConnectorOffline, fmt.Sprintf("Account #%d đã có Chrome Extension nhưng chưa online + đăng nhập Facebook. Mở Chrome profile đã pair account này và đăng nhập Facebook, rồi thử lại.", accountID)
	}
	return ReasonConnectorOffline, fmt.Sprintf("Account #%d chưa có Chrome Extension online. Mở Browser, pair Chrome Extension cho account này, giữ một tab Facebook đã đăng nhập, rồi thử lại.", accountID)
}

// extensionVersionAtLeast compares dotted version strings numerically segment by
// segment ("0.5.29.878" >= "0.5.26" → true). Missing trailing segments count as 0.
func extensionVersionAtLeast(v, min string) bool {
	va := strings.Split(strings.TrimSpace(v), ".")
	vb := strings.Split(strings.TrimSpace(min), ".")
	for i := 0; i < len(vb); i++ {
		bPart, _ := strconv.Atoi(vb[i])
		aPart := 0
		if i < len(va) {
			aPart, _ = strconv.Atoi(va[i])
		}
		if aPart != bPart {
			return aPart > bPart
		}
	}
	return true
}

func textOrUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "không rõ"
	}
	return s
}
