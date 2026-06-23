package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// Pre-comment invariant for direct-post intake (P1.3B). Even a strict-canonical-matched
// lead (P1.1) must be re-validated before it can produce a comment: a poisoned lead
// already in the DB — a foreign-group author or boilerplate content — must NOT be
// commented on. This is the second, independent guard (the ingest gate is P1.3A); it
// blocks a bad lead that exists for any reason, including pre-fix rows.

// directPostLeadTargetMismatch reports whether the resolved lead's context/content
// conflicts with the requested target, with a typed reason. Pure (delegates to the
// shared directpost invariants) so it is unit-testable without a DB.
func directPostLeadTargetMismatch(w *coordination.DirectPostCommentWorkflow, lead *models.Lead) (string, bool) {
	if conflict, reason := directpost.ContextConflict(w.GroupRef, lead.SourceURL, lead.AuthorURL, lead.GroupFBID); conflict {
		return reason, true
	}
	if !directpost.ValidContent(lead.Content) {
		return directpost.ReasonContentInvalid, true
	}
	return "", false
}

// blockDirectPostComment fails the workflow with the typed target-mismatch reason and logs
// safe diagnostics (post text + URLs are user-visible Facebook data — no cookies/tokens/
// session/browser state).
func blockDirectPostComment(ctx context.Context, db *store.Store, w *coordination.DirectPostCommentWorkflow, lead *models.Lead, reason string) {
	log.Printf("[DirectPostIntake] PRE-COMMENT BLOCK org=%d wf=%d lead_id=%d canonical=%q expected_group_ref=%q lead_source_url=%q lead_author=%q lead_author_url=%q content_preview=%q context_mismatch_reason=%s",
		w.OrgID, w.ID, lead.ID, w.CanonicalPostURL, w.GroupRef, lead.SourceURL, lead.Author, lead.AuthorURL, directPostContentPreview(lead.Content), reason)
	_, _ = db.Coordination().MarkDirectPostFailed(ctx, w.OrgID, w.ID,
		coordination.DPErrLeadTargetMismatch, "lead content/context conflicts with requested target")
}

// directPostFailureReasons maps a typed workflow error_code to a short, honest Vietnamese
// explanation the requester sees (P1.3C — no overpromise, clear failed reason). Unknown
// codes degrade to a generic verification-failed message rather than silence.
var directPostFailureReasons = map[string]string{
	coordination.DPErrIdentityMismatch:          "bài viết tìm thấy không khớp đúng bài/nhóm bạn yêu cầu",
	coordination.DPErrLeadTargetMismatch:        "nội dung/ngữ cảnh bài viết không khớp mục tiêu — không comment để tránh sai bài",
	coordination.DPErrImportNoObservedItem:      "không đọc được đúng bài viết bạn yêu cầu (có thể tài khoản chưa ở trong nhóm hoặc bài không hiển thị)",
	coordination.DPErrImportGroupMismatch:       "bài đọc được thuộc nhóm/trang khác với yêu cầu",
	coordination.DPErrImportBoilerplateContent:  "không trích xuất được nội dung bài viết hợp lệ",
	coordination.DPErrImportNoMeaningfulContent: "bài viết không có nội dung đủ để comment",
	coordination.DPErrLeadNotObserved:           "chưa quan sát được bài viết sau nhiều lần thử",
	coordination.DPErrCommentNotQueued:          "chưa đưa được comment vào hàng đợi — tài khoản Facebook chưa sẵn sàng hoặc lead không đủ điều kiện (mở dashboard để kiểm tra)",
}

// notifyDirectPostFailed sends one honest, secret-free failure line to the requester when a
// direct-post workflow reaches a terminal failure. Best-effort: nil notify is a no-op.
func notifyDirectPostFailed(notify func(string), w *coordination.DirectPostCommentWorkflow, code string) {
	if notify == nil {
		return
	}
	reason := directPostFailureReasons[code]
	if reason == "" {
		reason = "không xác minh được bài viết/ngữ cảnh"
	}
	notify(fmt.Sprintf("⚠️ Không thể comment bài %s — %s (mã: %s).", w.CanonicalPostURL, reason, code))
}

// directPostContentPreview returns a short, single-line, secret-free content snippet.
func directPostContentPreview(content string) string {
	s := strings.Join(strings.Fields(content), " ")
	const max = 160
	if len([]rune(s)) > max {
		return string([]rune(s)[:max]) + "…"
	}
	return s
}
