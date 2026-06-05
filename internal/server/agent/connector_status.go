package agent

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/connectors"
)

// connectorAccountStatus is one row of the PR-M2 presence board: for ONE
// Facebook account, who owns it, whether a connector is bound + online, and
// which FB identity is actually logged in. This turns the opaque aggregate
// "2 extensions online" into "which of my N accounts is reachable right now".
type connectorAccountStatus struct {
	AccountID         int64  `json:"account_id"`
	AccountName       string `json:"account_name"`
	AssignedUserID    int64  `json:"assigned_user_id"`
	AssignedUserName  string `json:"assigned_user_name"`
	AccountFBUserID      string `json:"account_fb_user_id"`
	AccountFBDisplayName string `json:"account_fb_display_name"`
	ConnectorID          int64  `json:"connector_id"`
	ConnectorName        string `json:"connector_name"`
	ConnectorOnline      bool   `json:"connector_online"`
	StreamStatus         string `json:"stream_status"`
	ConnectorFBUserID    string `json:"connector_fb_user_id"`
	// ConnectorFBDisplayName is the LIVE logged-in FB name reported by the
	// connector — the most accurate identity (account.name is often an
	// auto-generated placeholder like "Facebook 05/06"). The UI labels the row
	// by this when present.
	ConnectorFBDisplayName string `json:"connector_fb_display_name"`
	// Reachable = an online connector is bound AND logged into the SAME FB
	// account this record expects. The single field telling the operator
	// "will an automation on this account actually run".
	Reachable bool   `json:"reachable"`
	State     string `json:"state"` // online | logged_out | offline | no_connector | wrong_account
}

// connectorStatus is the tenant-facing presence board.
// GET /connectors/status — per-account connector + member + online state.
func (h *Handler) connectorStatus(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": "org context required"})
	}
	accounts, err := h.db.Identities().GetAllAccounts(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	conns, err := h.db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Index connectors by assigned account; prefer an ONLINE one when several
	// are bound to the same account.
	byAccount := make(map[int64]connectors.AgentToken, len(conns))
	for _, conn := range conns {
		if conn.AssignedAccountID <= 0 {
			continue
		}
		if existing, ok := byAccount[conn.AssignedAccountID]; !ok || (!existing.Online && conn.Online) {
			byAccount[conn.AssignedAccountID] = conn
		}
	}

	rows := make([]connectorAccountStatus, 0, len(accounts))
	onlineCount, reachableCount := 0, 0
	for _, acc := range accounts {
		if acc.Platform != models.PlatformFacebook {
			continue
		}
		row := connectorAccountStatus{
			AccountID:            acc.ID,
			AccountName:          acc.Name,
			AssignedUserID:       acc.AssignedUserID,
			AssignedUserName:     acc.AssignedUserName,
			AccountFBUserID:      strings.TrimSpace(acc.FBUserID),
			AccountFBDisplayName: strings.TrimSpace(acc.FBDisplayName),
			State:                "no_connector",
		}
		if conn, ok := byAccount[acc.ID]; ok {
			row.ConnectorID = conn.ID
			row.ConnectorName = conn.Name
			row.ConnectorOnline = conn.Online
			row.StreamStatus = conn.StreamStatus
			row.ConnectorFBUserID = strings.TrimSpace(conn.FBUserID)
			row.ConnectorFBDisplayName = strings.TrimSpace(conn.FBDisplayName)
			row.State, row.Reachable = deriveConnectorState(acc, conn)
			if conn.Online {
				onlineCount++
			}
		}
		if row.Reachable {
			reachableCount++
		}
		rows = append(rows, row)
	}

	// Online connectors NOT bound to any account — otherwise a paired-but-
	// mis-assigned extension would be invisible to the operator.
	unbound := make([]connectorAccountStatus, 0)
	for _, conn := range conns {
		if conn.AssignedAccountID == 0 && conn.Online {
			unbound = append(unbound, connectorAccountStatus{
				ConnectorID:       conn.ID,
				ConnectorName:     conn.Name,
				ConnectorOnline:   true,
				StreamStatus:      conn.StreamStatus,
				ConnectorFBUserID: strings.TrimSpace(conn.FBUserID),
				State:             "unassigned",
			})
		}
	}

	return c.JSON(fiber.Map{
		"accounts":        rows,
		"unbound_online":  unbound,
		"accounts_total":  len(rows),
		"online_total":    onlineCount,
		"reachable_total": reachableCount,
	})
}

// deriveConnectorState classifies the account×connector pair into one operator-
// readable state + whether automation can actually run on it.
func deriveConnectorState(acc models.Account, conn connectors.AgentToken) (string, bool) {
	if !conn.Online {
		return "offline", false
	}
	if !strings.EqualFold(strings.TrimSpace(conn.StreamStatus), "facebook_logged_in") {
		return "logged_out", false
	}
	// Online + logged in. If the account expects a specific FB user, the
	// connector must be logged into THAT user, else it acts as the wrong account
	// (the #49/#50 class of confusion).
	if acc.FBUserID != "" && conn.FBUserID != "" &&
		strings.TrimSpace(acc.FBUserID) != strings.TrimSpace(conn.FBUserID) {
		return "wrong_account", false
	}
	return "online", true
}
