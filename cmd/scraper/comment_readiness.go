package main

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/readiness"
	"github.com/thg/scraper/internal/store"
)

// Facebook Automation Reliability Track — §5 (No-ready-account behavior).
// A comment run must NEVER tell the operator "queued for comment" in a way that
// implies posting when no Facebook account can actually execute. Before scanning
// leads, the comment flow preflights the resolved account against the SAME
// readiness invariant crawl uses (PR-A): connector online + correct live
// Facebook identity + supported extension version. If the account cannot run,
// the run is BLOCKED with an actionable message instead of queueing comments
// that can never post.
//
// This reuses the shared decision (readiness.EvaluateCrawlAccountReadiness →
// connectors.PickReadyConnector) so create-time, crawl, and comment never
// diverge on what "ready" means. Account paused/cooldown/checkpoint stays a
// per-message gate downstream (DecideCaps) — this only blocks the "no ready
// account at all" case, never weakens the per-message caps.
//
// The readiness primitive now lives in the neutral internal/readiness package
// (PR29C), shared by crawl + comment with no dependency on internal/server/crawl.

// commentReadinessGate returns (blockMessage, blocked). When blocked is true the
// caller must return blockMessage WITHOUT queueing anything. blocked is false
// only when the account is fully ready to execute a comment now.
func commentReadinessGate(ctx context.Context, db *store.Store, orgID, userID int64, role string, accountID int64) (string, bool) {
	reason, detail := readiness.EvaluateCrawlAccountReadiness(ctx, db, orgID, userID, role, accountID)
	return commentReadinessDecision(reason, detail)
}

// commentReadinessDecision maps a (reason, detail) readiness verdict to
// (blockMessage, blocked). Pure so the block policy is unit-testable: ANY reason
// other than ReadinessReady blocks the comment run (connector offline, identity
// unknown/mismatch, extension update_required/unsupported, account not owned).
func commentReadinessDecision(reason, detail string) (string, bool) {
	if reason == readiness.ReadinessReady {
		return "", false
	}
	return commentReadinessBlock(detail), true
}

// commentReadinessBlock formats the no-ready-account block for a comment run: a
// comment-context headline (so the operator knows posting will NOT happen) plus
// the shared preflight's actionable detail (which specific step to fix).
func commentReadinessBlock(detail string) string {
	base := "Chưa có tài khoản Facebook sẵn sàng để chạy comment. Hãy kết nối Facebook (mở Chrome đã đăng nhập đúng tài khoản và pair connector) trước khi chạy comment."
	if d := strings.TrimSpace(detail); d != "" {
		return base + " Chi tiết: " + d
	}
	return base
}
