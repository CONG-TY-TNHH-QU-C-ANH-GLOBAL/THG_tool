package facebook

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/readiness"
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
// This reuses the shared decision (the neutral internal/readiness primitive →
// connectors.PickReadyConnector) so create-time, crawl, and comment never
// diverge on what "ready" means. Account paused/cooldown/checkpoint stays a
// per-message gate downstream (DecideCaps) — this only blocks the "no ready
// account at all" case, never weakens the per-message caps.

// CommentReadinessEvaluator is the narrow, consumer-owned port the comment
// readiness gate needs from the data layer. services/facebook owns this
// interface and depends ONLY on it (plus the neutral readiness reason codes) —
// it does NOT import internal/store. The composition root (cmd/scraper) supplies
// a tiny adapter backed by *store.Store + internal/readiness. The returned
// (reason, detail) is the readiness verdict: reason == readiness.ReadinessReady
// means the account may execute now; any other reason carries an actionable
// detail message.
type CommentReadinessEvaluator interface {
	EvaluateCommentReadiness(ctx context.Context, orgID, userID int64, role string, accountID int64) (reason, detail string)
}

// CommentReadinessGate returns (blockMessage, blocked). When blocked is true the
// caller must return blockMessage WITHOUT queueing anything. blocked is false
// only when the account is fully ready to execute a comment now.
func CommentReadinessGate(ctx context.Context, eval CommentReadinessEvaluator, orgID, userID int64, role string, accountID int64) (string, bool) {
	reason, detail := eval.EvaluateCommentReadiness(ctx, orgID, userID, role, accountID)
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
