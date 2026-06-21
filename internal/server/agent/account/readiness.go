package account

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// FB Automation Reliability PR-D — Readiness Matrix. One org-scoped projection of
// "can each account do crawl/comment/inbox/post, and if not, why" so every
// mission/action UI consumes typed reasons instead of guessing. Reasons reuse the
// SAME evaluators the gates use (connectors.PickReadyConnector + coordination
// EvaluateCaps/DecideCaps) so the matrix can never disagree with the gate.

// capabilityMsgType maps an OUTBOUND readiness capability to its behaviour-caps
// action type. crawl is NOT here — it is a read-only action (see the crawl branch
// below), not subject to outbound daily/risk/cooldown pacing. "post" maps to
// `group_post` (the live FB posting action); profile posting is a scaffold today,
// so a separate `profile_post` capability is intentionally deferred until it ships
// (documented so the group_post daily cap is never silently missed).
var capabilityMsgType = map[string]string{
	models.CapabilityComment: "comment",
	models.CapabilityInbox:   "inbox",
	models.CapabilityPost:    "group_post",
}

var readinessCapabilities = []string{
	models.CapabilityCrawl, models.CapabilityComment, models.CapabilityInbox, models.CapabilityPost,
}

// BuildAccountReadinessMatrix composes the per-account, per-capability readiness
// for an org, SCOPED to what the caller may see (RBAC: a member sees only their
// own accounts; an admin additionally sees unassigned org accounts — same privacy
// rule as the connector status board, so no other member's fb_user_id leaks).
// Read-only (no decay/writes). Connector eligibility = shared PickReadyConnector;
// crawl additionally honors the hard actor-mismatch block; comment/inbox/post
// additionally consult the read-only behaviour caps (EvaluateCaps).
func BuildAccountReadinessMatrix(db *store.Store, orgID, userID int64, role string) ([]models.AccountReadiness, error) {
	ctx := context.Background()
	accounts, err := db.Identities().GetAllAccounts(orgID)
	if err != nil {
		return nil, err
	}
	conns, _ := db.Connectors().ListLocalConnectors(orgID)
	connByID := indexConnectorsByID(conns)
	policy, _ := db.Connectors().GetExtensionPolicy()
	actorStates, _ := db.Coordination().AccountActorStatesForOrg(ctx, orgID)

	out := make([]models.AccountReadiness, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		if acc.Platform != models.PlatformFacebook {
			continue
		}
		if !models.CanViewAccountDevice(&acc, userID, role) {
			continue // RBAC + privacy: not the caller's own (and not admin-visible unassigned)
		}
		connID, connReason, extVer, machineLabel, profileID := resolveConnectorMeta(conns, connByID, acc, policy)
		ar := models.AccountReadiness{
			AccountID:        acc.ID,
			AccountName:      acc.Name,
			AssignedUserName: acc.AssignedUserName,
			FBUserID:         acc.FBUserID,
			FBDisplayName:    acc.FBDisplayName,
			ConnectorID:      connID,
			MachineLabel:     machineLabel,
			BrowserProfileID: profileID,
			ExtensionVersion: extVer,
			// State of the picked (or first matching) connector's version —
			// drives the staff soft-warning even while automation still runs.
			ExtensionVersionState: connectors.EvaluateVersionState(extVer, policy),
			RequiredAction:        readinessRequiredAction(connReason),
		}
		actorBlocked := actorStates[acc.ID].Blocked

		// P1.3E requester-scoped executability: green "Sẵn sàng" must mean the REQUESTER can run
		// now via their OWN live connector — never an org-wide / other member's connector. This is
		// additive; capabilities[].can below stays the org-wide projection other UIs consume.
		ex := resolveAccountExecutable(acc, connectors.OwnedBy(conns, userID), policy,
			actorBlocked, acc.Status == models.AccountActive, userID)
		applyExecStatus(&ar, ex)
		ar.Capabilities = buildCapabilityReadiness(ctx, db, acc, connReason, actorBlocked)
		out = append(out, ar)
	}
	return out, nil
}

// indexConnectorsByID maps connectors by their id for O(1) lookup.
func indexConnectorsByID(conns []connectors.AgentToken) map[int64]connectors.AgentToken {
	byID := make(map[int64]connectors.AgentToken, len(conns))
	for i := range conns {
		byID[conns[i].ID] = conns[i]
	}
	return byID
}

// resolveConnectorMeta resolves the readiness gate verdict for an account plus the
// version/label of the picked connector — or, when the picked connector is blocked
// (e.g. update_required), the first connector still assigned to the account so the
// staff panel can show WHICH build needs updating instead of a blank row.
func resolveConnectorMeta(conns []connectors.AgentToken, connByID map[int64]connectors.AgentToken, acc models.Account, policy connectors.VersionPolicy) (connID int64, connReason, extVer, machineLabel, profileID string) {
	connID, connReason = connectors.PickReadyConnector(conns, acc.ID, acc.FBUserID, policy)
	if c, ok := connByID[connID]; ok {
		return connID, connReason, c.Version, c.MachineLabel, c.BrowserProfileID
	}
	for j := range conns {
		if conns[j].AssignedAccountID == acc.ID {
			return connID, connReason, conns[j].Version, conns[j].MachineLabel, conns[j].BrowserProfileID
		}
	}
	return connID, connReason, "", "", ""
}

// applyExecStatus copies the decomposed executability verdict onto the readiness row.
func applyExecStatus(ar *models.AccountReadiness, ex execStatus) {
	ar.Configured = ex.configured
	ar.ControlAllowed = ex.controlAllowed
	ar.Paired = ex.paired
	ar.ConnectorOnline = ex.connectorOnline
	ar.HeartbeatFresh = ex.connectorOnline
	ar.LiveIdentityMatched = ex.liveIdentityMatched
	ar.SessionUsable = ex.sessionUsable
	ar.Executable = ex.executable
	ar.ExecReasonCode = ex.reasonCode
	ar.ExecReasonMessage = execReasonMessage(ex.reasonCode)
}

// buildCapabilityReadiness builds the per-capability can/why-not list for one
// account. crawl is read-only (outbound pacing does NOT apply, but the hard
// Verified-Actor block does); comment/inbox/post consult the read-only behaviour
// caps. reasons is always a non-nil slice so JSON is `[]`, never null.
func buildCapabilityReadiness(ctx context.Context, db *store.Store, acc models.Account, connReason string, actorBlocked bool) []models.CapabilityReadiness {
	connReady := connReason == connectors.ConnReady
	caps := make([]models.CapabilityReadiness, 0, len(readinessCapabilities))
	for _, capName := range readinessCapabilities {
		reasons := []string{}
		if !connReady {
			reasons = append(reasons, connReason)
		}
		if capName == models.CapabilityCrawl {
			if actorBlocked {
				reasons = append(reasons, "actor_mismatch_blocked")
			}
		} else if dec, derr := db.Coordination().EvaluateCaps(ctx, acc.ID, capabilityMsgType[capName]); derr == nil && !dec.Allowed {
			reasons = append(reasons, dec.Reason)
		}
		caps = append(caps, models.CapabilityReadiness{
			Capability: capName,
			Can:        len(reasons) == 0,
			Reasons:    reasons,
		})
	}
	return caps
}

// readinessRequiredAction turns the connector blocker into a one-line operator hint.
func readinessRequiredAction(connReason string) string {
	switch connReason {
	case connectors.ConnOffline:
		return "Mở Chrome profile đã pair account này và đăng nhập Facebook."
	case connectors.ConnIdentityUnknown:
		return "Mở tab Facebook đã đăng nhập (extension chưa đọc được c_user)."
	case connectors.ConnIdentityMismatch:
		return "Đăng nhập đúng tài khoản Facebook cho account này."
	case connectors.ConnExtensionUpdateRequired:
		return "Automation đang tạm dừng vì extension đã cũ — cập nhật extension để tiếp tục nhận task."
	case connectors.ConnExtensionUnsupported:
		return "Phiên bản extension này không còn được hỗ trợ — cài phiên bản mới."
	default:
		return ""
	}
}

// accountReadiness is the tenant-facing readiness matrix endpoint.
// GET /api/accounts/readiness
func (h *Handler) accountReadiness(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	if orgID <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": "org context required"})
	}
	matrix, err := BuildAccountReadinessMatrix(h.db, orgID, userID, role)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"accounts": matrix})
}
