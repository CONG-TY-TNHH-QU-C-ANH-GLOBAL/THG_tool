package agent

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/connectors"
)

// Admin connector overview (SaaS UX Hardening PR-3): workspace-level
// OPERATIONAL status — which staff manages which Facebook account,
// connector online/offline, last_seen, extension version/state,
// readiness and automation eligibility. View-only by design: it grants
// NO device control (pair/unpair/input stay owner-only per PR-M5) and
// serializes a dedicated projection — never models.Account, never
// cookies/proxy/session data.

type connectorOverviewRow struct {
	AccountID             int64    `json:"account_id"`
	AccountName           string   `json:"account_name"`
	FBDisplayName         string   `json:"fb_display_name"`
	StaffUserID           int64    `json:"staff_user_id"`
	StaffName             string   `json:"staff_name"`
	StaffEmail            string   `json:"staff_email"`
	StaffRole             string   `json:"staff_role"`
	ConnectorOnline       bool     `json:"connector_online"`
	LastSeen              string   `json:"last_seen"`
	ExtensionVersion      string   `json:"extension_version"`
	ExtensionVersionState string   `json:"extension_version_state"`
	Readiness             string   `json:"readiness"` // ready | typed blocker reason
	AutomationEligible    bool     `json:"automation_eligible"`
	AssignmentPaused      bool     `json:"assignment_paused"`
	BlockReasons          []string `json:"block_reasons"`
	// ContactProfileState (PR-5 audit column): complete | incomplete |
	// missing | "" (unassigned account). Read-only — comments fall back
	// to the company contact (or omit, per policy) when not complete.
	ContactProfileState string `json:"contact_profile_state"`
}

// connectorOverview handles GET /api/admin/connectors/overview (adminOnly).
func (h *Handler) connectorOverview(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": "org context required"})
	}
	ctx := context.Background()

	accounts, err := h.db.Identities().GetAllAccounts(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	conns, _ := h.db.Connectors().ListLocalConnectors(orgID)
	policy, _ := h.db.Connectors().GetExtensionPolicy()
	users, _ := h.db.ListUsers(orgID)
	userByID := make(map[int64]models.User, len(users))
	for _, u := range users {
		userByID[u.ID] = u
	}
	pausedByID := h.assignmentPausedMap(orgID)
	contactByUser := map[int64]*models.StaffContactProfile{}
	if profiles, perr := h.db.ListStaffContactProfiles(orgID); perr == nil {
		for i := range profiles {
			contactByUser[profiles[i].UserID] = &profiles[i]
		}
	}

	rows := make([]connectorOverviewRow, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		if acc.Platform != models.PlatformFacebook {
			continue
		}
		connID, connReason := connectors.PickReadyConnector(conns, acc.ID, acc.FBUserID, policy)
		row := connectorOverviewRow{
			AccountID:        acc.ID,
			AccountName:      acc.Name,
			FBDisplayName:    acc.FBDisplayName,
			StaffUserID:      acc.AssignedUserID,
			StaffName:        acc.AssignedUserName,
			Readiness:        connReason,
			AssignmentPaused: pausedByID[acc.ID],
			BlockReasons:     []string{},
		}
		if u, ok := userByID[acc.AssignedUserID]; ok {
			row.StaffEmail = u.Email
			row.StaffRole = string(u.Role)
			row.ContactProfileState = contactProfileState(contactByUser[acc.AssignedUserID])
		}
		// Surface the account's connector even when not ready (offline →
		// last_seen still answers "when was this device last alive").
		for j := range conns {
			cn := conns[j]
			if cn.ID == connID || (connID == 0 && cn.AssignedAccountID == acc.ID) {
				row.ConnectorOnline = cn.Online
				row.ExtensionVersion = cn.Version
				row.ExtensionVersionState = connectors.EvaluateVersionState(cn.Version, policy)
				if cn.LastSeen != nil {
					row.LastSeen = cn.LastSeen.UTC().Format(time.RFC3339)
				}
				break
			}
		}
		if connReason != connectors.ConnReady {
			row.BlockReasons = append(row.BlockReasons, connReason)
		}
		if row.AssignmentPaused {
			row.BlockReasons = append(row.BlockReasons, "assignment_paused_by_admin")
		}
		if dec, derr := h.db.Coordination().EvaluateCaps(ctx, acc.ID, "comment"); derr == nil && !dec.Allowed {
			row.BlockReasons = append(row.BlockReasons, dec.Reason)
		}
		row.AutomationEligible = len(row.BlockReasons) == 0
		rows = append(rows, row)
	}
	return c.JSON(fiber.Map{"accounts": rows, "count": len(rows)})
}

// contactProfileState classifies a staff contact profile for the audit
// column: complete (active + a usable contact line), incomplete (row
// exists but inactive/empty), missing (never configured).
func contactProfileState(p *models.StaffContactProfile) string {
	switch {
	case p == nil:
		return "missing"
	case p.Active && p.ContactLine() != "":
		return "complete"
	default:
		return "incomplete"
	}
}

// assignmentPausedMap reads the admin pause flag for every org account
// in one query. tenant-ok: org-scoped projection for the admin view.
func (h *Handler) assignmentPausedMap(orgID int64) map[int64]bool {
	out := map[int64]bool{}
	rows, err := h.db.DB().Query(
		`SELECT id, COALESCE(assignment_paused, 0) FROM accounts WHERE org_id = ?`, orgID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var paused int
		if err := rows.Scan(&id, &paused); err == nil {
			out[id] = paused == 1
		}
	}
	return out
}
