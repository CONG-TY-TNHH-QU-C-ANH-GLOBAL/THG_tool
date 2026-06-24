package facebook

import (
	"github.com/thg/scraper/internal/ai/comment"
	"github.com/thg/scraper/internal/models"
)

// Facebook comment-quality gate (Phase C). Moved verbatim out of cmd/scraper so the FB
// comment workflow owns its pre-queue quality boundary. Pure: orchestrates the
// internal/ai/comment intelligence primitives over a models.CompanyIdentity; no store, no IO.

// ScreenCommentQuality runs the PR-1 comment-quality boundary: sanitize + dedupe,
// reject A+A repeats, enforce the brand-trust contact policy (repair-then-rescreen),
// then deterministically ensure the workspace website. Returns (cleaned, "") to
// proceed or ("", skipReason). Comment-type callers only.
func ScreenCommentQuality(content string, identity models.CompanyIdentity) (string, string) {
	cleaned, ok, qreason := comment.SanitizeComment(content)
	if !ok {
		return "", qreason
	}
	content = cleaned
	// Duplicate guard (incident PR-1): an A+A repeated block must never enter the
	// outbox, even if it survived sentence-level dedup.
	if comment.DetectRepeatedText(content) {
		return "", "comment_quality_duplicate_text"
	}
	screened, skip := enforceContactPolicy(content, identity)
	if skip != "" {
		return "", skip
	}
	content = screened
	// Deterministic website inclusion: a configured workspace website must appear
	// in every comment. Grounded-only — never invents a URL, no-op when present.
	if web, added := comment.EnsureWebsite(content, identity); added {
		content = web
	}
	return content, ""
}

// enforceContactPolicy screens contacts (≤1 URL, grounded website/official contact,
// no fabricated email/phone). On a violation it REPAIRs toward the Company Identity
// and re-screens; only drops the lead if the repaired comment still fails. Returns
// (content, "") to proceed or ("", skipReason).
func enforceContactPolicy(content string, identity models.CompanyIdentity) (string, string) {
	cok, creason := comment.ScreenCommentContacts(content, identity)
	if cok {
		return content, ""
	}
	repaired, changed := comment.RepairCommentContacts(content, identity)
	rok, rreason := comment.ScreenCommentContacts(repaired, identity)
	if changed && rok {
		return repaired, ""
	}
	if changed {
		return "", rreason
	}
	return "", creason
}
