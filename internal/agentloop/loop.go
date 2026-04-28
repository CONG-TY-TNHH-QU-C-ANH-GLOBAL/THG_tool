package agentloop

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// MaxIterations is the re-think loop ceiling.
// Invariant ABORTED: exceed this → state = ABORTED, do not retry.
const MaxIterations = 5

// MinPlannerConfidence: below this the loop escalates to HUMAN_REQUIRED.
const MinPlannerConfidence = 0.50

// MinArchitectConfidence: below this the loop escalates to HUMAN_REQUIRED.
const MinArchitectConfidence = 0.60

// AgentLoop is the main orchestrator.
// It runs: Planner → Architect → Executor → Verifier → (re-think if fail).
//
// Agent State Machine:
//
//	IDLE → PLANNING → PATCHING → VERIFYING
//	     → SUCCESS | FAILED | ABORTED | POISON | HUMAN_REQUIRED
//
// Boundary: this agent fixes the SYSTEM only.
// It does NOT: crawl Facebook, manage browser sessions, or implement business logic.
type AgentLoop struct {
	planner   *Planner
	architect *Architect
	executor  *Executor
	verifier  *Verifier
	ledger    *ActionLedger
}

// New creates a fully wired AgentLoop.
func New(apiKey, plannerModel, architectModel, baseDir string, verifyCfg VerifyConfig) *AgentLoop {
	return &AgentLoop{
		planner:   NewPlanner(apiKey, plannerModel),
		architect: NewArchitect(apiKey, architectModel),
		executor:  NewExecutor(baseDir),
		verifier:  NewVerifier(verifyCfg),
		ledger:    newActionLedger(),
	}
}

// Run executes the full agent loop for the given task.
// It is safe to call from multiple goroutines (each call gets its own ledger/trace).
func (a *AgentLoop) Run(ctx context.Context, task Task) RunResult {
	traceID := uuid.New().String()[:8]
	trace := newDecisionTrace(traceID)
	state := StateIdle

	result := RunResult{TraceID: traceID}
	defer func() {
		result.Trace = trace.Entries()
		result.State = state
	}()

	slog.InfoContext(ctx, "agent loop started", "trace_id", traceID, "task", task.Description)

	// ── Phase 1: Planner ───────────────────────────────────────────────────────
	state = StatePlanning
	planStart := time.Now()
	plan, err := a.planner.Plan(ctx, task)
	planMs := time.Since(planStart).Milliseconds()

	if err != nil {
		trace.Record(0, "planner", "classify task", err.Error(), "failed", 0, planMs)
		result.Reason = "planner error: " + err.Error()
		state = StateFailed
		return result
	}

	trace.Record(0, "planner",
		fmt.Sprintf("domain=%s intent=%s", plan.Domain, plan.Intent),
		plan.RootCause, "ok", plan.Confidence, planMs)

	// Escalate if planner is uncertain.
	if plan.Confidence < MinPlannerConfidence || plan.Domain == DomainUnknown {
		trace.Record(0, "planner", "confidence too low", plan.Intent, "human", plan.Confidence, 0)
		result.Reason = fmt.Sprintf("planner confidence %.2f < %.2f — escalating to human", plan.Confidence, MinPlannerConfidence)
		state = StateHumanRequired
		return result
	}

	// ── Re-think Loop ──────────────────────────────────────────────────────────
	var prevFailure string

	for iter := 0; iter < MaxIterations; iter++ {
		result.Iterations = iter + 1
		slog.InfoContext(ctx, "agent iteration", "trace_id", traceID, "iter", iter, "domain", plan.Domain)

		// Context cancellation check at loop top.
		select {
		case <-ctx.Done():
			result.Reason = "context cancelled"
			state = StateAborted
			return result
		default:
		}

		// ── Phase 2: Architect ─────────────────────────────────────────────────
		state = StatePlanning // still in planning phase until patches are ready
		archStart := time.Now()
		design, err := a.architect.Design(ctx, plan, task.AvailableFiles, prevFailure)
		archMs := time.Since(archStart).Milliseconds()

		if err != nil {
			trace.Record(iter, "architect", "generate patch plan", err.Error(), "failed", 0, archMs)
			prevFailure = "architect call failed: " + err.Error()
			continue
		}

		trace.Record(iter, "architect",
			fmt.Sprintf("%d patches risk=%s", len(design.Patches), design.Risk),
			design.Rationale, "ok", design.Confidence, archMs)

		// Escalate if architect is uncertain.
		if design.Confidence < MinArchitectConfidence {
			trace.Record(iter, "architect", "confidence too low", design.Rationale, "human", design.Confidence, 0)
			result.Reason = fmt.Sprintf("architect confidence %.2f < %.2f — escalating to human", design.Confidence, MinArchitectConfidence)
			state = StateHumanRequired
			return result
		}

		if len(design.Patches) == 0 {
			trace.Record(iter, "architect", "no patches generated", "nothing to do", "ok", design.Confidence, 0)
			// No patches = architect thinks it's already fixed; run verifier to confirm.
		}

		// ── Phase 3: Executor ──────────────────────────────────────────────────
		state = StatePatching
		execErr := ""

		for _, patch := range design.Patches {
			hash := PatchHash(patch)

			if a.ledger.IsPoisoned(hash) {
				trace.Record(iter, "executor", "skip poison patch "+patch.File, patch.Why, "poison", 0, 0)
				state = StatePoison
				result.Reason = fmt.Sprintf("patch for %s is poisoned (failed %d× before)", patch.File, PoisonThreshold)
				return result
			}

			if a.ledger.IsApplied(hash) {
				trace.Record(iter, "executor", "skip already-applied patch "+patch.File, patch.Why, "skipped", 0, 0)
				continue
			}

			applyStart := time.Now()
			err := a.executor.Apply(patch)
			applyMs := time.Since(applyStart).Milliseconds()

			if err != nil {
				poisoned := a.ledger.RecordFailed(hash, patch.File)
				trace.Record(iter, "executor", "apply patch "+patch.File, err.Error(), "failed", 0, applyMs)

				if poisoned {
					state = StatePoison
					result.Reason = fmt.Sprintf("patch for %s marked as poison: %v", patch.File, err)
					return result
				}
				execErr = fmt.Sprintf("patch %s failed: %v", patch.File, err)
				break
			}

			a.ledger.RecordApplied(hash, patch.File)
			trace.Record(iter, "executor", "applied patch "+patch.File, patch.Why, "ok", 0, applyMs)
		}

		if execErr != "" {
			prevFailure = execErr
			continue // re-think
		}

		// ── Phase 4: Verifier ──────────────────────────────────────────────────
		state = StateVerifying
		verifyStart := time.Now()
		vr := a.verifier.Verify(ctx, plan.Domain)
		verifyMs := time.Since(verifyStart).Milliseconds()

		result.VerifyScore = vr.Score
		trace.Record(iter, "verifier",
			fmt.Sprintf("score=%.2f pass=%v", vr.Score, vr.Pass),
			vr.Reason, boolResult(vr.Pass), vr.Score, verifyMs)

		if vr.Pass {
			state = StateSuccess
			result.Reason = fmt.Sprintf("goal achieved — verify score %.2f", vr.Score)
			slog.InfoContext(ctx, "agent loop success", "trace_id", traceID, "score", vr.Score, "iters", iter+1)
			return result
		}

		// Verification failed — build context for architect rethink.
		prevFailure = fmt.Sprintf("verification failed (score=%.2f): %s", vr.Score, vr.Reason)
	}

	// Exhausted all iterations.
	state = StateAborted
	result.Reason = fmt.Sprintf("max iterations (%d) reached without success — last failure: %s", MaxIterations, prevFailure)
	slog.WarnContext(ctx, "agent loop aborted", "trace_id", traceID, "reason", result.Reason)
	return result
}

func boolResult(b bool) string {
	if b {
		return "ok"
	}
	return "failed"
}
