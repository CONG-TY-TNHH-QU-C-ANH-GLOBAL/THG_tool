package agent

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// RequireAccountForOrg fetches an account scoped to the caller's organization.
func RequireAccountForOrg(db *store.Store, c *fiber.Ctx, accountID, orgID int64) (*models.Account, error) {
	acc, err := db.GetAccountForOrg(accountID, orgID)
	if err != nil || acc == nil {
		return nil, c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	return acc, nil
}

// RequireAccountForOrgWS is the WebSocket-handler variant.
func RequireAccountForOrgWS(db *store.Store, orgID int64, role string, accountID int64) (*models.Account, bool) {
	acc, err := db.GetAccount(accountID)
	if err != nil || acc == nil {
		return nil, false
	}
	if !models.IsPlatformUser(orgID, models.UserRole(role)) && acc.OrgID != orgID {
		return nil, false
	}
	return acc, true
}

// RejectIfFacebookProfileMismatch enforces that an incoming Facebook identity
// matches the one already bound to the account slot.
func RejectIfFacebookProfileMismatch(db *store.Store, c *fiber.Ctx, ctx context.Context, acc *models.Account, incomingFBUserID string, orgID int64) error {
	incoming := strings.TrimSpace(incomingFBUserID)
	if incoming == "" || acc == nil || strings.TrimSpace(acc.FBUserID) == "" || acc.FBUserID == incoming {
		return nil
	}
	if appStore, err := store.NewAppStore(db); err == nil {
		_ = appStore.RecordLocalSession(ctx, acc.ID, orgID, store.SessionError,
			"Facebook profile mismatch; create a separate account slot for this Facebook user")
	}
	return c.Status(409).JSON(fiber.Map{"error": "facebook profile mismatch for this account slot"})
}
