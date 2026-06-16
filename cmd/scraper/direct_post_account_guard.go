package main

import (
	"log"
	"strings"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// Live-connector account resolution + fail-closed guard for Facebook WRITE actions (P1.3D):
// direct-post comment, bulk comment, inbox messages, group posts, profile posts. It enforces
//
//	active_connector_fb_identity -> resolved_account == selected == workflow/action account
//
// by resolving the Facebook account from the LIVE Chrome connector identity and refusing
// (fail closed) when identity is missing, ambiguous, or conflicts with the selected account.
// It NEVER first-ready-picks an account for a write action — that silent fallback is exactly
// what let a comment/post run from the wrong Facebook identity. Broad read/crawl/search
// actions are NOT guarded (they create no public side effect) and keep their own fallback.

// Customer-facing block messages (Vietnamese; no secrets/ids beyond what the user chose).
const (
	msgDPAccountNoLiveIdentity = "Chưa xác định được tài khoản Facebook đang đăng nhập trong Chrome. Hãy mở/kết nối lại Chrome profile rồi gửi lại lệnh."
	msgDPAccountAmbiguous      = "Có nhiều tài khoản Facebook đang online. Hãy chọn rõ tài khoản Facebook để thực hiện hành động này."
	msgDPAccountMismatch       = "Tài khoản đã chọn không khớp với Facebook đang đăng nhập trong Chrome. Hãy chọn đúng tài khoản hoặc mở Chrome đúng profile rồi thử lại."
	msgDPAccountOffline        = "Tài khoản đã chọn chưa có kết nối Chrome online. Hãy mở Chrome và kết nối lại tài khoản đó."
	msgDPAccountUnknown        = "Chưa đọc được danh tính Facebook từ Chrome. Hãy đợi đồng bộ vài giây hoặc kết nối lại Chrome profile."
	msgDPAccountVersion        = "Tiện ích Chrome cần được cập nhật để chạy. Hãy cập nhật extension rồi thử lại."
	msgDPAccountLookupError    = "Không kiểm tra được trạng thái kết nối Chrome. Hãy thử lại sau giây lát."
	msgDPAccountNotFound       = "Tài khoản Facebook đã chọn không tồn tại trong workspace."
)

// directPostAccountResolution is the verdict: a resolved account to act on, or a blocked
// message to return to the user (fail closed).
type directPostAccountResolution struct {
	accountID int64
	ok        bool
	message   string
}

// resolveDirectPostAccount resolves the SINGLE account an explicit direct-post / single-task
// write action must run on, using ONLY live connector identity, restricted to accounts the
// requester can CONTROL (PR-1). selectedAccountID is the dashboard selection (0 = none);
// requesterUserID/role are the caller identity (0 = legacy/unauthenticated → org-scoped).
// It never falls back to first-ready and never resolves another member's account.
func resolveDirectPostAccount(db *store.Store, orgID, selectedAccountID, requesterUserID int64) directPostAccountResolution {
	if db == nil || orgID <= 0 {
		return directPostAccountResolution{message: msgDPAccountLookupError}
	}
	if requesterUserID <= 0 {
		// A Facebook WRITE side-effect requires a proven requester — never resolve org-wide.
		return directPostAccountResolution{message: msgDPRequesterRequired}
	}
	conns, err := db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return directPostAccountResolution{message: msgDPAccountLookupError}
	}
	conns = controllableConnectors(conns, requesterUserID) // PR-1: requester-owned connectors only
	policy, _ := db.Connectors().GetExtensionPolicy()

	// Explicit selection → requester must CONTROL it AND it must be backed by a live,
	// identity-matched, ready connector the requester owns. A non-controllable selection
	// (another member's account, or a brain/first-ready pre-pick the requester does not own)
	// is blocked — never silently honoured.
	if selectedAccountID > 0 {
		acc, _ := db.Identities().GetAccountForOrg(selectedAccountID, orgID)
		if acc == nil {
			return directPostAccountResolution{message: msgDPAccountNotFound}
		}
		if !canRequesterControlAccount(acc, requesterUserID) {
			return directPostAccountResolution{message: msgDPAccountNotControllable}
		}
		_, reason := connectors.PickReadyConnector(conns, selectedAccountID, acc.FBUserID, policy)
		if reason != connectors.ConnReady {
			return directPostAccountResolution{message: connectorBlockMessage(reason)}
		}
		return directPostAccountResolution{accountID: selectedAccountID, ok: true}
	}

	// No selection → resolve from the requester-controllable live connectors, CONFIDENT ONLY:
	// a UNIQUE online + identity-matched + ready account the requester owns. Zero → no identity;
	// multiple → ambiguous. Other members' live accounts are never counted (PR-1).
	matched := liveReadyControllableAccountIDs(db, conns, policy, orgID, requesterUserID)
	switch len(matched) {
	case 1:
		return directPostAccountResolution{accountID: matched[0], ok: true}
	case 0:
		return directPostAccountResolution{message: msgDPAccountNoLiveIdentity}
	default:
		return directPostAccountResolution{message: msgDPAccountAmbiguous}
	}
}

// liveReadyAccountIDs (pure over conns) returns the DISTINCT account ids whose LIVE connector
// is online, logged in, identity-matched, and version-ready. fbToAccount maps a live
// fb_user_id to its account id (0 = unknown). Because the account is resolved BY the live
// fb_user_id, that id is the account's expected identity, so PickReadyConnector confirms the
// full readiness (online + logged_in + version) for it.
func liveReadyAccountIDs(conns []connectors.AgentToken, policy connectors.VersionPolicy, fbToAccount func(fb string) int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for i := range conns {
		c := conns[i]
		if !c.Online {
			continue
		}
		fb := strings.TrimSpace(c.FBUserID)
		if fb == "" {
			continue
		}
		accID := fbToAccount(fb)
		if accID == 0 || seen[accID] {
			continue
		}
		if _, reason := connectors.PickReadyConnector(conns, accID, fb, policy); reason == connectors.ConnReady {
			seen[accID] = true
			out = append(out, accID)
		}
	}
	return out
}

// connectorBlockMessage maps a PickReadyConnector reason to a customer-facing message.
func connectorBlockMessage(reason string) string {
	switch reason {
	case connectors.ConnIdentityMismatch:
		return msgDPAccountMismatch
	case connectors.ConnIdentityUnknown:
		return msgDPAccountUnknown
	case connectors.ConnOffline:
		return msgDPAccountOffline
	default:
		// version-gate reasons (ConnExtensionUpdateRequired / Unsupported / Outdated)
		return msgDPAccountVersion
	}
}

// guardFacebookWriteAccount resolves+validates the account for a Facebook WRITE action and,
// on success, PINS it into args["account_id"] so the whole chain (workflow/import/comment or
// post/inbox) uses the same identity-verified account. On block it returns (message, true)
// for the caller to return — NO workflow / outbound / post job is created.
func guardFacebookWriteAccount(db *store.Store, args map[string]any) (string, bool) {
	orgID := argInt64(args, "org_id")
	selected := argInt64(args, "account_id")
	requester := argInt64(args, "user_id")
	res := resolveDirectPostAccount(db, orgID, selected, requester)
	if !res.ok {
		log.Printf("[DirectPostAccount] BLOCK org=%d requester=%d selected_account=%d resolved_account=0 reason=%q", orgID, requester, selected, res.message)
		return res.message, true
	}
	log.Printf("[DirectPostAccount] org=%d requester=%d selected_account=%d resolved_account=%d live_identity_matched=true control_ok=true", orgID, requester, selected, res.accountID)
	args["account_id"] = res.accountID
	return "", false
}
