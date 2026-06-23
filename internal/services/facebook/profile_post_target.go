package facebook

import (
	"strings"

	"github.com/thg/scraper/internal/models"
)

// AccountFetcher captures the subset of the store that ResolveProfilePostTarget needs.
// Function-typed (not an interface) so callers/tests pass a closure without standing up
// the full store.
type AccountFetcher func(accountID, orgID int64) (*models.Account, error)

// ResolveProfilePostTarget picks the explicit profile_url first, then falls back to the
// account's FBProfileURL when account lookup succeeds. Returns ("", "no_profile_url_resolved")
// when neither resolves — the caller MUST refuse to queue rather than implicitly post to /me.
// Dropping the /me fallback (outbound-audit #5) closed a deterministic-boundary leak: /me
// resolves per-logged-in account, so multi-account ops could cross-post identities silently.
//
// This is the Facebook profile-post target convention, owned by the FB service. Pure: imports
// only models + stdlib (no store), behavior identical to the inline form it replaces.
func ResolveProfilePostTarget(fetch AccountFetcher, orgID, accountID int64, requestedURL string) (string, string) {
	if t := strings.TrimSpace(requestedURL); t != "" {
		return t, ""
	}
	if accountID <= 0 || fetch == nil {
		return "", "no_profile_url_resolved"
	}
	acc, err := fetch(accountID, orgID)
	if err != nil || acc == nil {
		return "", "no_profile_url_resolved"
	}
	if t := strings.TrimSpace(acc.FBProfileURL); t != "" {
		return t, ""
	}
	return "", "no_profile_url_resolved"
}
