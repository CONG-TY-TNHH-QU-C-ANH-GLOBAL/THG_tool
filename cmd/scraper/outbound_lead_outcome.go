package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
)

// Comment-quality screening (ScreenCommentQuality / enforceContactPolicy) moved to
// internal/services/facebook (Phase C). This file keeps the outbound outcome/queue
// handling + the store-free run accumulator (leadOutreachState) it feeds.

// leadOutreachState accumulates one queueLeadOutreach pass's counters + diagnostics.
// Pure (store-free) — colocated with the outcome formatters that read it.
type leadOutreachState struct {
	queued        int
	skipped       int
	approvedCount int
	scanned       int
	skipReasons   map[string]int
	// skipSamples keeps up to 5 sample lead IDs per skip reason (diagnosability).
	skipSamples map[string][]int64
	lastGenErr  error
	// riskBlock* capture the last risk_ceiling_exceeded deny for the response.
	riskBlockSeen    bool
	riskBlockRisk    float64
	riskBlockCeiling float64
}

func newLeadOutreachState() *leadOutreachState {
	return &leadOutreachState{
		skipReasons: map[string]int{},
		skipSamples: map[string][]int64{},
	}
}

func (s *leadOutreachState) recordSkip(reason string, leadID int64) {
	s.skipped++
	s.skipReasons[reason]++
	if leadID > 0 && len(s.skipSamples[reason]) < 5 {
		s.skipSamples[reason] = append(s.skipSamples[reason], leadID)
	}
}

// queueOutreachMessage inserts one queued outbound message and records the
// queue-stage outcome via the outboundRecorder port (ARCHCM2c seam — no direct
// *store.Store). Returns a non-nil error only on a hard store failure (preserving
// the original `return "", err`); a policy denial is recorded as a skip.
func (c *leadOutreachContext) queueOutreachMessage(ctx context.Context, lead models.Lead, targetURL, content, retrievalID string, st *leadOutreachState) error {
	result, err := c.outbound.QueueOutbound(&models.OutboundMessage{
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
	if !result.Allowed {
		st.recordSkip(result.Reason, lead.ID)
		if result.Reason == "risk_ceiling_exceeded" && result.RiskCeiling > 0 {
			st.riskBlockSeen = true
			st.riskBlockRisk = result.RiskScore
			st.riskBlockCeiling = result.RiskCeiling
		}
		// Record the rejection outcome so Operator Replay shows
		// "retrieved → drafted → rejected (reason)".
		if retrievalID != "" {
			c.outbound.RecordOutcome(ctx, c.orgID, retrievalID, "rejected")
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
		c.outbound.RecordOutcome(ctx, c.orgID, retrievalID, "queued")
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
		notify(facebook.FormatOutboundNotification(c.orgID, c.accountID, c.msgType, st.queued, st.skipped, mode))
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
		skipNote = fmt.Sprintf(" Bỏ qua %d lead (%s).", st.skipped, facebook.FriendlySkipReasons(st.skipReasons))
	}
	if st.queued == 0 {
		// Lead Lifecycle PR-5: degrade honestly — report what the org DOES have
		// (waiting/follow-up/archived) and a next step, not a dead-end "0 queued".
		return noEligibleCommentMessage(ctx, c.lifecycle, c.orgID, st.scanned, skipNote) + errDetails
	}
	// PR-5: name the source group ("Cần xử lý") so the operator knows selection came
	// from the act-now work queue, not the raw lead list.
	return fmt.Sprintf(
		"Đã đưa %d comment vào hàng đợi từ nhóm Cần xử lý sau khi quét %d lead. Đây CHƯA phải là đã đăng lên Facebook — hệ thống sẽ chạy bằng các tài khoản Facebook sẵn sàng và báo lại từng kết quả. Tóm tắt: %d đang chờ · 0 đang chạy · 0 đã đăng · 0 thất bại.%s%s",
		st.queued, st.scanned, st.queued, skipNote, errDetails,
	)
}
