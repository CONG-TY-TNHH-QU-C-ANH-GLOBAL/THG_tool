package server

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// requireAccountForOrg fetches an account scoped to the caller's organization.
//
// Tenant-facing handlers must use this helper instead of GetAccount + manual
// org_id comparison. It returns:
//   - (account, nil) when the account exists and belongs to orgID, or when
//     orgID == 0 (platform/founder context allowed by IsPlatformUser).
//   - (nil, fiberErr) when the account was not found in this org. The handler
//     MUST return fiberErr unchanged — the response body has already been
//     written.
//
// The 404 response intentionally does not differentiate between
// "account does not exist" and "account belongs to another org" so that
// account IDs cannot be enumerated across tenants.
func (s *Server) requireAccountForOrg(c *fiber.Ctx, accountID, orgID int64) (*models.Account, error) {
	acc, err := s.db.GetAccountForOrg(accountID, orgID)
	if err != nil || acc == nil {
		return nil, c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	return acc, nil
}

// requireAccountForOrgWS is the WebSocket-handler variant. It writes a text
// frame describing the failure and returns nil/false on rejection. Callers
// should `return` after a false result.
//
// Platform users (orgID == 0 with founder/superadmin role) bypass the check
// because they need cross-tenant browser observability.
func (s *Server) requireAccountForOrgWS(orgID int64, role string, accountID int64) (*models.Account, bool) {
	acc, err := s.db.GetAccount(accountID)
	if err != nil || acc == nil {
		return nil, false
	}
	if !models.IsPlatformUser(orgID, models.UserRole(role)) && acc.OrgID != orgID {
		return nil, false
	}
	return acc, true
}

// rejectIfFacebookProfileMismatch enforces that an incoming Facebook identity
// matches the one already bound to the account slot. When a mismatch is
// detected it persists a "local_error" browser_sessions row so the dashboard
// can surface the conflict to the operator, and writes a 409 response.
//
// Returns nil when the identity is acceptable (either the slot is unbound or
// the incoming fb_user_id matches). Returns the fiber response error otherwise
// — the handler MUST return it without writing further to the context.
func (s *Server) rejectIfFacebookProfileMismatch(c *fiber.Ctx, ctx context.Context, acc *models.Account, incomingFBUserID string, orgID int64) error {
	incoming := strings.TrimSpace(incomingFBUserID)
	if incoming == "" || acc == nil || strings.TrimSpace(acc.FBUserID) == "" || acc.FBUserID == incoming {
		return nil
	}
	if appStore, err := store.NewAppStore(s.db); err == nil {
		_ = appStore.RecordLocalSession(ctx, acc.ID, orgID, store.SessionError,
			"Facebook profile mismatch; create a separate account slot for this Facebook user")
	}
	return c.Status(409).JSON(fiber.Map{"error": "facebook profile mismatch for this account slot"})
}
