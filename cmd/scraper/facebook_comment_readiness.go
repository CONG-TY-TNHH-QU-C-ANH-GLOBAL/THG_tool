package main

import (
	"context"

	"github.com/thg/scraper/internal/readiness"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
)

// fbCommentReadinessEvaluator is the composition-root adapter that satisfies the
// consumer-owned facebook.CommentReadinessEvaluator port from the real
// *store.Store + the neutral internal/readiness primitive. It lives here (the
// wiring boundary) so services/facebook stays free of internal/store. Thin
// pass-through only — no logic, no behavior change.
type fbCommentReadinessEvaluator struct{ db *store.Store }

func (e fbCommentReadinessEvaluator) EvaluateCommentReadiness(ctx context.Context, orgID, userID int64, role string, accountID int64) (string, string) {
	return readiness.EvaluateCrawlAccountReadiness(ctx, e.db, orgID, userID, role, accountID)
}

// Compile-time check: the adapter satisfies the consumer-owned port.
var _ facebook.CommentReadinessEvaluator = fbCommentReadinessEvaluator{}
