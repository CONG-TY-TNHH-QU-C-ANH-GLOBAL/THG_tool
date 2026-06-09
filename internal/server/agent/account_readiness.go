package agent

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
	connByID := make(map[int64]connectors.AgentToken, len(conns))
	for i := range conns {
		connByID[conns[i].ID] = conns[i]
	}
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
		connID, connReason := connectors.PickReadyConnector(conns, acc.ID, acc.FBUserID, connectors.MinExtensionVersion)
		extVer, machineLabel, profileID := "", "", ""
		if c, ok := connByID[connID]; ok {
			extVer, machineLabel, profileID = c.Version, c.MachineLabel, c.BrowserProfileID
		}
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
			RequiredAction:   readinessRequiredAction(connReason),
		}
		connReady := connReason == connectors.ConnReady
		actorBlocked := actorStates[acc.ID].Blocked
		for _, capName := range readinessCapabilities {
			// Non-nil so JSON is `[]`, never `null` — a nil slice would marshal to
			// null and crash the FE (`cap.reasons[0]`). Contract: reasons is always
			// an array.
			reasons := []string{}
			if !connReady {
				reasons = append(reasons, connReason)
			}
			if capName == models.CapabilityCrawl {
				// crawl is read-only: outbound pacing (cooldown/risk/daily) does NOT
				// apply, but the hard Verified-Actor block (denies ALL execution) does.
				if actorBlocked {
					reasons = append(reasons, "actor_mismatch_blocked")
				}
			} else if dec, derr := db.Coordination().EvaluateCaps(ctx, acc.ID, capabilityMsgType[capName]); derr == nil && !dec.Allowed {
				reasons = append(reasons, dec.Reason)
			}
			ar.Capabilities = append(ar.Capabilities, models.CapabilityReadiness{
				Capability: capName,
				Can:        len(reasons) == 0,
				Reasons:    reasons,
			})
		}
		out = append(out, ar)
	}
	return out, nil
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
	case connectors.ConnExtensionOutdated:
		return "Cập nhật Chrome Extension lên phiên bản mới."
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
