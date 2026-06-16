package agent

import (
	"context"
	"log"
	"strings"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/store/coordination"
)

// directPostEval is the per-item verdict of the direct-post zero-trust gate. When the observed
// item validly IS the requested post, deps carries ForceLead + the context-preserving identity
// (primaryURL/postFBID/groupFBID) to ingest with. When the requested post came back poisoned
// (identity matched but group/content invalid), failedWorkflow is set and the caller must NOT
// ingest. A different/neighbour post leaves both flags false (fall through to normal filtering).
type directPostEval struct {
	deps           leadingest.Deps
	primaryURL     string
	postFBID       string
	groupFBID      string
	validObserved  bool
	failedWorkflow bool
	failureCode    string // typed terminal code set when failedWorkflow (for forensics)
}

// evaluateDirectPostCrawlItem runs the direct-post zero-trust validation for one observed item
// and encapsulates the three-way outcome (extracted verbatim from the former inline switch in
// the crawl ingest loop — same logging, same MarkDirectPostFailed, same fail-closed semantics):
//
//   - Valid          → ForceLead + stamp the context-preserving canonical identity + log passed.
//   - IdentityMatched → the requested post is poisoned: fail the workflow with the typed import
//     code, no lead/outbound, no canonical stamp (P1.3A fail-closed).
//   - otherwise       → a different/neighbour post: log skipped, fall through to normal filtering.
//
// Fiber-free. ExtraSignals is copied (never the shared baseDeps slice) on the valid path.
func (h *Handler) evaluateDirectPostCrawlItem(ctx context.Context, orgID int64, taskID string, wf *coordination.DirectPostCommentWorkflow, item connectorCrawlItem, baseDeps leadingest.Deps, sourceURL, content string) directPostEval {
	eval := directPostEval{
		deps:       baseDeps,
		primaryURL: sourceURL,
		postFBID:   strings.TrimSpace(item.PostFBID),
		groupFBID:  strings.TrimSpace(item.GroupFBID),
	}
	obs := directpost.ObservedItem{
		PostFBID:         strings.TrimSpace(item.PostFBID),
		SourceURL:        sourceURL,
		GroupFBID:        strings.TrimSpace(item.GroupFBID),
		AuthorName:       strings.TrimSpace(item.AuthorName),
		AuthorProfileURL: strings.TrimSpace(item.AuthorProfileURL),
		Content:          content,
	}
	id, v := validateDirectPostObservedItem(wf, obs)
	switch {
	case v.Valid:
		eval.validObserved = true
		eval.primaryURL = id.primaryURL
		eval.postFBID = id.postFBID
		eval.groupFBID = id.groupRef
		eval.deps.ForceLead = true
		eval.deps.ExtraSignals = append(append([]string{}, baseDeps.ExtraSignals...),
			"direct_post_context_validation:passed",
			"observed_source_url:"+sourceURL,
			"observed_author_name:"+obs.AuthorName,
			"observed_author_profile_url:"+obs.AuthorProfileURL)
		log.Printf("[ConnectorCrawl] direct_post_intake=true wf=%d import_task_id=%q requested_url=%q observed_source_url=%q observed_author=%q context_validation_result=passed filter_override_applied=true",
			wf.ID, taskID, eval.primaryURL, sourceURL, obs.AuthorName)
	case v.IdentityMatched:
		// The REQUESTED post came back poisoned (foreign group/page context or garbage content).
		// Refuse to create a lead or stamp the canonical URL; fail the workflow with a typed
		// reason instead of poisoning a lead.
		log.Printf("[ConnectorCrawl] direct_post_intake=true wf=%d import_task_id=%q requested_url=%q observed_source_url=%q observed_author=%q observed_author_profile_url=%q context_validation_result=failed context_mismatch_reason=%s observed_content_preview=%q",
			wf.ID, taskID, wf.CanonicalPostURL, sourceURL, obs.AuthorName, obs.AuthorProfileURL, v.Reason, contentPreview(content))
		eval.failureCode = importContextMismatchCode(v.Reason)
		h.failDirectPostImport(ctx, orgID, wf, eval.failureCode, "direct-post import item failed context/content validation")
		eval.failedWorkflow = true
	default:
		// A different/neighbour post (identity not the requested one) — let normal market
		// filtering decide; do not force, do not fail the workflow.
		log.Printf("[ConnectorCrawl] direct_post_intake=true wf=%d import_task_id=%q observed_source_url=%q context_validation_result=skipped reason=%s",
			wf.ID, taskID, sourceURL, v.Reason)
	}
	return eval
}

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

// directPostImportFailureCode decides the terminal code for a FINISHED direct-post import,
// given whether a valid requested-post lead was force-created (validObserved) and whether an
// item-level guard already failed the workflow (alreadyFailed). It returns ("", false) when
// nothing more is needed; otherwise DPErrImportNoObservedItem — the connector finished but
// the requested post was never positively observed (no silent retry-forever).
func directPostImportFailureCode(validObserved, alreadyFailed bool) (string, bool) {
	if validObserved || alreadyFailed {
		return "", false
	}
	return coordination.DPErrImportNoObservedItem, true
}

// importContextMismatchCode maps a directpost validation reason to the typed terminal
// workflow error for the ingest path (P1.3C granular codes). Only reached for an item that
// POSITIVELY matched the requested post id but failed context/content (IdentityMatched).
func importContextMismatchCode(reason string) string {
	switch reason {
	case directpost.ReasonGroupConflict:
		return coordination.DPErrImportGroupMismatch
	case directpost.ReasonContentInvalid:
		return coordination.DPErrImportBoilerplateContent
	default:
		return coordination.DPErrImportRejectedByGuard
	}
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
