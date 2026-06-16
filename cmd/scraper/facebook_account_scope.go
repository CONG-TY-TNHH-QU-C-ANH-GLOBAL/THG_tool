package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// Requester ownership/control scope for Facebook WRITE actions (PR-1 + PR-2).
//
// CONTROL is NOT VISIBILITY. The MVP control predicate for a Facebook WRITE action is the
// conjunction of three facts, every one of which must hold:
//
//	connector.created_by == requester_user_id                  (the requester OWNS the connector)
//	AND connector.live_fb_user_id == account.fb_user_id        (the connector is logged into it)
//	AND ( account.assigned_user_id == requester_user_id        (own account)
//	      OR ( account.assigned_user_id == 0                    (unassigned org account, controllable
//	           AND connector.created_by == requester_user_id ) ) //  ONLY via the requester's connector )
//
// It is decomposed across three call sites so each fact is checked once:
//   - controllableConnectors        → connector.created_by == requester       (connector ownership)
//   - PickReadyConnector(expectedFB) → connector.live_fb_user_id == account.fb (identity match)
//   - canRequesterControlAccount    → account side (own OR unassigned)
//
// admin ROLE grants nothing here, and account VISIBILITY (models.CanViewAccountDevice) grants
// nothing here — both are deliberately absent. A member-owned account is NEVER controllable by
// an admin. An unassigned account is controllable ONLY because the requester's OWN connector is
// live on it (the connector-ownership conjunct is enforced by controllableConnectors, so the
// account-side test only has to allow the unassigned case). Future shared-account control is a
// clean seam: a facebook_account_authorizations(account_id, user_id, permission='control')
// lookup ORs into canRequesterControlAccount without touching callers.
//
// requesterUserID <= 0 (Telegram / legacy / unauthenticated) CANNOT prove control → every write
// resolver fails closed with msgDPRequesterRequired. Org-scope legacy behaviour survives only
// for read/crawl/system flows, which never reach these resolvers.

// Customer-facing block messages (Vietnamese) for the ownership/control gate.
const (
	msgDPAccountNotControllable = "Bạn không có quyền điều khiển tài khoản Facebook này. Vui lòng chọn tài khoản Facebook do bạn kết nối hoặc được cấp quyền."
	msgDPNoControllableLive     = "Chưa có tài khoản Facebook nào do bạn kết nối đang online. Hãy mở Chrome và đăng nhập tài khoản Facebook của bạn rồi thử lại."
	msgDPRequesterRequired      = "Cần xác định người dùng thực hiện trước khi chạy tự động hoá Facebook ghi (comment/inbox/đăng bài). Vui lòng đăng nhập lại."
)

// canRequesterControlAccount is the ACCOUNT-side half of the control predicate (see file
// header). It deliberately ignores role and visibility. requesterUserID must be proven (> 0).
//   - own assigned account (assigned_user_id == requester) → controllable.
//   - unassigned org account (assigned_user_id == 0)        → allowed HERE, but real control
//     additionally requires a requester-owned live connector on its identity (enforced by
//     controllableConnectors + PickReadyConnector — never by admin role).
//   - another member's account (assigned to someone else)   → NEVER (admin included).
func canRequesterControlAccount(acc *models.Account, requesterUserID int64) bool {
	if requesterUserID <= 0 || acc == nil {
		return false
	}
	return acc.AssignedUserID == requesterUserID || acc.AssignedUserID == 0
}

// connectorControllableBy reports whether requesterUserID OWNS the connector
// (connector.created_by == requester). Strict: an unproven requester (<=0) or a connector with
// no tracked owner (created_by <= 0) is NOT controllable — a Facebook WRITE must never run on a
// connector whose owner cannot be proven to be the requester.
func connectorControllableBy(c connectors.AgentToken, requesterUserID int64) bool {
	return requesterUserID > 0 && c.CreatedBy > 0 && c.CreatedBy == requesterUserID
}

// controllableConnectors keeps only the connectors the requester owns (connector-ownership
// conjunct). An unproven requester yields an empty list (fail closed downstream).
func controllableConnectors(conns []connectors.AgentToken, requesterUserID int64) []connectors.AgentToken {
	out := make([]connectors.AgentToken, 0, len(conns))
	for _, c := range conns {
		if connectorControllableBy(c, requesterUserID) {
			out = append(out, c)
		}
	}
	return out
}

// liveReadyControllableAccountIDs is liveReadyAccountIDs restricted to accounts the requester
// can control: conns is already filtered to requester-owned connectors, and the fb→account map
// admits an account only when canRequesterControlAccount holds — so the result is exactly
// (live + identity-matched + requester-owned connector) ∩ (own or unassigned account).
func liveReadyControllableAccountIDs(db *store.Store, conns []connectors.AgentToken, policy connectors.VersionPolicy, orgID, requesterUserID int64) []int64 {
	return liveReadyAccountIDs(conns, policy, func(fb string) int64 {
		acc, _ := db.Identities().GetAccountByFacebookIdentity(orgID, fb)
		if acc == nil || !canRequesterControlAccount(acc, requesterUserID) {
			return 0
		}
		return acc.ID
	})
}

// resolveControllablePool returns the eligible execution pool for a DISTRIBUTED write action
// (comment_all_leads / auto_comment / auto_inbox / inbox_all_leads): the requester-controllable
// accounts that are live + identity-matched. An explicit selection narrows the pool to that one
// account (still ownership + live checked). Returns (pool, "") on success or (nil, blockMsg) when
// no safe plan exists — never another member's account.
func resolveControllablePool(db *store.Store, orgID, selectedAccountID, requesterUserID int64) ([]int64, string) {
	if db == nil || orgID <= 0 {
		return nil, msgDPAccountLookupError
	}
	if requesterUserID <= 0 {
		return nil, msgDPRequesterRequired // identity required for a Facebook write side-effect
	}
	// Explicit selection → single-account decision, ownership + live enforced by the guard.
	if selectedAccountID > 0 {
		res := resolveDirectPostAccount(db, orgID, selectedAccountID, requesterUserID)
		if !res.ok {
			return nil, res.message
		}
		return []int64{res.accountID}, ""
	}
	conns, err := db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return nil, msgDPAccountLookupError
	}
	conns = controllableConnectors(conns, requesterUserID)
	policy, _ := db.Connectors().GetExtensionPolicy()
	pool := liveReadyControllableAccountIDs(db, conns, policy, orgID, requesterUserID)
	if len(pool) == 0 {
		return nil, msgDPNoControllableLive
	}
	return pool, ""
}

// cloneOutreachArgs shallow-copies the action args so a per-account account_id pin in the
// pool loop never leaks across iterations.
func cloneOutreachArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	return out
}

// runPooledOutreach runs a DISTRIBUTED outreach action across the requester-controllable live
// pool: each eligible account gets its OWN execution (its own outbound items, gated per-account
// by readiness / coverage / risk / daily limits in queueLeadOutreach). Non-owned members'
// accounts are silently excluded — never enlisted, never fatal. An empty pool fails closed with
// a clear message. This is NOT broadcast/seeding: there is no same-post fan-out and no persona
// content; per-account volume stays bounded by the existing multi-actor coverage policy.
func runPooledOutreach(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	requester := argInt64(args, "user_id")
	selected := argInt64(args, "account_id")
	pool, blockMsg := resolveControllablePool(db, orgID, selected, requester)
	if blockMsg != "" {
		log.Printf("[FBPool] BLOCK org=%d requester=%d type=%s selected_account=%d reason=%q", orgID, requester, msgType, selected, blockMsg)
		return blockMsg, nil
	}
	log.Printf("[FBPool] org=%d requester=%d type=%s pool=%v", orgID, requester, msgType, pool)
	// Single account → return its result/error directly (single-task-like contract).
	if len(pool) == 1 {
		a := cloneOutreachArgs(args)
		a["account_id"] = pool[0]
		return queueLeadOutreach(ctx, db, msgGen, msgType, a, notify)
	}
	// Multi-account → each account is INDEPENDENT: per-account coverage/cooldown/dedup/risk run
	// inside queueLeadOutreach, and one account's error MUST NOT poison the others — it is logged
	// and reported per account while the rest continue.
	results := make([]string, 0, len(pool))
	for _, accID := range pool {
		a := cloneOutreachArgs(args)
		a["account_id"] = accID
		out, err := queueLeadOutreach(ctx, db, msgGen, msgType, a, notify)
		if err != nil {
			log.Printf("[FBPool] account=%d type=%s error=%v (other accounts continue)", accID, msgType, err)
			results = append(results, fmt.Sprintf("• Tài khoản #%d: lỗi — %v", accID, err))
			continue
		}
		results = append(results, fmt.Sprintf("• Tài khoản #%d: %s", accID, out))
	}
	header := fmt.Sprintf("Đã chạy trên %d tài khoản Facebook của bạn:", len(pool))
	return header + "\n" + strings.Join(results, "\n"), nil
}
