package connector

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/connectors"
	"github.com/thg/scraper/internal/telegram/render"
)

// extension_update_required alerting (PR-8): when a connector reports a
// BLOCKED extension build (update_required / unsupported), notify the
// owning staff member (personal in-app), the workspace admins
// (org-wide in-app) and the org's Telegram channels — rate-limited to
// once per connector per 24h so a 30-second heartbeat can never spam.
// Heartbeats are the trigger, never the message bus.

const extensionAlertInterval = 24 * time.Hour

var (
	extensionAlertMu   sync.Mutex
	extensionAlertLast = map[string]time.Time{} // "org:connector" → last alert
)

func extensionAlertDue(orgID, agentID int64) bool {
	extensionAlertMu.Lock()
	defer extensionAlertMu.Unlock()
	key := fmt.Sprintf("%d:%d", orgID, agentID)
	if last, ok := extensionAlertLast[key]; ok && time.Since(last) < extensionAlertInterval {
		return false
	}
	extensionAlertLast[key] = time.Now()
	return true
}

// resolveExtensionAlertContext looks up the owning staff member + the account the
// outdated connector serves, for the actionable alert copy. Best-effort: any lookup
// failure leaves the corresponding name blank. Behavior identical to the inline
// block it replaced — extracted only to keep maybeAlertExtensionOutdated under the
// complexity gate.
func (h *Handler) resolveExtensionAlertContext(orgID, agentID int64) (ownerID int64, staffName, accountName string) {
	if conns, lerr := h.db.Connectors().ListLocalConnectors(orgID); lerr == nil {
		for i := range conns {
			if conns[i].ID != agentID {
				continue
			}
			ownerID = conns[i].CreatedBy
			if acc, aerr := h.db.Identities().GetAccountForOrg(conns[i].AssignedAccountID, orgID); aerr == nil && acc != nil {
				accountName = acc.Name
				if acc.FBDisplayName != "" {
					accountName = acc.FBDisplayName
				}
			}
			break
		}
	}
	if owner, _ := h.db.GetUserByID(ownerID); owner != nil {
		staffName = owner.Name
	}
	return ownerID, staffName, accountName
}

// maybeAlertExtensionOutdated is called from the heartbeat handlers
// AFTER presence is persisted. Cheap early exits; all failures logged,
// never surfaced to the extension.
func (h *Handler) maybeAlertExtensionOutdated(orgID, agentID int64, version string) {
	if orgID <= 0 || agentID <= 0 {
		return
	}
	policy, err := h.db.Connectors().GetExtensionPolicy()
	if err != nil {
		return
	}
	state := connectors.EvaluateVersionState(version, policy)
	if connectors.VersionStateAllowsAutomation(state) {
		return
	}
	if !extensionAlertDue(orgID, agentID) {
		return
	}

	// Resolve owner + account for actionable copy.
	ownerID, staffName, accountName := h.resolveExtensionAlertContext(orgID, agentID)

	payload, _ := json.Marshal(map[string]string{"version": version, "state": state})
	staffBody := "Automation đang tạm dừng vì extension của bạn đã cũ. Cập nhật extension để tiếp tục nhận task."
	if state == connectors.VersionStateUnsupported {
		staffBody = "Phiên bản extension này không còn được hỗ trợ. Vui lòng cài phiên bản mới."
	}
	if ownerID > 0 {
		if err := h.db.InsertNotification(orgID, ownerID, models.NotificationExtensionUpdateRequired,
			"Cần cập nhật extension", staffBody, string(payload)); err != nil {
			log.Printf("[ExtensionAlert] staff notification failed org=%d: %v", orgID, err)
		}
	}
	if err := h.db.InsertNotification(orgID, 0, models.NotificationExtensionUpdateRequired,
		"Connector cần cập nhật extension",
		"Automation paused until staff updates extension. ("+staffName+" / "+accountName+")",
		string(payload)); err != nil {
		log.Printf("[ExtensionAlert] admin notification failed org=%d: %v", orgID, err)
	}
	if h.tgEvents != nil {
		_, _ = h.tgEvents.NotifyEvent(orgID, "extension_update_required", "facebook",
			render.ExtensionUpdateRequired("", staffName, accountName, version, state))
	}
}
