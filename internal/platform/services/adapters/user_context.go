// Package adapters transforms storage shapes into domain entities. This is the
// ONLY layer that touches *store.Store / models.* — resolvers and handlers
// consume the domain contracts, never the ORM rows.
// See specs/domains/platform-foundation/DOMAIN.md § Storage model is NOT the domain model.
package adapters

import (
	"time"

	"github.com/thg/scraper/internal/platform/services/contracts"
	"github.com/thg/scraper/internal/store"
)

// LoadUserContext reads raw storage and produces the domain UserContext that
// resolvers consume. The only place organization/user storage rows are touched.
// "now" is captured here (the IO layer) so resolvers stay pure & deterministic.
func LoadUserContext(db *store.Store, userID int64) (contracts.UserContext, error) {
	now := time.Now().UnixMilli()

	user, err := db.GetUserByID(userID)
	if err != nil {
		return contracts.UserContext{}, err
	}
	if user == nil {
		// Authed route with no resolvable user — degenerate but not an error.
		return contracts.UserContext{ResolvedAt: now}, nil
	}

	uc := contracts.UserContext{
		UserID:        user.ID,
		Role:          string(user.Role),
		Authenticated: true,
		ResolvedAt:    now,
	}

	if user.OrgID > 0 {
		org, err := db.GetOrganization(user.OrgID)
		if err != nil {
			return contracts.UserContext{}, err
		}
		if org != nil {
			uc.Org = &contracts.OrgContext{
				ID:       org.ID,
				Name:     org.Name,
				PlanTier: string(org.PlanTier),
				Active:   org.Active,
			}
		}
	}
	return uc, nil
}
