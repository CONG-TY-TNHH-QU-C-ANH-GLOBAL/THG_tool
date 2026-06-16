package copilot

import "github.com/thg/scraper/internal/models"

// accountsForActionPlanning returns the accounts that may appear in the LLM/Brain
// action-planning context for this requester (PR-1 context isolation).
//
// Rule: LLM-visible accounts for action planning == requester-CONTROLLABLE accounts only.
// CONTROL is not visibility and not role: a member's Facebook account is private to them, and
// an admin must NOT see (and therefore cannot plan actions on) another member's account. If the
// model never sees another member's account_id, it cannot select or hallucinate it. This mirrors
// the ACCOUNT-side control predicate the write guard enforces (own OR unassigned org account) —
// real control of an unassigned account still needs the requester's own live connector, checked
// at execution; member-owned accounts are excluded here regardless of role.
//   - own account (assigned_user_id == requester)  → included
//   - unassigned org-owned account                 → included (plannable; execution re-checks)
//   - another member's account                      → excluded (admin included)
//
// requesterUserID <= 0 is the legacy / Telegram / system path: it is used ONLY for read/crawl
// planning (every Facebook WRITE action fails closed at the execution guard for requester<=0),
// so the org-wide set is returned to keep read/crawl working — no write can run from it.
func (a *Agent) accountsForActionPlanning(orgID, requesterUserID int64) []models.Account {
	all, _ := a.db.Identities().GetAllAccounts(orgID)
	if requesterUserID <= 0 {
		return all
	}
	out := make([]models.Account, 0, len(all))
	for i := range all {
		if all[i].AssignedUserID == requesterUserID || all[i].AssignedUserID == 0 {
			out = append(out, all[i])
		}
	}
	return out
}
