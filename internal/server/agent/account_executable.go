package agent

import (
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/connectors"
)

// Requester-scoped EXECUTABILITY resolver (P1.3E). Green "Sẵn sàng" must mean the REQUESTER can
// run automation on THIS account right now — NOT that some fresh org-wide connector happens to
// match the account's FB identity. So executability is computed over the requester's OWN
// connectors (connectors.OwnedBy) + the strict control predicate (models.AccountControlAllowed),
// reusing the SAME PickReadyConnector evaluator the execution gate uses so the UI can never
// over-promise vs what execution will actually do. It is INDEPENDENT of the org-wide
// capabilities[].can projection, which is left untouched for backward compatibility.

// streamFacebookLoggedIn mirrors the connector "logged in" stream value (kept local to avoid a
// gateway import). sessionWallTokens flag a stream/error that is a usable-session blocker
// (login wall / checkpoint / logged out) vs a benign not-yet-ready state (idle / waiting pairing).
const agentStreamFacebookLoggedIn = "facebook_logged_in"

var sessionWallTokens = []string{"checkpoint", "logged_out", "loggedout", "login", "blocked", "challenge", "restricted"}

// execStatus is the decomposed verdict written onto AccountReadiness.
type execStatus struct {
	configured          bool
	controlAllowed      bool
	paired              bool
	connectorOnline     bool
	liveIdentityMatched bool
	sessionUsable       bool
	sessionWall         bool
	executable          bool
	reasonCode          string
}

// resolveAccountExecutable computes the requester-scoped executability of one account. ownedConns
// MUST already be filtered to the requester's connectors (connectors.OwnedBy). actorBlocked is the
// Verified-Actor hard block; accountActive is acc.Status == active.
func resolveAccountExecutable(acc models.Account, ownedConns []connectors.AgentToken, policy connectors.VersionPolicy, actorBlocked, accountActive bool, requesterUserID int64) execStatus {
	st := execStatus{
		configured:     strings.TrimSpace(acc.FBUserID) != "",
		controlAllowed: models.AccountControlAllowed(&acc, requesterUserID),
	}
	// Inspect the requester-owned connectors bound to / matching this account.
	for i := range ownedConns {
		c := ownedConns[i]
		boundToAcc := c.AssignedAccountID == acc.ID ||
			(acc.FBUserID != "" && strings.TrimSpace(c.FBUserID) == strings.TrimSpace(acc.FBUserID))
		if !boundToAcc {
			continue
		}
		st.paired = true
		if c.Online {
			st.connectorOnline = true
			if strings.EqualFold(strings.TrimSpace(c.StreamStatus), agentStreamFacebookLoggedIn) {
				st.sessionUsable = true
				if acc.FBUserID != "" && strings.TrimSpace(c.FBUserID) == strings.TrimSpace(acc.FBUserID) {
					st.liveIdentityMatched = true
				}
			} else if isSessionWall(c.StreamStatus, c.ChromeError) {
				st.sessionWall = true
			}
		}
	}

	// Authoritative readiness gate over OWNED connectors (same evaluator as execution).
	_, connReason := connectors.PickReadyConnector(ownedConns, acc.ID, acc.FBUserID, policy)
	accountBlocked := actorBlocked || !accountActive
	st.reasonCode = classifyExecReason(st, connReason, accountBlocked)
	st.executable = st.reasonCode == models.ExecReasonReady
	return st
}

// classifyExecReason maps the decomposed signals + the connector gate reason to one typed
// account-level reason code. Precedence: control → account block → connector liveness.
func classifyExecReason(st execStatus, connReason string, accountBlocked bool) string {
	switch {
	case !st.controlAllowed:
		return models.ExecReasonNotControllable
	case accountBlocked:
		return models.ExecReasonAccountBlocked
	}
	switch connReason {
	case connectors.ConnReady:
		return models.ExecReasonReady
	case connectors.ConnIdentityMismatch:
		return models.ExecReasonIdentityMismatch
	case connectors.ConnIdentityUnknown:
		return models.ExecReasonPairingPending
	case connectors.ConnExtensionUpdateRequired, connectors.ConnExtensionUnsupported:
		return connReason // typed version codes (FE maps via existing reasonMessages; safe default otherwise)
	default: // ConnOffline / anything else: distinguish by what owned connector state we observed
		switch {
		case !st.paired:
			return models.ExecReasonNoConnector
		case !st.connectorOnline:
			return models.ExecReasonConnectorStale
		case st.sessionWall:
			return models.ExecReasonSessionBlocked
		default:
			return models.ExecReasonPairingPending // online but not logged-in yet
		}
	}
}

func isSessionWall(streamStatus, chromeError string) bool {
	s := strings.ToLower(strings.TrimSpace(streamStatus) + " " + strings.TrimSpace(chromeError))
	for _, t := range sessionWallTokens {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// execReasonMessage is the short, secret-free Vietnamese message per typed code. The FE has its
// own labels (the canonical UI copy); this parallel message serves non-FE API consumers and a
// safe default for unmapped (e.g. version) codes.
func execReasonMessage(code string) string {
	switch code {
	case models.ExecReasonReady:
		return "Sẵn sàng"
	case models.ExecReasonNoConnector:
		return "Chưa kết nối Chrome cho tài khoản này."
	case models.ExecReasonConnectorStale:
		return "Mất kết nối Chrome (không còn nhịp tín hiệu)."
	case models.ExecReasonPairingPending:
		return "Đang chờ ghép nối / đăng nhập Facebook trong Chrome."
	case models.ExecReasonIdentityMismatch:
		return "Chrome đang đăng nhập Facebook khác với tài khoản này."
	case models.ExecReasonSessionBlocked:
		return "Phiên Facebook đang bị chặn/checkpoint hoặc đã đăng xuất."
	case models.ExecReasonAccountBlocked:
		return "Tài khoản đang bị chặn/tạm ngưng."
	case models.ExecReasonNotControllable:
		return "Bạn không có quyền điều khiển tài khoản này."
	default:
		return "Cần cập nhật tiện ích Chrome để chạy."
	}
}
