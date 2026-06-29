package main

import (
	"strings"
	"time"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Crawl account auto-pick (RBAC member-scope + connector→screenshot→account ladder).
// Behavior is preserved exactly from the previous inline crawl_runtime.go version; it
// is extracted here as the account-resolution concern (distinct from crawl
// runtime/dispatch) and stays in cmd — the dispatch core is what moves to
// internal/crawler in ARCHCM4b.

// pickReadyFacebookAccountIDForCrawl auto-picks the account a connector-less crawl
// should run on, gated by the PR-M3 member-ownership filter, then walking the
// connector → screenshot → logged-in-account ladder (first match wins). Behavior is
// unchanged from the previous inline version; the ladder stages are extracted into
// helpers (cognitive-complexity) and the owner gate now reuses the canonical
// callerRestrictedToOwnedAccounts helper (the deferred ARCHCM-R1a crawl adoption).
func pickReadyFacebookAccountIDForCrawl(db *store.Store, orgID, userID int64, role string) (int64, error) {
	allow, err := crawlOwnershipGate(db, orgID, userID, role)
	if err != nil {
		return 0, err
	}
	if allow == nil {
		return 0, nil // member owns no account — nothing safe to auto-pick
	}
	if id := pickOnlineConnectorAccount(db, orgID, allow); id > 0 {
		return id, nil
	}
	if id, sErr := pickFreshScreenshotAccount(db, orgID, allow); sErr != nil || id > 0 {
		return id, sErr
	}
	return pickLoggedInFacebookAccount(db, orgID, allow)
}

// crawlOwnershipGate builds the PR-M3 ownership predicate for auto-pick: a non-
// privileged sales member (models.RestrictedToOwnedAccounts) is limited to accounts
// they own; admin / platform / the userID<=0 scheduler are org-wide. Returns a nil
// allow func to signal "member owns nothing — stop, pick nothing" (caller returns
// 0, nil), or a non-nil error on the ownership lookup. Behaviorally identical to the
// previous inline gate.
func crawlOwnershipGate(db *store.Store, orgID, userID int64, role string) (func(int64) bool, error) {
	if !models.RestrictedToOwnedAccounts(userID, role) {
		return func(int64) bool { return true }, nil
	}
	accs, err := db.Identities().GetAccountsForUser(orgID, userID)
	if err != nil {
		return nil, err
	}
	owned := make(map[int64]bool, len(accs))
	for _, a := range accs {
		owned[a.ID] = true
	}
	if len(owned) == 0 {
		return nil, nil
	}
	return func(id int64) bool { return owned[id] }, nil
}

// pickOnlineConnectorAccount returns the account an ONLINE, FB-logged-in extension
// connector is actually bound to. The dispatcher (pickOnlineConnectorForCrawl) routes
// by the connector's assigned_account_id, so the picker MUST agree with it. Otherwise
// the picker can choose an account by the stale accounts.browser_logged_in flag (e.g.
// #50) that NO online connector serves, while the live connector is bound to a
// different account record (e.g. #49) — and every dispatch then fails with "Chrome
// Extension is not online for this account". That picker↔dispatcher mismatch was the
// #49/#50 bug. A connector-list error is swallowed (fall through to the next stage),
// matching the original. Returns 0 when nothing matches.
func pickOnlineConnectorAccount(db *store.Store, orgID int64, allow func(int64) bool) int64 {
	conns, cErr := db.Connectors().ListLocalConnectors(orgID)
	if cErr != nil {
		return 0
	}
	for _, c := range conns {
		if c.Online && c.AssignedAccountID > 0 && allow(c.AssignedAccountID) &&
			strings.EqualFold(strings.TrimSpace(c.StreamStatus), browsergateway.StreamFacebookLoggedIn) {
			return c.AssignedAccountID
		}
	}
	return 0
}

// pickFreshScreenshotAccount returns the account of the latest connector screenshot
// when it is FB-logged-in and fresh (<= 5 minutes). A screenshot-lookup error is
// returned (not swallowed), matching the original. Returns 0 when nothing matches.
func pickFreshScreenshotAccount(db *store.Store, orgID int64, allow func(int64) bool) (int64, error) {
	screen, err := db.Connectors().GetLatestConnectorScreenshot(orgID, 0)
	if err != nil {
		return 0, err
	}
	if screen != nil &&
		screen.AccountID > 0 && allow(screen.AccountID) &&
		screen.AgentID > 0 &&
		strings.EqualFold(strings.TrimSpace(screen.StreamStatus), browsergateway.StreamFacebookLoggedIn) &&
		time.Since(screen.UpdatedAt) <= 5*time.Minute {
		return screen.AccountID, nil
	}
	return 0, nil
}

// pickLoggedInFacebookAccount is the final fallback: the first active, browser-logged-in
// Facebook account with a known fb_user_id that the requester may use. A lookup error is
// returned (not swallowed), matching the original. Returns 0 when nothing matches.
func pickLoggedInFacebookAccount(db *store.Store, orgID int64, allow func(int64) bool) (int64, error) {
	accounts, err := db.Identities().GetAllAccounts(orgID)
	if err != nil {
		return 0, err
	}
	for _, acc := range accounts {
		if acc.Platform == models.PlatformFacebook &&
			acc.BrowserLoggedIn &&
			acc.Status == models.AccountActive &&
			strings.TrimSpace(acc.FBUserID) != "" && allow(acc.ID) {
			return acc.ID, nil
		}
	}
	return 0, nil
}
