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

// capabilityMsgType maps a readiness capability to the behaviour-caps action type
// ("" = connector-only, no daily/risk caps — crawl is not an outbound action).
var capabilityMsgType = map[string]string{
	models.CapabilityCrawl:   "",
	models.CapabilityComment: "comment",
	models.CapabilityInbox:   "inbox",
	models.CapabilityPost:    "group_post",
}

var readinessCapabilities = []string{
	models.CapabilityCrawl, models.CapabilityComment, models.CapabilityInbox, models.CapabilityPost,
}

// BuildAccountReadinessMatrix composes the per-account, per-capability readiness
// for an org. Read-only (no decay/writes). Connector eligibility comes from the
// shared PickReadyConnector; outbound capabilities additionally consult the
// read-only behaviour caps (EvaluateCaps).
func BuildAccountReadinessMatrix(db *store.Store, orgID int64) ([]models.AccountReadiness, error) {
	ctx := context.Background()
	identities, err := db.Identities().AccountIdentitiesForOrg(orgID)
	if err != nil {
		return nil, err
	}
	conns, _ := db.Connectors().ListLocalConnectors(orgID)
	connByID := make(map[int64]connectors.AgentToken, len(conns))
	for i := range conns {
		connByID[conns[i].ID] = conns[i]
	}

	out := make([]models.AccountReadiness, 0, len(identities))
	for accID, ident := range identities {
		connID, connReason := connectors.PickReadyConnector(conns, accID, ident.FBUserID, connectors.MinExtensionVersion)
		extVer := ""
		if c, ok := connByID[connID]; ok {
			extVer = c.Version
		}
		ar := models.AccountReadiness{
			AccountID:        accID,
			FBUserID:         ident.FBUserID,
			FBDisplayName:    ident.FBDisplayName,
			ConnectorID:      connID,
			ExtensionVersion: extVer,
			RequiredAction:   readinessRequiredAction(connReason),
		}
		connReady := connReason == connectors.ConnReady
		for _, capName := range readinessCapabilities {
			var reasons []string
			if !connReady {
				reasons = append(reasons, connReason)
			}
			if msgType := capabilityMsgType[capName]; msgType != "" {
				if dec, derr := db.Coordination().EvaluateCaps(ctx, accID, msgType); derr == nil && !dec.Allowed {
					reasons = append(reasons, dec.Reason)
				}
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
	if orgID <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": "org context required"})
	}
	matrix, err := BuildAccountReadinessMatrix(h.db, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"accounts": matrix})
}
