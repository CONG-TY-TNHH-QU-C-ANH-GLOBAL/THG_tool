package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// Direct-post intake application service (P1 PR-2 runtime). The narrow seam the
// Copilot direct-comment handler (commentSinglePost) calls when a post is NOT yet a
// lead: it creates/resumes a durable workflow (PR-1 store) and enqueues EXACTLY ONE
// single-post import per intake_key, then returns the async acknowledgement. It does
// NOT persist leads, send Telegram, or queue comments — those happen in the existing
// ingest + outbound paths; the poller (direct_post_intake_scheduler.go) drives the
// continuation. Copilot owns none of this persistence.

// directPostIntakeAck is the async acknowledgement for an unknown post. It does NOT
// promise an automatic comment (P1.3C): the comment only happens if the import positively
// verifies the requested post + context. Overpromising "will auto-comment" was wrong — a
// failed verification must surface honestly via the workflow's typed error_code.
const directPostIntakeAck = "Đã nhận bài viết. Mình sẽ import và CHỈ comment nếu xác minh đúng bài viết và đúng context. Nếu không xác minh được, mình sẽ báo lý do thay vì comment sai."

type directPostIntake struct {
	db       *store.Store
	jobStore *jobs.Store
}

func newDirectPostIntake(db *store.Store, jobStore *jobs.Store) *directPostIntake {
	return &directPostIntake{db: db, jobStore: jobStore}
}

// directPostCommentInput is the request payload (canonical URL + actor identity).
type directPostCommentInput struct {
	OrgID             int64
	RequestedByUserID int64
	AccountID         int64
	UserRole          string
	CanonicalPostURL  string
	PostFBID          string
	GroupRef          string
	Prompt            string
}

// request creates/resumes the workflow and ensures a single-post import is in flight
// for the post, then returns the async ack. Idempotent: a repeat request for the same
// actor/action reuses the workflow; a different actor for the same post shares the
// one import (FindActiveImportTaskForIntakeKey).
func (s *directPostIntake) request(ctx context.Context, in directPostCommentInput) (string, error) {
	in.CanonicalPostURL = strings.TrimSpace(in.CanonicalPostURL)
	if in.OrgID <= 0 || in.CanonicalPostURL == "" {
		return "", fmt.Errorf("direct-post intake requires org_id and canonical_post_url")
	}
	w, err := s.db.Coordination().CreateOrGetDirectPostCommentWorkflow(ctx, coordination.DirectPostWorkflowInput{
		OrgID: in.OrgID, RequestedByUserID: in.RequestedByUserID, UserRole: in.UserRole,
		AccountID: in.AccountID, CanonicalPostURL: in.CanonicalPostURL,
		PostFBID: in.PostFBID, GroupRef: in.GroupRef, Prompt: in.Prompt,
	})
	if err != nil {
		return "", err
	}
	// A fresh request after a TERMINAL failure re-opens the workflow so the import
	// retries — otherwise the ack would lie (promise a comment a dead workflow can't
	// deliver). Completed / in-progress workflows are left as-is.
	if w.Status == coordination.DPStatusFailed || w.Status == coordination.DPStatusCancelled {
		if ok, _ := s.db.Coordination().ResetDirectPostWorkflowForRetry(ctx, in.OrgID, w.ID); ok {
			if rw, _ := s.db.Coordination().GetDirectPostCommentWorkflowByID(ctx, in.OrgID, w.ID); rw != nil {
				w = rw
			}
		}
	}
	// Already past the import gate (in-flight or done) → don't re-enqueue; just ack.
	if w.Status != coordination.DPStatusRequested {
		return directPostIntakeAck, nil
	}
	// One import per intake_key: reuse a shared in-flight import if present.
	intakeKey := coordination.DirectPostIntakeKey(in.OrgID, in.CanonicalPostURL)
	taskID, err := s.db.Coordination().FindActiveImportTaskForIntakeKey(ctx, in.OrgID, intakeKey)
	if err != nil {
		return "", err
	}
	if taskID == "" {
		// Enqueue exactly one single-post import (facebook_post, max_items=1). If it
		// cannot be dispatched, leave the workflow in 'requested' so the poller/user
		// can retry, and still ack honestly (we accepted the post).
		newTask, derr := s.enqueueSinglePostImport(ctx, in, w.ID)
		if derr != nil {
			return directPostIntakeAck, nil
		}
		taskID = newTask
	}
	_, _ = s.db.Coordination().MarkDirectPostImportQueued(ctx, in.OrgID, w.ID, taskID)
	return directPostIntakeAck, nil
}

// directPostImportArgs builds the single-post crawl args, PINNING the import to the action
// account (workflow.account_id) when present so the read happens from the same viewpoint
// that will comment. When AccountID is absent we leave account_id unset (the caller logs a
// warning); we never substitute a different account for an explicit direct-post command.
func directPostImportArgs(in directPostCommentInput) map[string]any {
	args := map[string]any{
		"org_id":      in.OrgID,
		"user_id":     in.RequestedByUserID,
		"user_role":   in.UserRole,
		"max_items":   int64(1), // exactly this one post — never a broad crawl
		"user_prompt": in.Prompt,
		// P1.3E target identity: the extension uses this to wait for + extract ONLY the
		// requested post container (never a sidebar/related/foreign-group candidate), and to
		// return a typed failure when the target is not rendered. submitOpenCrawl forwards it
		// into task.extras["direct_post_target"]; broad crawl never sets these keys.
		"direct_post_post_fbid": in.PostFBID,
		"direct_post_group_ref": in.GroupRef,
		"direct_post_canonical": in.CanonicalPostURL,
	}
	if in.AccountID > 0 {
		args["account_id"] = in.AccountID // PIN — no cross-account auto-pick
	}
	return args
}

// enqueueSinglePostImport submits ONE facebook_post crawl (max_items=1) and returns
// the deterministic task id it was filed under.
//
// P1.3C account routing: the import is PINNED to the action account (workflow.account_id)
// — the same account that will post the comment. The earlier design left account_id unset
// and let submitOpenCrawl auto-pick ANY ready connector, which let the import run on a
// different account (#50) than the comment (#49); if that import account is not a member of
// the target group, Facebook serves a wrong/unavailable post and the explicit intake either
// imports the wrong content or nothing. Pinning makes the read happen from the SAME
// viewpoint that will act. We do NOT silently fall back to another account for an explicit
// direct-post command (fail closed): if the action account's connector cannot run the
// import, submitOpenCrawl returns an error and the workflow surfaces it rather than reading
// the post through a stranger's session.
func (s *directPostIntake) enqueueSinglePostImport(ctx context.Context, in directPostCommentInput, workflowID int64) (string, error) {
	args := directPostImportArgs(in)
	if in.AccountID <= 0 {
		log.Printf("[DirectPostIntake] WARN org=%d wf=%d no action account on workflow; import cannot be account-pinned canonical=%q",
			in.OrgID, workflowID, in.CanonicalPostURL)
	}
	sources := []jobs.Source{{Type: "facebook_post", URL: in.CanonicalPostURL, Label: "direct_post_intake"}}
	if _, err := submitOpenCrawl(ctx, s.db, s.jobStore, "facebook_crawl", sources, args); err != nil {
		return "", err
	}
	taskID := openCrawlTaskID("facebook_crawl", sources, args)
	importAccount := argInt64(args, "account_id")
	// Structured correlation log. action_account == import_account is now the invariant for
	// explicit direct-post; a divergence here is a bug to investigate (see error code
	// direct_post_import_account_mismatch).
	log.Printf("[DirectPostIntake] single-post import enqueued org=%d wf=%d action_account=%d import_account=%d account_pinned=%t import_task_id=%q expected_post_fbid=%q expected_group_ref=%q canonical=%q",
		in.OrgID, workflowID, in.AccountID, importAccount, in.AccountID > 0 && importAccount == in.AccountID,
		taskID, in.PostFBID, in.GroupRef, in.CanonicalPostURL)
	return taskID, nil
}
