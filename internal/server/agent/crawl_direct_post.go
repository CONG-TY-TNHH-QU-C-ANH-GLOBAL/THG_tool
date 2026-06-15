package agent

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/store/coordination"
)

// Explicit direct-post intake support for the connector crawl-result ingest. When a user
// issues "Comment bài này cho tôi <url>", the single-post import that results may create a
// lead even if the market filter would reject it (P1.2 ForceLead) — BUT only after the
// observed item is ZERO-TRUST validated against the requested target (P1.3A): positive
// post-id identity, no conflicting group/page context, and meaningful content. The durable
// link is body.TaskID == direct_post_comment_workflows.import_task_id.

// directPostLeadIdentity is the context-preserving identity an explicit direct-post lead
// must carry once validation passes, so a connector's lossy permalink.php never overwrites
// the requested group permalink (which also lets the P1.1 exact-canonical lookup match).
type directPostLeadIdentity struct {
	primaryURL string
	postFBID   string
	groupRef   string
}

// resolveDirectPostIntake returns the workflow this crawl task is importing for (by
// import_task_id == taskID), or nil for an ordinary crawl. nil-safe; errors degrade to
// nil (normal filtering) — the fix never makes a normal crawl worse.
func (h *Handler) resolveDirectPostIntake(ctx context.Context, orgID int64, taskID string) *coordination.DirectPostCommentWorkflow {
	if h.db == nil || orgID <= 0 || strings.TrimSpace(taskID) == "" {
		return nil
	}
	wf, err := h.db.Coordination().FindDirectPostWorkflowByImportTaskID(ctx, orgID, taskID)
	if err != nil || wf == nil {
		return nil
	}
	return wf
}

// validateDirectPostObservedItem zero-trust validates the observed item against the
// workflow target. On Valid it returns the context-preserving identity to persist; the
// Validation tells the caller whether a non-valid result is "the requested post but
// poisoned" (IdentityMatched → fail the workflow) or "a different/neighbour post"
// (skip, let normal filtering handle it). The canonical URL is stamped ONLY when valid —
// never onto unverified content.
func validateDirectPostObservedItem(wf *coordination.DirectPostCommentWorkflow, obs directpost.ObservedItem) (directPostLeadIdentity, directpost.Validation) {
	if wf == nil {
		return directPostLeadIdentity{}, directpost.Validation{}
	}
	v := directpost.Validate(directpost.ExpectedTarget{
		PostFBID:     wf.PostFBID,
		GroupRef:     wf.GroupRef,
		CanonicalURL: wf.CanonicalPostURL,
	}, obs)
	if !v.Valid {
		return directPostLeadIdentity{}, v
	}
	primary := strings.TrimSpace(wf.CanonicalPostURL)
	if primary == "" {
		primary = strings.TrimSpace(obs.SourceURL) // degrade gracefully; never empty the URL
	}
	return directPostLeadIdentity{primaryURL: primary, postFBID: wf.PostFBID, groupRef: wf.GroupRef}, v
}

// importContextMismatchCode maps a validation reason to the typed terminal workflow error
// for the ingest path: a content failure vs a context/identity conflict.
func importContextMismatchCode(reason string) string {
	if reason == directpost.ReasonContentInvalid {
		return coordination.DPErrLeadContentInvalid
	}
	return coordination.DPErrImportedItemContextMismatch
}

// contentPreview returns a short, single-line, secret-free snippet of post content for
// diagnostics. Post text is user-visible Facebook content (no cookies/tokens/session).
func contentPreview(content string) string {
	s := strings.Join(strings.Fields(content), " ")
	const max = 160
	if len([]rune(s)) > max {
		return string([]rune(s)[:max]) + "…"
	}
	return s
}
