package agent

import (
	"context"
	"log/slog"
	"strings"

	"github.com/thg/scraper/internal/store/coordination"
)

// Direct-post import terminal-failure surfacing (P1.3 UX). When an explicit "Comment bài này"
// import fails terminally, the requester previously saw only the async ack — the typed reason
// lived in the workflow row + logs + (optionally) an admin Telegram message. failDirectPostImport
// also records the reason in the REQUESTER's own Copilot history so they see why no comment was
// sent. directpost.Validate and the fail-closed semantics are unchanged; this only surfaces them.

// directPostFailureUserMessage maps a typed terminal import code to a clear, secret-free
// Vietnamese message for the requester's Copilot/dashboard history.
func directPostFailureUserMessage(code string) string {
	switch code {
	case coordination.DPErrImportGroupMismatch:
		return "Không comment vì bài import bị lệch group/context. Vui lòng kiểm tra lại link hoặc quyền xem bài."
	case coordination.DPErrImportNoObservedItem:
		return "Không comment vì trình duyệt chưa quan sát được bài viết mục tiêu. Vui lòng mở/reload Chrome profile hoặc kiểm tra quyền xem bài."
	case coordination.DPErrImportBoilerplateContent, coordination.DPErrImportNoMeaningfulContent:
		return "Không comment vì trình duyệt chỉ quan sát được nội dung không hợp lệ hoặc giao diện Facebook, chưa phải bài viết thật."
	case coordination.DPErrImportTargetNotRendered:
		return "Không comment vì trình duyệt chưa hiển thị được bài viết mục tiêu (có thể bài bị giới hạn quyền xem, đã xoá, hoặc tải chậm). Hãy mở/reload Chrome đúng profile rồi thử lại."
	default:
		return "Không comment vì không xác minh được bài viết mục tiêu. Vui lòng thử lại hoặc kiểm tra quyền xem bài."
	}
}

// directPostFailureCodeFromExtensionError maps an extension-reported crawl error string for a
// DIRECT-POST import to a typed terminal workflow code (P1.3E). The extension returns a typed
// error in crawl_result.error (e.g. "direct_post_target_not_rendered"); reuse the existing
// granular codes where one exists so the requester sees one consistent taxonomy.
func directPostFailureCodeFromExtensionError(errMsg string) string {
	switch s := strings.ToLower(strings.TrimSpace(errMsg)); {
	case strings.Contains(s, "target_not_rendered"), strings.Contains(s, "wrong_page"):
		return coordination.DPErrImportTargetNotRendered
	case strings.Contains(s, "boilerplate"):
		return coordination.DPErrImportBoilerplateContent
	case strings.Contains(s, "group_mismatch"):
		return coordination.DPErrImportGroupMismatch
	case strings.Contains(s, "post_mismatch"), strings.Contains(s, "post_id_mismatch"):
		return coordination.DPErrImportPostIDMismatch
	default:
		return coordination.DPErrImportNoObservedItem
	}
}

// failDirectPostImport marks the workflow failed (CAS-guarded) and — only when THIS call won
// the transition — records the typed reason in the requester's private Copilot history. The
// CAS gate makes it safe to call for every poisoned item: only the first transition surfaces a
// message, so multiple poisoned items in one import never spam the requester.
func (h *Handler) failDirectPostImport(ctx context.Context, orgID int64, wf *coordination.DirectPostCommentWorkflow, code, internalMsg string) {
	if wf == nil {
		return
	}
	changed, _ := h.db.Coordination().MarkDirectPostFailed(ctx, orgID, wf.ID, code, internalMsg)
	if !changed || wf.RequestedByUserID <= 0 {
		return
	}
	_ = h.db.Prompts().InsertUserAutomationLog(orgID, wf.AccountID, wf.RequestedByUserID,
		directPostFailureUserMessage(code), "direct_post_failed", code, false)
}

// logDirectPostImportForensics emits one structured line per direct-post import so the phantom
// DOM class of bug is debuggable: it ties the connector's scroll diagnostics (max_articles_seen,
// passes, scroll_moved_ever, final_scroll_target — read panic-safe via normalizeConnectorScrollDiag)
// to the import outcome (how many raw items arrived, whether a valid post was observed, and the
// terminal failure code). Elevated to Warn when the import failed terminally.
func logDirectPostImportForensics(ctx context.Context, req connectorCrawlResultRequest, wf *coordination.DirectPostCommentWorkflow, dpValidObserved, dpFailed bool, finalCode string) {
	if wf == nil {
		return
	}
	sd := normalizeConnectorScrollDiag(req.ScrollDiag)
	attrs := []any{
		"task_id", req.TaskID,
		"workflow_id", wf.ID,
		"expected_post_fbid", wf.PostFBID,
		"expected_group_ref", wf.GroupRef,
		"raw_items", len(req.Items),
		"max_articles_seen", sd.MaxArticlesSeen,
		"passes", sd.Passes,
		"max_doc_height", sd.MaxDocHeight,
		"scroll_moved_ever", sd.ScrollMovedEver,
		"final_scroll_target", sd.FinalScrollTarget,
		"valid_observed", dpValidObserved,
		"terminal_failed", dpFailed || finalCode != "",
		"final_failure_code", finalCode,
	}
	if dpFailed || finalCode != "" {
		slog.WarnContext(ctx, "direct-post import forensic: terminal failure", attrs...)
		return
	}
	slog.InfoContext(ctx, "direct-post import forensic", attrs...)
}
