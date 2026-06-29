package main

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// fakeRecorder is a store-free outboundRecorder stub. The seam (ARCHCM2c) is what
// makes queueOutreachMessage testable without a real *store.Store.
type fakeRecorder struct {
	result   queueOutcome
	err      error
	queued   *models.OutboundMessage
	cooldown time.Duration
	outcomes []string // statuses passed to RecordOutcome, in order
}

func (f *fakeRecorder) QueueOutbound(msg *models.OutboundMessage, cooldown time.Duration) (queueOutcome, error) {
	f.queued = msg
	f.cooldown = cooldown
	return f.result, f.err
}

func (f *fakeRecorder) RecordOutcome(_ context.Context, _ int64, _, status string) {
	f.outcomes = append(f.outcomes, status)
}

func newTestOutreachCtx(rec outboundRecorder) *leadOutreachContext {
	return &leadOutreachContext{
		outbound:  rec,
		orgID:     7,
		accountID: 3,
		msgType:   "comment",
		actx:      models.ActionContext{InitiatorUserID: 99},
	}
}

// TestQueueOutreachMessage_Allowed pins the queue-success path: counters bump, an
// ExecPlanned result counts as approved, and a "queued" outcome is recorded.
func TestQueueOutreachMessage_Allowed(t *testing.T) {
	rec := &fakeRecorder{result: queueOutcome{Allowed: true, ExecutionState: models.ExecPlanned}}
	c := newTestOutreachCtx(rec)
	st := newLeadOutreachState()

	if err := c.queueOutreachMessage(context.Background(), models.Lead{ID: 1}, "https://t", "hi", "ret-1", st); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if st.queued != 1 || st.approvedCount != 1 || st.skipped != 0 {
		t.Fatalf("queued=%d approved=%d skipped=%d, want 1/1/0", st.queued, st.approvedCount, st.skipped)
	}
	if len(rec.outcomes) != 1 || rec.outcomes[0] != "queued" {
		t.Fatalf("outcomes=%v, want [queued]", rec.outcomes)
	}
	// 24h cooldown must be preserved verbatim.
	if rec.cooldown != 24*time.Hour {
		t.Fatalf("cooldown=%v, want 24h", rec.cooldown)
	}
	// The queued OutboundMessage payload must be pinned exactly — this PR touches the
	// queue-write seam, so the fields the old c.db.QueueOutboundForOrg path built must
	// pass through the port unchanged. Flat checks keep cognitive complexity low.
	m := rec.queued
	if m == nil {
		t.Fatal("rec.queued is nil, want the queued OutboundMessage")
	}
	if m.OrgID != 7 {
		t.Errorf("OrgID=%d, want 7", m.OrgID)
	}
	if m.AccountID != 3 {
		t.Errorf("AccountID=%d, want 3", m.AccountID)
	}
	if m.Type != "comment" {
		t.Errorf("Type=%q, want \"comment\"", m.Type)
	}
	if m.TargetURL != "https://t" {
		t.Errorf("TargetURL=%q, want \"https://t\"", m.TargetURL)
	}
	if m.Content != "hi" {
		t.Errorf("Content=%q, want \"hi\"", m.Content)
	}
	// CreatedBy is immutable execution ownership sourced from actx.InitiatorUserID (99).
	if m.CreatedBy != 99 {
		t.Errorf("CreatedBy=%d, want 99 (actx.InitiatorUserID)", m.CreatedBy)
	}
}

// TestQueueOutreachMessage_RiskBlock pins the risk_ceiling_exceeded deny: it is a
// skip (not an error), captures the risk block for the response, and records "rejected".
func TestQueueOutreachMessage_RiskBlock(t *testing.T) {
	rec := &fakeRecorder{result: queueOutcome{
		Allowed: false, Reason: "risk_ceiling_exceeded", RiskScore: 0.9, RiskCeiling: 0.5,
	}}
	c := newTestOutreachCtx(rec)
	st := newLeadOutreachState()

	if err := c.queueOutreachMessage(context.Background(), models.Lead{ID: 2}, "https://t", "hi", "ret-2", st); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if st.queued != 0 || st.skipped != 1 {
		t.Fatalf("queued=%d skipped=%d, want 0/1", st.queued, st.skipped)
	}
	if !st.riskBlockSeen || st.riskBlockRisk != 0.9 || st.riskBlockCeiling != 0.5 {
		t.Fatalf("riskBlock seen=%v risk=%v ceil=%v, want true/0.9/0.5", st.riskBlockSeen, st.riskBlockRisk, st.riskBlockCeiling)
	}
	if len(rec.outcomes) != 1 || rec.outcomes[0] != "rejected" {
		t.Fatalf("outcomes=%v, want [rejected]", rec.outcomes)
	}
}

// TestQueueOutreachMessage_NoRetrievalID: no Knowledge outcome is recorded when the
// retrievalID is empty (preserves the original `if retrievalID != ""` guards).
func TestQueueOutreachMessage_NoRetrievalID(t *testing.T) {
	rec := &fakeRecorder{result: queueOutcome{Allowed: true}}
	c := newTestOutreachCtx(rec)
	st := newLeadOutreachState()

	if err := c.queueOutreachMessage(context.Background(), models.Lead{ID: 3}, "https://t", "hi", "", st); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(rec.outcomes) != 0 {
		t.Fatalf("outcomes=%v, want none", rec.outcomes)
	}
}
