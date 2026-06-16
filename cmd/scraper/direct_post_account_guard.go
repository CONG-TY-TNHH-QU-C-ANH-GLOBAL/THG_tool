package main

import (
	"log"
	"strings"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// Live-connector account resolution + fail-closed guard for explicit direct-post comment
// (P1.3D). It enforces the invariant
//
//	active_connector_fb_identity -> resolved_account == selected == workflow == import == comment
//
// by resolving the Facebook account from the LIVE Chrome connector identity and refusing
// (fail closed) when identity is missing, ambiguous, or conflicts with the selected account.
// It NEVER first-ready-picks an account for a risky direct-post command — that silent
// fallback is exactly what let a comment run from the wrong Facebook identity.

// Customer-facing block messages (Vietnamese; no secrets/ids beyond what the user chose).
const (
	msgDPAccountNoLiveIdentity = "Chưa xác định được tài khoản Facebook đang đăng nhập trong Chrome. Hãy mở/kết nối lại Chrome profile rồi gửi lại lệnh."
	msgDPAccountAmbiguous      = "Có nhiều tài khoản Facebook đang online. Hãy chọn rõ tài khoản Facebook để comment bài này."
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

// resolveDirectPostAccount resolves the account an explicit direct-post comment must run on,
// using ONLY live connector identity. selectedAccountID is the dashboard selection (0 = none).
// It never falls back to first-ready.
func resolveDirectPostAccount(db *store.Store, orgID, selectedAccountID int64) directPostAccountResolution {
	if db == nil || orgID <= 0 {
		return directPostAccountResolution{message: msgDPAccountLookupError}
	}
	conns, err := db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return directPostAccountResolution{message: msgDPAccountLookupError}
	}
	policy, _ := db.Connectors().GetExtensionPolicy()

	// Explicit selection → it must be backed by a live, identity-matched, ready connector.
	if selectedAccountID > 0 {
		acc, _ := db.Identities().GetAccountForOrg(selectedAccountID, orgID)
		if acc == nil {
			return directPostAccountResolution{message: msgDPAccountNotFound}
		}
		_, reason := connectors.PickReadyConnector(conns, selectedAccountID, acc.FBUserID, policy)
		if reason != connectors.ConnReady {
			return directPostAccountResolution{message: connectorBlockMessage(reason)}
		}
		return directPostAccountResolution{accountID: selectedAccountID, ok: true}
	}

	// No selection → resolve from the live connectors, CONFIDENT ONLY: a UNIQUE online +
	// identity-matched + ready account. Zero → no identity; multiple → ambiguous.
	matched := liveReadyAccountIDs(conns, policy, func(fb string) int64 {
		if acc, _ := db.Identities().GetAccountByFacebookIdentity(orgID, fb); acc != nil {
			return acc.ID
		}
		return 0
	})
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

// guardDirectPostAccount resolves+validates the account for a direct-post comment and, on
// success, PINS it into args["account_id"] so workflow/import/comment all use the same
// identity-verified account. On block it returns (message, true) for the caller to return —
// NO workflow/import/outbound is created.
func guardDirectPostAccount(db *store.Store, args map[string]any) (string, bool) {
	orgID := argInt64(args, "org_id")
	selected := argInt64(args, "account_id")
	res := resolveDirectPostAccount(db, orgID, selected)
	if !res.ok {
		log.Printf("[DirectPostAccount] BLOCK org=%d selected_account=%d resolved_account=0 reason=%q", orgID, selected, res.message)
		return res.message, true
	}
	log.Printf("[DirectPostAccount] org=%d selected_account=%d resolved_account=%d live_identity_matched=true", orgID, selected, res.accountID)
	args["account_id"] = res.accountID
	return "", false
}
