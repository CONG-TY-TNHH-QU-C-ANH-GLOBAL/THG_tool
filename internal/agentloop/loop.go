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
//
// Full pipeline per iteration:
//
//	Planner (LLM) → Architect (LLM) → SandboxValidator → Verifier → Promote|Rollback
//
// Safe-deployment contract:
//
//	Patches are NEVER permanently applied until:
//	  1. go build ./... passes (sandbox validation)
//	  2. Verifier score ≥ VerifyPassThreshold (goal-based success)
//	  3. Verifier stability loop passes (T+0, T+2s, T+4s)
//
// On any failure after patches are applied → automatic rollback via Snapshot.
//
// Boundary: this agent fixes the SYSTEM only (infra, browser, frontend, jobs).
// It does NOT: crawl Facebook, manage session state, or implement business logic.
type AgentLoop struct {
	planner   *Planner
	architect *Architect
	executor  *Executor
	verifier  *Verifier
	ledger    *ActionLedger
	baseDir   string
}

// New creates a fully wired AgentLoop.
func New(apiKey, plannerModel, architectModel, baseDir string, verifyCfg VerifyConfig) *AgentLoop {
	return &AgentLoop{
		planner:   NewPlanner(apiKey, plannerModel),
		architect: NewArchitect(apiKey, architectModel),
		executor:  NewExecutor(baseDir),
		verifier:  NewVerifier(verifyCfg),
		ledger:    newActionLedger(),
		baseDir:   baseDir,
	}
}

// Run executes the full agent loop for the given task.
// Safe to call concurrently — each invocation gets its own ledger, trace, and sandbox.
func (a *AgentLoop) Run(ctx context.Context, task Task) RunResult {
	traceID := uuid.New().String()[:8]
	trace := newDecisionTrace(traceID)
	state := StateIdle
	var planDomain Domain = DomainUnknown

	result := RunResult{TraceID: traceID}
	defer func() {
		result.Trace = trace.Entries()
		result.State = state
		metricRun(state, planDomain)
		metricIterations(result.Iterations)
	}()

	slog.InfoContext(ctx, "agent loop started", "trace_id", traceID, "task", task.Description)

	// ── Phase 1: Planner ───────────────────────────────────────────────────────
	state = StatePlanning
	t0 := time.Now()
	plan, err := a.planner.Plan(ctx, task)
	planMs := time.Since(t0).Milliseconds()
	metricStep("planner", time.Since(t0).Seconds())

	if err != nil {
		trace.Record(0, "planner", "classify task", err.Error(), "failed", 0, planMs)
		result.Reason = "planner error: " + err.Error()
		state = StateFailed
		return result
	}
	planDomain = plan.Domain

	trace.Record(0, "planner",
		fmt.Sprintf("domain=%s intent=%s", plan.Domain, plan.Intent),
		plan.RootCause, "ok", plan.Confidence, planMs)

	if plan.Confidence < MinPlannerConfidence || plan.Domain == DomainUnknown {
		trace.Record(0, "planner", "confidence too low", plan.Intent, "human", plan.Confidence, 0)
		result.Reason = fmt.Sprintf("planner confidence %.2f < %.2f — escalating to human", plan.Confidence, MinPlannerConfidence)
		metricHumanEscalation("planner")
		state = StateHumanRequired
		return result
	}

	// ── Re-think Loop ──────────────────────────────────────────────────────────
	var prevFailure string

	for iter := range MaxIterations {
		result.Iterations = iter + 1
		slog.InfoContext(ctx, "agent iteration",
			"trace_id", traceID, "iter", iter, "domain", plan.Domain)

		select {
		case <-ctx.Done():
			result.Reason = "context cancelled"
			state = StateAborted
			return result
		default:
		}

		// ── Phase 2: Architect ─────────────────────────────────────────────────
		state = StatePlanning
		t1 := time.Now()
		design, err := a.architect.Design(ctx, plan, task.AvailableFiles, prevFailure)
		archMs := time.Since(t1).Milliseconds()
		metricStep("architect", time.Since(t1).Seconds())

		if err != nil {
			trace.Record(iter, "architect", "generate patch plan", err.Error(), "failed", 0, archMs)
			prevFailure = "architect call failed: " + err.Error()
			continue
		}

		trace.Record(iter, "architect",
			fmt.Sprintf("%d patches risk=%s", len(design.Patches), design.Risk),
			design.Rationale, "ok", design.Confidence, archMs)

		if design.Confidence < MinArchitectConfidence {
			trace.Record(iter, "architect", "confidence too low", design.Rationale, "human", design.Confidence, 0)
			result.Reason = fmt.Sprintf("architect confidence %.2f < %.2f — escalating to human", design.Confidence, MinArchitectConfidence)
			metricHumanEscalation("architect")
			state = StateHumanRequired
			return result
		}

		// ── Phase 3: Executor + Sandbox ────────────────────────────────────────
		state = StatePatching

		// Filter patches through ledger: skip applied, abort if poisoned.
		var toApply []Patch
		poisoned := false
		for _, patch := range design.Patches {
			hash := PatchHash(patch)
			if a.ledger.IsPoisoned(hash) {
				trace.Record(iter, "executor", "poison patch "+patch.File, patch.Why, "poison", 0, 0)
				metricPatch("poison")
				poisoned = true
				break
			}
			if a.ledger.IsApplied(hash) {
				trace.Record(iter, "executor", "skip applied "+patch.File, patch.Why, "skipped", 0, 0)
				metricPatch("skipped")
				continue
			}
			toApply = append(toApply, patch)
		}

		if poisoned {
			state = StatePoison
			result.Reason = fmt.Sprintf("poison patch detected — failed %d× before", PoisonThreshold)
			return result
		}

		// Apply via sandbox (snapshot → apply → go build).
		// On failure → auto-rollback happens inside ValidateAndApply.
		sandbox := NewSandboxValidator(a.baseDir)
		t2 := time.Now()

		if len(toApply) > 0 {
			applyErr := sandbox.ValidateAndApply(ctx, toApply, a.executor)
			execMs := time.Since(t2).Milliseconds()
			metricStep("executor", time.Since(t2).Seconds())

			if applyErr != nil {
				// Distinguish build failures from apply failures for metrics.
				isBuildFail := contains(applyErr.Error(), "build failed")
				if isBuildFail {
					metricBuildFailure()
				}

				// Record each patch as failed in ledger (may trigger poison).
				for _, patch := range toApply {
					hash := PatchHash(patch)
					nowPoison := a.ledger.RecordFailed(hash, patch.File)
					metricPatch("failed")
					if nowPoison {
						trace.Record(iter, "executor", "patch is now poisoned "+patch.File, applyErr.Error(), "poison", 0, execMs)
						state = StatePoison
						result.Reason = fmt.Sprintf("patch %s marked as poison: %v", patch.File, applyErr)
						return result
					}
				}

				trace.Record(iter, "executor", "apply failed", applyErr.Error(), "failed", 0, execMs)
				prevFailure = applyErr.Error()
				continue
			}

			// Patches applied + build passed.
			for _, patch := range toApply {
				a.ledger.RecordApplied(PatchHash(patch), patch.File)
				metricPatch("applied")
				trace.Record(iter, "executor", "applied "+patch.File, patch.Why, "ok", 0, execMs)
			}
		} else {
			// No new patches — architect thinks it's already fixed; verify to confirm.
			trace.Record(iter, "executor", "no new patches", "all already applied or empty plan", "skipped", 0, 0)
		}

		// ── Phase 4: Verifier ──────────────────────────────────────────────────
		state = StateVerifying
		t3 := time.Now()
		vr := a.verifier.Verify(ctx, plan.Domain)
		verifyMs := time.Since(t3).Milliseconds()
		metricStep("verifier", time.Since(t3).Seconds())
		metricVerifyScore(plan.Domain, vr.Score)

		result.VerifyScore = vr.Score
		trace.Record(iter, "verifier",
			fmt.Sprintf("score=%.2f pass=%v", vr.Score, vr.Pass),
			vr.Reason, boolResult(vr.Pass), vr.Score, verifyMs)

		if vr.Pass {
			// ✅ Goal achieved — commit snapshot backups.
			if sandbox.HasSnapshot() {
				sandbox.Commit()
			}
			state = StateSuccess
			result.Reason = fmt.Sprintf("goal achieved — verify score %.2f after %d iteration(s)", vr.Score, iter+1)
			slog.InfoContext(ctx, "agent loop success",
				"trace_id", traceID, "score", vr.Score, "iters", iter+1)
			return result
		}

		// ❌ Verification failed — rollback production files, re-think.
		if sandbox.HasSnapshot() {
			if rollbackErr := sandbox.Rollback(); rollbackErr != nil {
				slog.ErrorContext(ctx, "rollback failed — production state may be inconsistent",
					"trace_id", traceID, "error", rollbackErr)
			} else {
				slog.WarnContext(ctx, "production rolled back after verification failure",
					"trace_id", traceID, "score", vr.Score)
			}
			metricRollback()
			trace.Record(iter, "executor", "production rolled back", vr.Reason, "failed", 0, 0)
		}

		prevFailure = fmt.Sprintf("verification failed (score=%.2f): %s", vr.Score, vr.Reason)
	}

	// All iterations exhausted.
	state = StateAborted
	result.Reason = fmt.Sprintf("max iterations (%d) reached — last failure: %s", MaxIterations, prevFailure)
	slog.WarnContext(ctx, "agent loop aborted", "trace_id", traceID, "reason", result.Reason)
	return result
}

func boolResult(b bool) string {
	if b {
		return "ok"
	}
	return "failed"
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
