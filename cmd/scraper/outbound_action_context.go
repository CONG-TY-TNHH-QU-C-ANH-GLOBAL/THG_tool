package main

import (
	"fmt"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// resolveCallerAccountID picks the FB account_id the skill executor will use,
// enforcing execution-layer ownership per RBAC-1 (see
// feedback_shared_battlefield_not_crm.md):
//
//   - If requestedAccountID > 0: load it and verify the caller owns it.
//     Admin / platform roles pass; sales must match acc.AssignedUserID.
//   - If requestedAccountID <= 0 and the caller is identified (userID > 0):
//     pick from the caller's OWNED accounts only (sales = GetAccountsForUser,
//     admin / platform = GetAllAccounts).
//   - If userID <= 0 (Telegram bot / legacy unauthenticated path): pick
//     from any account in the org (preserves current behaviour; future PR
//     resolves Telegram operator → DB user).
//
// preferLoggedIn rewards the first FB-platform, browser-logged-in, active
// account in the candidate list (legacy lead-outreach behaviour). Set to
// false for post / profile_post paths that don't need a logged-in browser.
// resolveUserActionContext produces the campaign-ready models.ActionContext for
// a member-initiated (Source=manual) outbound. It wraps the deterministic
// account resolution; a future resolveCampaignActionContext returns the SAME
// shape so the execution path stays source-agnostic (campaign is additive).
// ConnectorID/CampaignID/ExecutionSourceID are left 0 — filled by the future
// connector-availability + campaign layers.
func resolveUserActionContext(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (models.ActionContext, error) {
	accID, err := resolveCallerAccountID(db, orgID, userID, role, requestedAccountID, preferLoggedIn)
	if err != nil {
		return models.ActionContext{}, err
	}
	return models.ActionContext{
		OrgID:           orgID,
		Source:          models.ActionSourceManual,
		InitiatorUserID: userID,
		AccountID:       accID,
	}, nil
}

func resolveCallerAccountID(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (int64, error) {
	if requestedAccountID > 0 {
		return callerAccountForExplicitID(db, orgID, userID, role, requestedAccountID)
	}

	candidates, err := ownedAccountCandidates(db, orgID, userID, role)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		if userID > 0 {
			return 0, fmt.Errorf("you have no Facebook account assigned in org %d; ask an admin to assign one", orgID)
		}
		return 0, fmt.Errorf("no Facebook account available for org %d", orgID)
	}
	if preferLoggedIn {
		return selectExecutionAccount(db, orgID, userID, candidates)
	}
	return candidates[0].ID, nil
}

// callerAccountForExplicitID loads an explicitly requested account and enforces
// execution-layer ownership: it must exist in the org, and (when the caller is
// identified) the caller must be allowed to own it.
func callerAccountForExplicitID(db *store.Store, orgID, userID int64, role string, requestedAccountID int64) (int64, error) {
	acc, err := db.Identities().GetAccountForOrg(requestedAccountID, orgID)
	if err != nil || acc == nil {
		return 0, fmt.Errorf("account_id %d not found in org %d", requestedAccountID, orgID)
	}
	if userID > 0 && !models.IsAccountOwnerAllowed(acc, userID, role) {
		return 0, fmt.Errorf("you do not own account #%d", requestedAccountID)
	}
	return acc.ID, nil
}

// ownedAccountCandidates returns the accounts the caller may resolve from:
// sales staff see only their owned accounts; admin / platform roles and the
// legacy unauthenticated (userID <= 0) path see all org accounts. The restriction
// decision is the shared models.RestrictedToOwnedAccounts predicate (ARCHCM2a).
func ownedAccountCandidates(db *store.Store, orgID, userID int64, role string) ([]models.Account, error) {
	if models.RestrictedToOwnedAccounts(userID, role) {
		return db.Identities().GetAccountsForUser(orgID, userID)
	}
	return db.Identities().GetAllAccounts(orgID)
}

// selectExecutionAccount applies the deterministic ExecutionContext resolution
// (Organic Sales Network): NO heuristic, NO "first logged-in", NO
// newest-connector, NO auto-magic default. Resolution order: explicit
// account_id (handled by the caller) -> user Default Account ->
// exactly-one-owned-account -> error execution_context_required.
func selectExecutionAccount(db *store.Store, orgID, userID int64, candidates []models.Account) (int64, error) {
	ownedIDs := make(map[int64]bool, len(candidates))
	for _, acc := range candidates {
		ownedIDs[acc.ID] = true
	}
	if def := db.GetUserDefaultAccount(orgID, userID); def > 0 && ownedIDs[def] {
		return def, nil
	}
	var usable []int64
	for _, acc := range candidates {
		if acc.Platform == models.PlatformFacebook && acc.Status == models.AccountActive {
			usable = append(usable, acc.ID)
		}
	}
	if len(usable) == 1 {
		return usable[0], nil
	}
	if len(usable) == 0 {
		return 0, fmt.Errorf("execution_context_required: no usable Facebook account — pair a Chrome connector and log into Facebook first")
	}
	return 0, fmt.Errorf("execution_context_required: you have %d Facebook accounts — set a Default Account in Settings (or pass account_id)", len(usable))
}
