package agent

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/store/coordination"
)

// Explicit direct-post intake support for the connector crawl-result ingest
// (hotfix fix/direct-post-intake-filter-bypass). When a user issues
// "Comment bài này cho tôi <url>", the single-post import that results MUST create the
// lead even if the generic market-signal filter would reject it, AND keep the requested
// group-context URL rather than the connector's lossy permalink.php. The DURABLE link
// is body.TaskID == direct_post_comment_workflows.import_task_id — no in-memory state.

// directPostLeadIdentity is the context-preserving identity an explicit direct-post lead
// must carry so a connector's lossy permalink.php never overwrites the requested group
// permalink (which would also defeat the P1.1 exact-canonical identity lookup).
type directPostLeadIdentity struct {
	primaryURL string // requested canonical group permalink (not permalink.php)
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

// directPostItemOverride reports whether `item` is the explicit post `wf` requested and,
// if so, the context-preserving identity to persist. A neighbouring post returned by the
// same crawl (different post id) is NOT overridden and goes through normal filtering.
func directPostItemOverride(wf *coordination.DirectPostCommentWorkflow, observedURL, itemPostFBID string) (directPostLeadIdentity, bool) {
	if wf == nil {
		return directPostLeadIdentity{}, false
	}
	pid := strings.TrimSpace(itemPostFBID)
	if pid == "" {
		pid = leadingest.ExtractFacebookPostID(observedURL)
	}
	if wf.PostFBID != "" && pid != "" && pid != wf.PostFBID {
		return directPostLeadIdentity{}, false // a different post — not the one requested
	}
	primary := strings.TrimSpace(wf.CanonicalPostURL)
	if primary == "" {
		primary = strings.TrimSpace(observedURL) // degrade gracefully; never empty the URL
	}
	return directPostLeadIdentity{primaryURL: primary, postFBID: wf.PostFBID, groupRef: wf.GroupRef}, true
}
