package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store/coordination"
)

// FIRST-WIN side effects for outbound finalization: the attempt-scoped writes
// committed exactly once per terminal (begin/finish attempt, failure evidence,
// actor verdict, action_ledger, risk signal, quota refund, inbox thread). All are
// best-effort telemetry — failures are logged, never propagated to the callback.

// applyFirstWinSideEffects commits the once-per-terminal attempt-scoped writes.
// BeginExecutionAttempt failure is best-effort: it logs, leaves attemptID=0, and
// skips the attempt-scoped writes (finish/verdict/ledger/risk/events) — quota,
// inbox and notify still run afterwards, exactly as the former inline branch.
func (f *outboundFinalizer) applyFirstWinSideEffects(ctx context.Context) {
	attemptID, err := f.h.db.Coordination().BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID:      f.orgID,
		OutboundID: f.id,
		AccountID:  f.msg.AccountID,
		TargetURL:  f.msg.TargetURL,
		ActionType: f.msg.Type,
		Attempt:    1,
		Status:     models.AttemptVerifying,
	})
	if err != nil {
		slog.WarnContext(ctx, "exec-verify: begin attempt failed",
			"org_id", f.orgID, "outbound_id", f.id, "error", err)
		f.attemptID = 0
		return
	}
	f.attemptID = attemptID
	f.failureReason = f.computeFailureReason()
	f.persistFailureEvidence(ctx)
	if err := f.h.db.Coordination().FinishExecutionAttempt(ctx, f.attemptID, f.outcome, f.failureReason, proofToEvidence(f.proof)); err != nil {
		slog.WarnContext(ctx, "exec-verify: finish attempt failed",
			"attempt_id", f.attemptID, "outcome", f.outcome, "error", err)
	}
	f.recordActorVerdict(ctx)
	f.recordActionLedger(ctx)
	f.applyRiskSignal(ctx)
	events.Info(ctx, events.ExecutionVerified,
		events.FieldOutboundID, f.id,
		events.FieldAttemptID, f.attemptID,
		events.FieldOutcome, f.outcome,
		events.FieldAccountID, f.msg.AccountID,
		events.FieldActionType, f.msg.Type,
	)
}

// computeFailureReason mirrors the inline reason derivation: empty on success,
// else the reported reason, falling back to the outcome string.
func (f *outboundFinalizer) computeFailureReason() string {
	if models.IsSuccessOutcome(f.outcome) {
		return ""
	}
	if f.report.FailureReason != "" {
		return f.report.FailureReason
	}
	return string(f.outcome)
}

// persistFailureEvidence (PR8A) writes the failing-tab screenshot (non-success +
// b64 present) to an org-scoped path and rides it into proof.NavDiagnostic.Screenshot
// Path BEFORE FinishExecutionAttempt. FIRST-WIN only so a replay cannot rewrite it.
// Best-effort: telemetry only, never propagated.
func (f *outboundFinalizer) persistFailureEvidence(ctx context.Context) {
	if !models.IsSuccessOutcome(f.outcome) && f.report.EvidenceScreenshotB64 != "" {
		if path, sErr := persistEvidenceScreenshot(f.orgID, f.id, f.attemptID, f.report.EvidenceScreenshotB64); sErr != nil {
			slog.WarnContext(ctx, "exec-verify: evidence screenshot persist failed",
				"org_id", f.orgID, "outbound_id", f.id, "attempt_id", f.attemptID, "error", sErr)
		} else if path != "" {
			if f.proof.NavDiagnostic == nil {
				f.proof.NavDiagnostic = &models.NavDiagnostic{}
			}
			f.proof.NavDiagnostic.ScreenshotPath = path
		}
	}
}

// recordActorVerdict is the Verified Actor integrity gate (P1b). Best-effort,
// extracted verbatim: compares expected vs observed Facebook identity, stamps the
// append-only verdict on the attempt + account runtime state, and BLOCKS the
// account from auto-execute on a definite mismatch. Errors are logged, never fatal.
func (f *outboundFinalizer) recordActorVerdict(ctx context.Context) {
	if f.attemptID > 0 && f.msg.AccountID > 0 {
		expectedFB, actualFB := f.resolveActorFBIdentities()
		verdict := coordination.ClassifyActorVerdict(expectedFB, actualFB)
		if err := f.h.db.Coordination().MarkAttemptActorVerification(ctx, f.attemptID, expectedFB, actualFB, verdict); err != nil {
			slog.WarnContext(ctx, "exec-verify: actor verdict stamp failed",
				"attempt_id", f.attemptID, "error", err)
		}
		block := verdict == models.ActorVerdictMismatch
		blockReason := ""
		if block {
			blockReason = fmt.Sprintf("actor mismatch: expected fb_user_id %s, observed %s", expectedFB, actualFB)
		}
		if err := f.h.db.Coordination().RecordAccountActorVerdict(ctx, f.orgID, f.msg.AccountID, verdict, actualFB, blockReason, block); err != nil {
			slog.WarnContext(ctx, "exec-verify: actor verdict record failed",
				"org_id", f.orgID, "account_id", f.msg.AccountID, "error", err)
		}
		if block {
			// Operator alert: high-signal structured log + a problem event in the
			// account owner's chat. The block also surfaces as a dashboard chip and
			// is cleared via the admin clear-actor-block route.
			slog.ErrorContext(ctx, "exec-verify: ACTOR MISMATCH — account blocked from auto-execute",
				"org_id", f.orgID, "account_id", f.msg.AccountID, "outbound_id", f.id,
				"expected_fb_user_id", expectedFB, "actual_fb_user_id", actualFB)
			system.NotifyActorMismatch(f.h.db, f.orgID, f.msg.AccountID, f.id, expectedFB, actualFB)
		}
	}
}

// resolveActorFBIdentities returns the account's EXPECTED fb_user_id (or "" if the
// account is unreadable) and the OBSERVED fb_user_id from the proof's NavDiagnostic.
func (f *outboundFinalizer) resolveActorFBIdentities() (string, string) {
	expectedFB := ""
	if acc, aErr := f.h.db.Identities().GetAccountForOrg(f.msg.AccountID, f.orgID); aErr == nil && acc != nil {
		expectedFB = acc.FBUserID
	}
	actualFB := ""
	if f.proof.NavDiagnostic != nil {
		actualFB = f.proof.NavDiagnostic.FBUserID
	}
	return expectedFB, actualFB
}

// recordActionLedger updates the action_ledger row by outbound id with the
// outcome alias + reason. Best-effort (telemetry); extracted verbatim.
func (f *outboundFinalizer) recordActionLedger(ctx context.Context) {
	ledgerOutcome := models.LedgerOutcomeAlias(f.outcome)
	ledgerReason := string(f.outcome)
	if f.failureReason != "" && f.failureReason != string(f.outcome) {
		ledgerReason = string(f.outcome) + ":" + f.failureReason
	}
	if _, err := f.h.db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, f.orgID, f.id, ledgerOutcome, ledgerReason); err != nil {
		slog.WarnContext(ctx, "exec-verify: ledger outcome update failed",
			"org_id", f.orgID, "outbound_id", f.id, "error", err)
	}
}

// applyRiskSignal applies the outcome's risk signal to the account. Best-effort;
// extracted verbatim (same gate: a signal exists and the account is known).
func (f *outboundFinalizer) applyRiskSignal(ctx context.Context) {
	if sig := models.RiskSignalForOutcome(f.outcome); sig != "" && f.msg.AccountID > 0 {
		if err := f.h.db.Coordination().ApplyRiskSignal(ctx, f.orgID, f.msg.AccountID, sig, 0); err != nil {
			events.Warn(ctx, events.ExecutionHookFailed,
				events.FieldHook, "ApplyRiskSignal",
				events.FieldOrgID, f.orgID,
				events.FieldAccountID, f.msg.AccountID,
				"signal", sig,
				events.FieldErr, err,
			)
		}
	}
}

// refundQuotaForFailure (invariant 2026-06-01) refunds the daily counter slot a
// non-success terminal reserved at queue time. FIRST-WIN only (replay/stale
// returned earlier), so it runs once per terminal. Extracted verbatim.
func (f *outboundFinalizer) refundQuotaForFailure(ctx context.Context) {
	if !models.IsSuccessOutcome(f.outcome) && f.msg.AccountID > 0 {
		if err := f.h.db.Coordination().RefundDailyCounter(ctx, f.msg.AccountID, f.msg.Type); err != nil {
			slog.WarnContext(ctx, "exec-verify: quota refund failed",
				"org_id", f.orgID, "account_id", f.msg.AccountID, "action_type", f.msg.Type, "error", err)
		}
	}
}

// upsertInboxThreadForSuccess records inbox thread bookkeeping on an actual
// landing only. Failures are logged (not swallowed) — silent inbox data loss the
// operator would never see explained. Extracted verbatim.
func (f *outboundFinalizer) upsertInboxThreadForSuccess(ctx context.Context) {
	if models.IsSuccessOutcome(f.outcome) && f.msg.Type == "inbox" && f.msg.TargetURL != "" {
		threadID, threadErr := f.h.db.Threads().CreateThreadForOrg(f.orgID, 0, string(f.msg.Platform), f.msg.TargetURL, f.msg.TargetName, "")
		if threadErr != nil {
			slog.WarnContext(ctx, "exec-verify: inbox thread create failed",
				"org_id", f.orgID, "outbound_id", f.id, "target", f.msg.TargetURL, "error", threadErr)
		} else if err := f.h.db.Threads().AddThreadMessage(threadID, "outbound", f.msg.Content, true); err != nil {
			slog.WarnContext(ctx, "exec-verify: inbox thread message store failed",
				"org_id", f.orgID, "outbound_id", f.id, "thread_id", threadID, "error", err)
		}
	}
}
