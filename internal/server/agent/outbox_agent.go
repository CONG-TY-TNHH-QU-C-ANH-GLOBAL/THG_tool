package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/telegram/control"
)

// maxEvidenceScreenshotBytes bounds a decoded evidence screenshot before it
// touches disk. A q40 JPEG of a 1080p tab is ~80–160 KB; 1 MB is a generous
// ceiling that still rejects an extension shipping a runaway payload.
const maxEvidenceScreenshotBytes = 1 << 20 // 1 MiB

// persistEvidenceScreenshot decodes the extension's out-of-band base64 JPEG to
// an ORG-SCOPED file under data/evidence/<orgID>/ and returns the relative path
// to record in NavDiagnostic.ScreenshotPath. The bytes never enter evidence_json.
//
// Tenant safety: every path component is server-derived (orgID, outboundID,
// attemptID are internal ids issued in org-scoped txs) — no extension-supplied
// string reaches the filename, so this cannot be steered out of the org dir.
// Best-effort: any failure returns "" with a logged warning; evidence capture
// must never block the terminal callback.
func persistEvidenceScreenshot(orgID, outboundID, attemptID int64, b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	// Accept a data: URL prefix or raw base64.
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+1:]
	}
	if b64 == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decode evidence screenshot: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxEvidenceScreenshotBytes {
		return "", fmt.Errorf("evidence screenshot size out of bounds: %d bytes", len(raw))
	}
	dir := filepath.Join("data", "evidence", strconv.FormatInt(orgID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir evidence dir: %w", err)
	}
	name := fmt.Sprintf("ob%d-att%d-%d.jpg", outboundID, attemptID, time.Now().UTC().Unix())
	rel := filepath.ToSlash(filepath.Join(dir, name))
	if err := os.WriteFile(filepath.FromSlash(rel), raw, 0o644); err != nil {
		return "", fmt.Errorf("write evidence screenshot: %w", err)
	}
	return rel, nil
}

// notificationDetail selects the most diagnostic string to surface in the
// operator's failure notification (the `Chi tiet:` line in chat/Telegram).
//
// proof.Notes carries the extension's GRANULAR path/gate prefix
// (path2.article_not_found_in_feed, path2.group_home_nav_failed,
// outbox.crawler_nav_failed, …) plus landed_url — enough to bucket a failure
// from a single run. We prefer it so the failure is self-explanatory in chat
// without opening the diagnostic endpoint. The data path that feeds it is
// overwrite-free: ClassifyExtensionReport copies report.Notes verbatim (it
// only fills Notes on success-WITHOUT-proof, never on a failure), and
// EnforceTargetIdentity only APPENDS (" · entity-drift …"), never replaces.
//
// report.FailureReason then string(outcome) are last-resort fallbacks only —
// coarse labels that cannot distinguish path from gate (redirected_feed vs
// context_drift), so they are used solely when no note survived.
func notificationDetail(proof runtime.VerifierProof, report runtime.ExtensionExecutionReport, outcome models.ExecutionOutcome) string {
	if d := strings.TrimSpace(proof.Notes); d != "" {
		return d
	}
	if d := strings.TrimSpace(report.FailureReason); d != "" {
		return d
	}
	return string(outcome)
}

// proofToEvidence adapts the runtime verifier's proof shape onto the
// coordination domain's evidence shape. Two types exist (instead of
// one shared) to avoid an import cycle: runtime cannot import store,
// and coordination cannot import runtime. The fields are 1:1 today;
// if they diverge, this is the seam to translate.
func proofToEvidence(p runtime.VerifierProof) coordination.VerificationEvidence {
	return coordination.VerificationEvidence{
		CommentPermalink: p.CommentPermalink,
		MessageBubbleID:  p.MessageBubbleID,
		DOMSnippet:       p.DOMSnippet,
		PageURLAfter:     p.PageURLAfter,
		ObservedAt:       p.ObservedAt,
		Notes:            p.Notes,
		NavDiagnostic:    p.NavDiagnostic, // PR8A: structured landing telemetry → evidence_json
	}
}

// agentGetOutbox returns approved outbound messages for local execution.
// GET /api/agent/outbox
//
// Each successful claim issues a fresh execution_id and stamps a
// per-row lease_expiry (see store.DefaultOutboundLease). Both fields
// flow back in the response so the executor can echo execution_id
// on its /sent or /failed callback — that token gates the terminal
// CAS in store.FinalizeOutboundAttempt and is what prevents
// duplicate-comment when the extension's service worker restarts
// mid-execution or a flaky network triggers a retry.
func (h *Handler) agentGetOutbox(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	agentID, _ := c.Locals("agent_id").(int64)
	assignedAccountID, _ := c.Locals("agent_assigned_account_id").(int64)
	workerID, _ := c.Locals("agent_token_fp").(string)
	if orgID <= 0 || agentID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	// 10-min fallback only applies to legacy rows (lease_expiry IS
	// NULL). New claims get a per-row lease so this global window is
	// no longer the primary stale-detection knob.
	_ = h.db.ResetStaleExecutingForOrg(orgID, 10*time.Minute)
	candidates, err := h.db.GetOutboundByExecutionStateForOrg(orgID, models.ExecPlanned, "", limit*4)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	msgs := make([]models.OutboundMessage, 0, limit)
	for _, msg := range candidates {
		if len(msgs) >= limit {
			break
		}
		claimed, ok, err := h.claimCandidate(orgID, agentID, assignedAccountID, workerID, msg)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ok {
			continue
		}
		msgs = append(msgs, claimed)
	}
	return c.JSON(fiber.Map{"messages": msgs, "count": len(msgs)})
}

// agentOutboxSent records a verified send attempt.
// POST /api/agent/outbox/:id/sent
//
// Step 3 — Execution Verification. Legacy contract: hitting this endpoint
// asserts the extension thinks the action succeeded. New contract: the
// body MAY include an ExtensionExecutionReport with DOM proof
// (CommentPermalink / MessageBubbleID / DOMSnippet / PageURLAfter /
// CountIncreased / NodeMatched / BubbleFresh / Duplicate / Notes). The
// server classifies the outcome using the same taxonomy a server-side
// chromedp verifier would emit, writes an execution_attempts row, updates
// the action_ledger by outbound_id, and applies the corresponding risk
// signal. Extensions that POST with no body still mark the outbound as
// sent (OptimisticSuccess) — backward-compatible.
func (h *Handler) agentOutboxSent(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Legacy contract default: hitting /sent asserts success. The body
	// supplements with DOM evidence when present.
	report := runtime.ExtensionExecutionReport{Success: true}
	_ = c.BodyParser(&report)

	outcome, proof := runtime.ClassifyExtensionReport(report)
	// PR-1: terminal pair (state, outcome) instead of a single status.
	// The verifier's classification supersedes the endpoint name —
	// even though /sent fires, a non-success outcome lands the row in
	// finished/<non-verified> per TerminalFromOutcome.
	resolution, err := h.finalizeOutbound(c, orgID, id, report, outcome, proof)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return resolution.write(c)
}

// agentOutboxFailed records a failed send attempt.
// POST /api/agent/outbox/:id/failed
//
// Step 3 — Execution Verification. Legacy contract: hitting this endpoint
// asserts the extension failed to send. New contract: the body MAY include
// an ExtensionExecutionReport whose FailureReason maps to a specific
// outcome (captcha, rate_limited, blocked, redirected_feed, …). Without a
// body the classifier returns ExecutionShadowRejected — the safe default
// that flags the row as a real failure rather than letting it silently
// claim success.
func (h *Handler) agentOutboxFailed(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Default failure assertion; body may supply richer FailureReason.
	report := runtime.ExtensionExecutionReport{Success: false}
	_ = c.BodyParser(&report)

	outcome, proof := runtime.ClassifyExtensionReport(report)
	// Same execution_id-gated CAS pathway as agentOutboxSent.
	resolution, err := h.finalizeOutbound(c, orgID, id, report, outcome, proof)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return resolution.write(c)
}

// finalizeResolution is the HTTP-shaped result of a /sent or /failed
// callback. The handler builds one of these and writes it through.
// Centralising the shape here keeps the two handlers symmetric and
// makes the three terminal pathways (committed / idempotent replay /
// stale execution_id) easy to audit.
type finalizeResolution struct {
	HTTPStatus int
	Body       fiber.Map
}

func (f *finalizeResolution) write(c *fiber.Ctx) error {
	return c.Status(f.HTTPStatus).JSON(f.Body)
}

// finalizeOutbound is the single write-point for terminal outbound
// callbacks. It encodes three invariants:
//
//  1. EXECUTION IDENTITY (post-hoc defense for wrong-post bug class):
//     EnforceTargetIdentity downgrades success-class outcomes to
//     ContextDrift when the extension's reported page_url_after does
//     not address the same Facebook entity as the queued target_url.
//
//  2. TERMINAL CAS (lease + execution_id idempotency):
//     FinalizeOutboundAttempt requires (status='sending', execution_id
//     matches OR row's execution_id is empty for legacy rows). A
//     replayed callback hitting an already-terminal row returns
//     idempotent-OK; a callback whose execution_id no longer matches
//     the row (because ResetStaleExecutingForOrg lease-evicted
//     it and a new claim issued a fresh token) returns 409 stale.
//
//  3. SIDE EFFECTS ARE COMMITTED ONLY ON FIRST-WIN:
//     execution_attempts row, action_ledger update, and risk-signal
//     application happen INSIDE the finalized==true branch only.
//     Replays do NOT replay them — that was the whole point of
//     introducing execution_id. The previous architecture wrote side
//     effects unconditionally on every /sent or /failed hit and so
//     would have multiplied evidence rows on SW-restart-triggered
//     duplicate callbacks.
//
// Errors from the side-effect writes are logged but never propagated
// — they are verification telemetry, not the load-bearing path.
func (h *Handler) finalizeOutbound(
	c *fiber.Ctx,
	orgID, id int64,
	report runtime.ExtensionExecutionReport,
	outcome models.ExecutionOutcome,
	proof runtime.VerifierProof,
) (*finalizeResolution, error) {
	ctx := c.UserContext()
	f := &outboundFinalizer{
		h:       h,
		orgID:   orgID,
		id:      id,
		report:  report,
		outcome: outcome,
		proof:   proof,
	}
	if res := f.loadOutbound(); res != nil {
		return res, nil
	}
	f.enforceTargetIdentity(ctx)
	if res, err := f.attemptFinalization(ctx); res != nil || err != nil {
		return res, err
	}
	// FIRST-WIN PATH — commit side effects exactly once (the !finalized
	// replay/stale callbacks returned above, so none of the below can run twice).
	f.applyFirstWinSideEffects(ctx)
	f.refundQuotaForFailure(ctx)
	f.upsertInboxThreadForSuccess(ctx)
	f.notifyOutboundFinalized()
	return f.buildResponse(), nil
}

// outboundFinalizer carries the per-callback state for one terminal /sent or
// /failed finalization. It is created and used once per finalizeOutbound call and
// never reused/stored; mutating methods use a pointer receiver (notably proof is
// mutated by persistFailureEvidence before it rides into evidence/notification).
// The request context is NOT stored on the struct — it is passed explicitly to
// the methods that perform store/runtime operations, as the first parameter.
type outboundFinalizer struct {
	h               *Handler
	orgID           int64
	id              int64
	report          runtime.ExtensionExecutionReport
	outcome         models.ExecutionOutcome
	proof           runtime.VerifierProof
	msg             *models.OutboundMessage
	terminalState   models.ExecutionState
	terminalOutcome models.VerificationOutcome
	attemptID       int64
	failureReason   string
}

// loadOutbound fetches the outbound row. A missing row yields the 404 resolution
// the caller returns verbatim; otherwise it caches the row and returns nil.
func (f *outboundFinalizer) loadOutbound() *finalizeResolution {
	msg, msgErr := f.h.db.GetOutboundForOrg(f.orgID, f.id)
	if msgErr != nil {
		return &finalizeResolution{
			HTTPStatus: 404,
			Body:       fiber.Map{"error": "outbound message not found"},
		}
	}
	f.msg = msg
	return nil
}

// enforceTargetIdentity applies the defense-in-depth identity downgrade
// (success-class → ContextDrift on target_url/page_url_after entity mismatch) and
// emits the non-success diagnostic log line (precise landing cause via NavDiagnostic).
func (f *outboundFinalizer) enforceTargetIdentity(ctx context.Context) {
	f.outcome, f.proof = runtime.EnforceTargetIdentity(f.outcome, f.proof, f.msg.TargetURL, f.msg.Type)
	if !models.IsSuccessOutcome(f.outcome) {
		redirectClass, navStage, landedURL := "", "", ""
		if f.proof.NavDiagnostic != nil {
			redirectClass = f.proof.NavDiagnostic.RedirectClass
			navStage = f.proof.NavDiagnostic.Stage
			landedURL = f.proof.NavDiagnostic.LandedURL
		}
		slog.WarnContext(ctx, "exec-verify: non-success outcome",
			"org_id", f.orgID, "outbound_id", f.id,
			"account_id", f.msg.AccountID,
			"target_url", f.msg.TargetURL,
			"outcome", string(f.outcome),
			"failure_reason", f.report.FailureReason,
			"redirect_class", redirectClass,
			"nav_stage", navStage,
			"landed_url", landedURL,
			"page_url_after", f.proof.PageURLAfter,
			"notes", f.proof.Notes,
			"dom_snippet", f.proof.DOMSnippet,
		)
	}
}

// attemptFinalization computes the terminal (state, verification_outcome) pair and
// runs the execution_id-gated CAS. It returns a non-nil resolution for the two
// non-first-win terminals (stale 409 / idempotent replay 200), a non-nil error for
// a CAS failure, or (nil, nil) when THIS callback won the terminal transition.
func (f *outboundFinalizer) attemptFinalization(ctx context.Context) (*finalizeResolution, error) {
	f.terminalState, f.terminalOutcome = models.TerminalFromOutcome(f.outcome)

	finalized, currentState, currentOutcome, currentExecID, err := f.h.db.FinalizeOutboundAttempt(ctx, f.orgID, f.id, f.report.ExecutionID, f.terminalState, f.terminalOutcome)
	if err != nil {
		return nil, err
	}
	if finalized {
		return nil, nil
	}
	// Disambiguate: replay (same token, already terminal) vs stale (token
	// mismatch, row was re-claimed).
	if f.report.ExecutionID != "" && currentExecID != "" && f.report.ExecutionID != currentExecID {
		// Stale: the row was lease-evicted and re-claimed; this callback belongs
		// to an execution that no longer owns the row. Refuse loudly — 409.
		slog.WarnContext(ctx, "exec-verify: stale execution_id",
			"org_id", f.orgID, "outbound_id", f.id,
			"submitted_execution_id", f.report.ExecutionID,
			"current_execution_id", currentExecID,
			"current_state", currentState,
			"current_outcome", currentOutcome,
		)
		return &finalizeResolution{
			HTTPStatus: 409,
			Body: fiber.Map{
				"error":                "stale execution_id",
				"current_state":        string(currentState),
				"current_outcome":      string(currentOutcome),
				"current_execution_id": currentExecID,
			},
		}, nil
	}
	// Idempotent replay: same execution_id, row already terminal. Return
	// success-shaped response WITHOUT replaying side effects.
	slog.InfoContext(ctx, "exec-verify: idempotent replay",
		"org_id", f.orgID, "outbound_id", f.id,
		"execution_id", f.report.ExecutionID,
		"current_state", currentState, "current_outcome", currentOutcome,
	)
	return &finalizeResolution{
		HTTPStatus: 200,
		Body: fiber.Map{
			"execution_state":      string(currentState),
			"verification_outcome": string(currentOutcome),
			"outcome":              string(f.outcome),
			"idempotent":           true,
		},
	}, nil
}

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

// notifyOutboundFinalized emits the global-admin notifier event (verified vs
// detailed-failure fork) and the per-org Telegram channel event (when tgEvents is
// configured). Best-effort; extracted verbatim.
func (f *outboundFinalizer) notifyOutboundFinalized() {
	if models.IsVerifiedSuccess(f.terminalState, f.terminalOutcome) {
		system.NotifyOutboundStatus(f.h.db, f.h.notifier, f.orgID, f.id, f.terminalState, f.terminalOutcome)
	} else {
		// Surface the extension's GRANULAR diagnostic note directly in the
		// operator's chat (see notificationDetail for the overwrite-free contract).
		system.NotifyOutboundStatusDetail(f.h.db, f.h.notifier, f.orgID, f.id, f.terminalState, f.terminalOutcome, notificationDetail(f.proof, f.report, f.outcome))
	}

	// Per-ORG Telegram CHANNEL notification (distinct from the global-admin notifier
	// above): uses the org's own bot + channel destinations. Best-effort.
	if f.h.tgEvents != nil {
		verified := models.IsVerifiedSuccess(f.terminalState, f.terminalOutcome)
		if ev := agentEventType(f.msg.Type, verified, models.IsSuccessOutcome(f.outcome)); ev != "" {
			channel := strings.ToLower(string(f.msg.Platform))
			if channel == "" {
				channel = "facebook"
			}
			f.h.tgEvents.NotifyAction(control.ActionNotice{
				OrgID: f.orgID, OutboundID: f.id, EventType: ev, Channel: channel,
				Workspace:   f.h.orgName(f.orgID),
				Agent:       f.h.agentName(f.msg.AccountID),
				Author:      f.msg.TargetName,
				CommentText: f.msg.Content,
				PostURL:     f.msg.TargetURL,
				Reason:      failureReasonText(f.report, string(f.outcome)),
				BaseURL:     f.h.baseURL,
			})
		}
	}
}

// buildResponse is the FIRST-WIN terminal response: the (state, outcome) pair the
// dashboard consumes, plus the attempt id (0 when BeginExecutionAttempt failed).
func (f *outboundFinalizer) buildResponse() *finalizeResolution {
	return &finalizeResolution{
		HTTPStatus: 200,
		Body: fiber.Map{
			"execution_state":      string(f.terminalState),
			"verification_outcome": string(f.outcome),
			"attempt_id":           f.attemptID,
		},
	}
}

// agentEventType maps an outbound action type + outcome to a Telegram destination event key.
// "" means "no notification for this action type". comment submitted-but-not-verified is
// comment_unverified (pending manual verification).
func agentEventType(actionType string, verified, success bool) string {
	switch actionType {
	case "comment":
		if verified {
			return "comment_verified"
		}
		if success {
			return "comment_unverified"
		}
		return "comment_failed"
	case "inbox":
		if success {
			return "inbox_sent"
		}
		return "inbox_failed"
	case "group_post", "post":
		if success {
			return "post_submitted"
		}
		return "post_failed"
	}
	return ""
}
