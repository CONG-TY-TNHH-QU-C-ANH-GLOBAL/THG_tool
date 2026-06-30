package crawlingest

import (
	"strings"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/store/coordination"
)

// Direct-post import OUTCOME classification — pure, deterministic mapping from a
// validation/extension signal to a typed terminal code (or a requester-facing
// message). No DB, no IO, no Handler receiver. Extracted verbatim from
// crawl_direct_post.go / crawl_direct_post_failure.go to keep the handlers thin;
// behavior and the typed taxonomy are unchanged.

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
