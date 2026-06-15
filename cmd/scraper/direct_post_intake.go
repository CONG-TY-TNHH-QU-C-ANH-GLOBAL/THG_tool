package main

import (
	"context"
	"fmt"
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

// directPostIntakeAck is the async acknowledgement for an unknown post — promised
// ONLY because a durable workflow + single-post import are actually created here.
const directPostIntakeAck = "Đã nhận bài viết này. Mình sẽ đưa bài viết vào leads của workspace, đọc nội dung và tự động comment khi đủ dữ liệu."

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
		newTask, derr := s.enqueueSinglePostImport(ctx, in)
		if derr != nil {
			return directPostIntakeAck, nil
		}
		taskID = newTask
	}
	_, _ = s.db.Coordination().MarkDirectPostImportQueued(ctx, in.OrgID, w.ID, taskID)
	return directPostIntakeAck, nil
}

// enqueueSinglePostImport submits ONE facebook_post crawl (max_items=1) and returns
// the deterministic task id it was filed under. The task id is recomputed AFTER
// submitOpenCrawl so it reflects the auto-picked account (submitOpenCrawl mutates
// args), matching the actual job for correlation/dedup.
func (s *directPostIntake) enqueueSinglePostImport(ctx context.Context, in directPostCommentInput) (string, error) {
	// NOTE: account_id is deliberately NOT pinned. The import only needs to READ the
	// post, so submitOpenCrawl auto-picks any ready connector (or falls back to the
	// worker queue) instead of hard-failing when the actor's own connector is offline.
	// The actor's account_id is used later for the COMMENT (workflow.account_id).
	args := map[string]any{
		"org_id":      in.OrgID,
		"user_id":     in.RequestedByUserID,
		"user_role":   in.UserRole,
		"max_items":   int64(1), // exactly this one post — never a broad crawl
		"user_prompt": in.Prompt,
	}
	sources := []jobs.Source{{Type: "facebook_post", URL: in.CanonicalPostURL, Label: "direct_post_intake"}}
	if _, err := submitOpenCrawl(ctx, s.db, s.jobStore, "facebook_crawl", sources, args); err != nil {
		return "", err
	}
	return openCrawlTaskID("facebook_crawl", sources, args), nil
}
