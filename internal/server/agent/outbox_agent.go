package agent

import (
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
		if msg.AccountID <= 0 {
			continue
		}
		if assignedAccountID > 0 && msg.AccountID != assignedAccountID {
			continue
		}
		ownsStream, err := h.db.Connectors().ConnectorOwnsAccountStream(orgID, agentID, msg.AccountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ownsStream {
			continue
		}
		claim, err := h.db.ClaimPlannedOutboundForOrg(orgID, msg.ID, workerID, 0)
		if err != nil || claim == nil {
			continue
		}
		msg.ExecutionState = models.ExecExecuting
		msg.ExecutionID = claim.ExecutionID
		// Activity feed: execution_started — the autonomous-first
		// vocabulary makes "extension claimed and is about to mutate
		// the live DOM" a distinct event from "intent was queued"
		// (execution_planned) and from terminal events
		// (execution_verified / execution_failed).
		system.NotifyExecutionStarted(h.db, orgID, msg.AccountID, msg.ID, claim.ExecutionID, msg.Type)
		msgs = append(msgs, msg)
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

	msg, msgErr := h.db.GetOutboundForOrg(orgID, id)
	if msgErr != nil {
		return &finalizeResolution{
			HTTPStatus: 404,
			Body:       fiber.Map{"error": "outbound message not found"},
		}, nil
	}

	// Defense-in-depth identity check. EnforceTargetIdentity downgrades
	// success-class outcomes to ContextDrift if target_url/page_url_after
	// entity ids mismatch.
	outcome, proof = runtime.EnforceTargetIdentity(outcome, proof, msg.TargetURL, msg.Type)

	// Diagnostic instrumentation: emit a structured log line for every
	// non-success terminal so operators can see WHAT the extension did
	// without having to query execution_attempts.evidence_json. Captures
	// the proof.notes field which carries the landed_url + gate-fail
	// detail from the extension's lifecycle gates (see outbound.js patch
	// 3c17f1a). Precursor to EXP-1 typed events on the Runtime Control
	// Plane (see project_runtime_control_plane memory).
	if !models.IsSuccessOutcome(outcome) {
		// PR8A: when the extension shipped a NavDiagnostic, surface the
		// classified landing cause (redirect_class + stage + landed_url)
		// directly in the log line — that is the precise root cause the
		// investigation needs, not the generic outcome token.
		redirectClass, navStage, landedURL := "", "", ""
		if proof.NavDiagnostic != nil {
			redirectClass = proof.NavDiagnostic.RedirectClass
			navStage = proof.NavDiagnostic.Stage
			landedURL = proof.NavDiagnostic.LandedURL
		}
		slog.WarnContext(ctx, "exec-verify: non-success outcome",
			"org_id", orgID, "outbound_id", id,
			"account_id", msg.AccountID,
			"target_url", msg.TargetURL,
			"outcome", string(outcome),
			"failure_reason", report.FailureReason,
			"redirect_class", redirectClass,
			"nav_stage", navStage,
			"landed_url", landedURL,
			"page_url_after", proof.PageURLAfter,
			"notes", proof.Notes,
			"dom_snippet", proof.DOMSnippet,
		)
	}

	// PR-1 dual-column terminal pair: (state, verification_outcome).
	// TerminalFromOutcome is the single mapping from the rich
	// execution_attempts taxonomy onto the outbound row's two new
	// columns. ExecExpired only applies for ExecutionRetryExhausted
	// — every other terminal carries some observation.
	terminalState, terminalOutcome := models.TerminalFromOutcome(outcome)

	finalized, currentState, currentOutcome, currentExecID, err := h.db.FinalizeOutboundAttempt(ctx, orgID, id, report.ExecutionID, terminalState, terminalOutcome)
	if err != nil {
		return nil, err
	}
	if !finalized {
		// Disambiguate: is this a replay (same token, already terminal)
		// or a stale (token mismatch, row was re-claimed)?
		if report.ExecutionID != "" && currentExecID != "" && report.ExecutionID != currentExecID {
			// Stale: the row was lease-evicted and re-claimed; this
			// callback belongs to an execution that no longer owns
			// the row. Refuse loudly — 409 surfaces to the dashboard.
			slog.WarnContext(ctx, "exec-verify: stale execution_id",
				"org_id", orgID, "outbound_id", id,
				"submitted_execution_id", report.ExecutionID,
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
		// Idempotent replay: same execution_id, row already terminal.
		// Return success-shaped response WITHOUT replaying side effects.
		slog.InfoContext(ctx, "exec-verify: idempotent replay",
			"org_id", orgID, "outbound_id", id,
			"execution_id", report.ExecutionID,
			"current_state", currentState, "current_outcome", currentOutcome,
		)
		return &finalizeResolution{
			HTTPStatus: 200,
			Body: fiber.Map{
				"execution_state":      string(currentState),
				"verification_outcome": string(currentOutcome),
				"outcome":              string(outcome),
				"idempotent":           true,
			},
		}, nil
	}

	// FIRST-WIN PATH — commit side effects exactly once.
	attemptID, err := h.db.Coordination().BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID:      orgID,
		OutboundID: id,
		AccountID:  msg.AccountID,
		TargetURL:  msg.TargetURL,
		ActionType: msg.Type,
		Attempt:    1,
		Status:     models.AttemptVerifying,
	})
	if err != nil {
		slog.WarnContext(ctx, "exec-verify: begin attempt failed",
			"org_id", orgID, "outbound_id", id, "error", err)
		attemptID = 0
	} else {
		failureReason := ""
		if !models.IsSuccessOutcome(outcome) {
			failureReason = report.FailureReason
			if failureReason == "" {
				failureReason = string(outcome)
			}
		}
		// PR8A evidence pack: persist the failing-tab screenshot (out-of-band
		// base64 → org-scoped disk path). Done in the FIRST-WIN branch only so a
		// replay cannot rewrite the file, and BEFORE proofToEvidence so the path
		// rides into evidence_json. Non-success only — a verified send needs no
		// failure screenshot. Telemetry-only: failures are logged, not propagated.
		if !models.IsSuccessOutcome(outcome) && report.EvidenceScreenshotB64 != "" {
			if path, sErr := persistEvidenceScreenshot(orgID, id, attemptID, report.EvidenceScreenshotB64); sErr != nil {
				slog.WarnContext(ctx, "exec-verify: evidence screenshot persist failed",
					"org_id", orgID, "outbound_id", id, "attempt_id", attemptID, "error", sErr)
			} else if path != "" {
				if proof.NavDiagnostic == nil {
					proof.NavDiagnostic = &models.NavDiagnostic{}
				}
				proof.NavDiagnostic.ScreenshotPath = path
			}
		}
		if err := h.db.Coordination().FinishExecutionAttempt(ctx, attemptID, outcome, failureReason, proofToEvidence(proof)); err != nil {
			slog.WarnContext(ctx, "exec-verify: finish attempt failed",
				"attempt_id", attemptID, "outcome", outcome, "error", err)
		}

		// Verified Actor integrity gate (P1b — specs/COMMENT_INTELLIGENCE_PIPELINE.md §7b).
		// Compare the account's EXPECTED Facebook identity against the live c_user
		// the executor observed. The verdict is stamped APPEND-ONLY on this attempt
		// row + the account's runtime state; action_ledger is NOT mutated for it. A
		// definite mismatch BLOCKS the account from further auto-execute (the claim
		// cap-check denies it) until an operator clears the block. Best-effort —
		// failures here are logged, never abort finalize.
		//
		// TODO(p1b-atomicity): the verdict stamp runs AFTER FinishExecutionAttempt,
		// not in the same transaction. If the process crashes between the two, the
		// attempt is finalized but carries no actor_verdict (and no block is set).
		// Harden later by folding FinishExecutionAttempt + MarkAttemptActorVerification
		// + RecordAccountActorVerdict into ONE coordination finalize transaction.
		// Tracked in specs/COMMENT_INTELLIGENCE_PIPELINE.md §11.
		if attemptID > 0 && msg.AccountID > 0 {
			expectedFB := ""
			if acc, aErr := h.db.Identities().GetAccountForOrg(msg.AccountID, orgID); aErr == nil && acc != nil {
				expectedFB = acc.FBUserID
			}
			actualFB := ""
			if proof.NavDiagnostic != nil {
				actualFB = proof.NavDiagnostic.FBUserID
			}
			verdict := coordination.ClassifyActorVerdict(expectedFB, actualFB)
			if err := h.db.Coordination().MarkAttemptActorVerification(ctx, attemptID, expectedFB, actualFB, verdict); err != nil {
				slog.WarnContext(ctx, "exec-verify: actor verdict stamp failed",
					"attempt_id", attemptID, "error", err)
			}
			block := verdict == models.ActorVerdictMismatch
			blockReason := ""
			if block {
				blockReason = fmt.Sprintf("actor mismatch: expected fb_user_id %s, observed %s", expectedFB, actualFB)
			}
			if err := h.db.Coordination().RecordAccountActorVerdict(ctx, orgID, msg.AccountID, verdict, actualFB, blockReason, block); err != nil {
				slog.WarnContext(ctx, "exec-verify: actor verdict record failed",
					"org_id", orgID, "account_id", msg.AccountID, "error", err)
			}
			if block {
				// Operator alert: high-signal structured log + a problem event in
				// the account owner's chat. The block also surfaces as a chip in
				// the dashboard and is cleared via the admin clear-actor-block route.
				slog.ErrorContext(ctx, "exec-verify: ACTOR MISMATCH — account blocked from auto-execute",
					"org_id", orgID, "account_id", msg.AccountID, "outbound_id", id,
					"expected_fb_user_id", expectedFB, "actual_fb_user_id", actualFB)
				system.NotifyActorMismatch(h.db, orgID, msg.AccountID, id, expectedFB, actualFB)
			}
		}

		ledgerOutcome := models.LedgerOutcomeAlias(outcome)
		ledgerReason := string(outcome)
		if failureReason != "" && failureReason != string(outcome) {
			ledgerReason = string(outcome) + ":" + failureReason
		}
		if _, err := h.db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, orgID, id, ledgerOutcome, ledgerReason); err != nil {
			slog.WarnContext(ctx, "exec-verify: ledger outcome update failed",
				"org_id", orgID, "outbound_id", id, "error", err)
		}

		if sig := models.RiskSignalForOutcome(outcome); sig != "" && msg.AccountID > 0 {
			if err := h.db.Coordination().ApplyRiskSignal(ctx, orgID, msg.AccountID, sig, 0); err != nil {
				events.Warn(ctx, events.ExecutionHookFailed,
					events.FieldHook, "ApplyRiskSignal",
					events.FieldOrgID, orgID,
					events.FieldAccountID, msg.AccountID,
					"signal", sig,
					events.FieldErr, err,
				)
			}
		}
		events.Info(ctx, events.ExecutionVerified,
			events.FieldOutboundID, id,
			events.FieldAttemptID, attemptID,
			events.FieldOutcome, outcome,
			events.FieldAccountID, msg.AccountID,
			events.FieldActionType, msg.Type,
		)
	}

	// QUOTA REFUND (invariant 2026-06-01): the *_today counter reserved a slot
	// at QUEUE time (IncrementCounterTx). A non-success terminal posted nothing,
	// so refund the slot — failed execution must not consume the business quota
	// (it stays retryable). FIRST-WIN ONLY: the !finalized replay/stale paths
	// returned above, so this runs once per terminal and cannot double-refund
	// (RefundCounterTx's >0 guard is the belt-and-suspenders). Pure quota
	// accounting — risk/pacing already moved via ApplyRiskSignal and is
	// untouched here (audit: comments_today is not coupled to risk_score).
	if !models.IsSuccessOutcome(outcome) && msg.AccountID > 0 {
		if err := h.db.Coordination().RefundDailyCounter(ctx, msg.AccountID, msg.Type); err != nil {
			slog.WarnContext(ctx, "exec-verify: quota refund failed",
				"org_id", orgID, "account_id", msg.AccountID, "action_type", msg.Type, "error", err)
		}
	}

	// Inbox-specific thread bookkeeping — only on actual landing. Failures are
	// logged (not swallowed): a dropped thread/message is silent inbox data
	// loss the operator would otherwise never see explained.
	if models.IsSuccessOutcome(outcome) && msg.Type == "inbox" && msg.TargetURL != "" {
		threadID, threadErr := h.db.Threads().CreateThreadForOrg(orgID, 0, string(msg.Platform), msg.TargetURL, msg.TargetName, "")
		if threadErr != nil {
			slog.WarnContext(ctx, "exec-verify: inbox thread create failed",
				"org_id", orgID, "outbound_id", id, "target", msg.TargetURL, "error", threadErr)
		} else if err := h.db.Threads().AddThreadMessage(threadID, "outbound", msg.Content, true); err != nil {
			slog.WarnContext(ctx, "exec-verify: inbox thread message store failed",
				"org_id", orgID, "outbound_id", id, "thread_id", threadID, "error", err)
		}
	}

	// PR-2 V2: notification now consumes the (state, outcome) pair
	// directly — no legacy OutboundStatus translation. The single-source-
	// of-truth predicate IsVerifiedSuccess gates the verified/failed event
	// fork inside NotifyOutboundStatusDetail.
	if models.IsVerifiedSuccess(terminalState, terminalOutcome) {
		system.NotifyOutboundStatus(h.db, h.notifier, orgID, id, terminalState, terminalOutcome)
	} else {
		// Surface the extension's GRANULAR diagnostic note directly in the
		// operator's chat (see notificationDetail for the overwrite-free
		// data-path contract).
		system.NotifyOutboundStatusDetail(h.db, h.notifier, orgID, id, terminalState, terminalOutcome, notificationDetail(proof, report, outcome))
	}

	return &finalizeResolution{
		HTTPStatus: 200,
		Body: fiber.Map{
			"execution_state":      string(terminalState),
			"verification_outcome": string(outcome),
			"attempt_id":           attemptID,
		},
	}, nil
}
