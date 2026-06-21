package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/thg/scraper/internal/ai/comment"
	"github.com/thg/scraper/internal/models"
)

// screenCommentQuality runs the PR-1 comment-quality boundary: sanitize + dedupe,
// reject A+A repeats, enforce the brand-trust contact policy (repair-then-rescreen),
// then deterministically ensure the workspace website. Returns (cleaned, "") to
// proceed or ("", skipReason). Comment-type callers only.
func screenCommentQuality(content string, identity models.CompanyIdentity) (string, string) {
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

// queueOutreachMessage inserts one queued outbound message and records the
// queue-stage outcome. Returns a non-nil error only on a hard store failure
// (preserving the original `return "", err`); a policy denial is recorded as a skip.
func (c *leadOutreachContext) queueOutreachMessage(ctx context.Context, lead models.Lead, targetURL, content, retrievalID string, st *leadOutreachState) error {
	result, err := c.db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:      c.orgID,
		Type:       c.msgType,
		Platform:   models.PlatformFacebook,
		AccountID:  c.accountID,
		TargetURL:  targetURL,
		TargetName: lead.Author,
		Content:    content,
		Context:    lead.Content,
		AIModel:    "agent",
		CreatedBy:  c.actx.InitiatorUserID, // immutable execution ownership (from ActionContext)
	}, 24*time.Hour)
	if err != nil {
		return err
	}
	if !result.Decision.Allowed {
		st.recordSkip(result.Decision.Reason, lead.ID)
		if result.Decision.Reason == "risk_ceiling_exceeded" && result.Decision.RiskCeiling > 0 {
			st.riskBlockSeen = true
			st.riskBlockRisk = result.Decision.RiskScore
			st.riskBlockCeiling = result.Decision.RiskCeiling
		}
		// Record the rejection outcome so Operator Replay shows
		// "retrieved → drafted → rejected (reason)".
		if retrievalID != "" {
			c.db.Knowledge().RecordOutcome(ctx, c.orgID, retrievalID, "rejected")
		}
		return nil
	}
	st.queued++
	if result.ExecutionState == models.ExecPlanned {
		st.approvedCount++
	}
	// Stage outcome: queue success. The browser-execution layer owns the FINAL
	// sent/failed outcome against the same retrievalID.
	if retrievalID != "" {
		c.db.Knowledge().RecordOutcome(ctx, c.orgID, retrievalID, "queued")
	}
	return nil
}

// outreachMode derives the queue-result mode label from how many messages were
// auto-approved vs queued and whether the caller requested auto. Shared by the
// lead-outreach and Facebook-post queue paths (identical logic).
func outreachMode(approvedCount, queued int, requestedAuto bool) string {
	switch {
	case approvedCount > 0 && approvedCount == queued:
		return "approved_auto"
	case approvedCount > 0:
		return "mixed"
	case requestedAuto:
		// Caller asked for auto but the org is not opted in; make it visible.
		return "draft_org_not_auto"
	}
	return "draft"
}

func outreachErrDetails(lastGenErr error) string {
	if lastGenErr != nil {
		return fmt.Sprintf(" | Last Error: %v", lastGenErr)
	}
	return ""
}

func outreachRiskDetails(st *leadOutreachState, accountID int64) string {
	if st.riskBlockSeen {
		return fmt.Sprintf(" risk_block=account=%d,risk_score=%.3f,effective_ceiling=%.3f", accountID, st.riskBlockRisk, st.riskBlockCeiling)
	}
	return ""
}

// formatOutreachResult emits the run notification + structured log line and returns
// the caller-facing summary (comment vs other msgType), preserving the original
// strings exactly.
func (c *leadOutreachContext) formatOutreachResult(ctx context.Context, requestedAuto bool, notify func(string), st *leadOutreachState) string {
	mode := outreachMode(st.approvedCount, st.queued, requestedAuto)
	if notify != nil && st.queued > 0 {
		notify(formatOutboundNotification(c.orgID, c.accountID, c.msgType, st.queued, st.skipped, mode))
	}
	// Investigation ask §1: one structured, diagnosable line per run — scanned vs
	// queued vs skipped, the reason histogram AND sample lead IDs per reason.
	log.Printf("[queueLeadOutreach] org=%d type=%s scanned=%d queued=%d skipped=%d reasons=%v samples=%v",
		c.orgID, c.msgType, st.scanned, st.queued, st.skipped, st.skipReasons, st.skipSamples)

	errDetails := outreachErrDetails(st.lastGenErr)
	if c.msgType == "comment" {
		return c.formatCommentResult(ctx, st, errDetails)
	}
	riskDetails := outreachRiskDetails(st, c.accountID)
	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s reasons=%v%s%s", c.msgType, st.queued, st.skipped, mode, st.skipReasons, riskDetails, errDetails)
}

// formatCommentResult builds the business-friendly comment summary (queued ≠ posted),
// degrading honestly to noEligibleCommentMessage when nothing was queued.
func (c *leadOutreachContext) formatCommentResult(ctx context.Context, st *leadOutreachState, errDetails string) string {
	// Business-friendly: queued ≠ posted. Surface a status summary.
	skipNote := ""
	if st.skipped > 0 {
		skipNote = fmt.Sprintf(" Bỏ qua %d lead (%s).", st.skipped, friendlySkipReasons(st.skipReasons))
	}
	if st.queued == 0 {
		// Lead Lifecycle PR-5: degrade honestly — report what the org DOES have
		// (waiting/follow-up/archived) and a next step, not a dead-end "0 queued".
		return noEligibleCommentMessage(ctx, c.db, c.orgID, st.scanned, skipNote) + errDetails
	}
	// PR-5: name the source group ("Cần xử lý") so the operator knows selection came
	// from the act-now work queue, not the raw lead list.
	return fmt.Sprintf(
		"Đã đưa %d comment vào hàng đợi từ nhóm Cần xử lý sau khi quét %d lead. Đây CHƯA phải là đã đăng lên Facebook — hệ thống sẽ chạy bằng các tài khoản Facebook sẵn sàng và báo lại từng kết quả. Tóm tắt: %d đang chờ · 0 đang chạy · 0 đã đăng · 0 thất bại.%s%s",
		st.queued, st.scanned, st.queued, skipNote, errDetails,
	)
}
