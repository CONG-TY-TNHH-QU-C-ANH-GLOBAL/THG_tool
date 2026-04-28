package agentloop

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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

// domainLocks prevents two agent runs from patching the same domain concurrently.
// A corrupted codebase is worse than a delayed fix.
// Invariant SINGLE_WRITER_PER_DOMAIN: only one active run per domain at a time.
var domainLocks sync.Map

func acquireDomainLock(domain Domain) func() {
	mu, _ := domainLocks.LoadOrStore(domain, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// AgentLoop is the main orchestrator.
//
// Full pipeline per iteration:
//
//	Planner (LLM) → Architect (LLM) → IsolatedSandbox → Verifier → Promote|Rollback
//
// Safe-deployment contract:
//
//	Patches are NEVER permanently applied until:
//	  1. go build ./... passes in /tmp sandbox (not production)
//	  2. Verifier score ≥ VerifyPassThreshold (goal-based success)
//	  3. Verifier stability loop passes (domain-appropriate timing)
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
	// AlertFn is called on terminal/escalation states.
	// Wire to Telegram, Slack, or any notification channel.
	// Optional — nil disables alerts.
	AlertFn func(state AgentState, traceID, reason string)
}

// New creates a fully wired AgentLoop.
func New(apiKey, plannerModel, architectModel, baseDir string, verifyCfg VerifyConfig) *AgentLoop {
	return &AgentLoop{
		planner:   NewPlanner(apiKey, plannerModel),
		architect: NewArchitect(apiKey, architectModel),
		executor:  NewExecutor(baseDir),
		verifier:  NewVerifier(verifyCfg),
		ledger:    newPersistentLedger(baseDir),
		baseDir:   baseDir,
	}
}

// alert fires AlertFn if set, and always logs at the appropriate level.
func (a *AgentLoop) alert(ctx context.Context, state AgentState, traceID, reason string) {
	switch state {
	case StatePoison, StateAborted, StateFailed:
		slog.ErrorContext(ctx, "agent terminal state",
			"trace_id", traceID, "state", state, "reason", reason)
	case StateHumanRequired:
		slog.WarnContext(ctx, "agent escalated to human",
			"trace_id", traceID, "state", state, "reason", reason)
	}
	if a.AlertFn != nil {
		a.AlertFn(state, traceID, reason)
	}
}

// Run executes the full agent loop for the given task.
// Concurrent calls on the same domain are serialised via domainLocks.
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
		a.alert(ctx, state, traceID, result.Reason)
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
		a.alert(ctx, state, traceID, result.Reason)
		return result
	}

	// ── Domain lock: only one active run per domain ────────────────────────────
	// Prevents two concurrent runs from racing to patch the same files.
	unlock := acquireDomainLock(plan.Domain)
	defer unlock()

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
			a.alert(ctx, state, traceID, result.Reason)
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
			a.alert(ctx, state, traceID, result.Reason)
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
			a.alert(ctx, state, traceID, result.Reason)
			return result
		}

		// Blast radius gate — hard stop before any file is touched.
		if len(toApply) > 0 {
			brc := NewBlastRadiusChecker(DefaultBlastRadius)
			if brErr := brc.Check(toApply); brErr != nil {
				trace.Record(iter, "executor", "blast radius exceeded", brErr.Error(), "failed", 0, 0)
				result.Reason = brErr.Error()
				state = StateFailed
				a.alert(ctx, state, traceID, result.Reason)
				return result
			}
		}

		// Apply via isolated sandbox:
		//   1. Copy Go source to /tmp/agentloop-{traceID}/
		//   2. Apply patches there
		//   3. go build ./... in sandbox
		//   4. Promote (snapshot + apply) to production
		//   5. Verifier decides commit or rollback
		sandbox, sandboxErr := NewIsolatedSandbox(traceID, a.baseDir)
		if sandboxErr != nil {
			trace.Record(iter, "executor", "sandbox init failed", sandboxErr.Error(), "failed", 0, 0)
			prevFailure = "sandbox init: " + sandboxErr.Error()
			continue
		}
		defer sandbox.Discard()

		t2 := time.Now()

		if len(toApply) > 0 {
			// Step 1: apply patches in sandbox (production untouched).
			if applyErr := sandbox.Apply(toApply); applyErr != nil {
				execMs := time.Since(t2).Milliseconds()
				trace.Record(iter, "executor", "sandbox apply failed", applyErr.Error(), "failed", 0, execMs)
				for _, patch := range toApply {
					a.ledger.RecordFailed(PatchHash(patch), patch.File)
					metricPatch("failed")
				}
				prevFailure = applyErr.Error()
				continue
			}

			// Step 2: build in sandbox — must pass before production is touched.
			if buildErr := sandbox.Build(ctx); buildErr != nil {
				execMs := time.Since(t2).Milliseconds()
				metricBuildFailure()
				trace.Record(iter, "executor", "sandbox build failed", buildErr.Error(), "failed", 0, execMs)
				for _, patch := range toApply {
					hash := PatchHash(patch)
					nowPoison := a.ledger.RecordFailed(hash, patch.File)
					metricPatch("failed")
					if nowPoison {
						trace.Record(iter, "executor", "patch is now poisoned "+patch.File, buildErr.Error(), "poison", 0, execMs)
						state = StatePoison
						result.Reason = fmt.Sprintf("patch %s marked as poison after build failure: %v", patch.File, buildErr)
						a.alert(ctx, state, traceID, result.Reason)
						return result
					}
				}
				prevFailure = buildErr.Error()
				continue
			}

			// Step 3: promote to production (snapshot backup + apply).
			execMs := time.Since(t2).Milliseconds()
			metricStep("executor", time.Since(t2).Seconds())

			if promoteErr := sandbox.Promote(toApply, a.executor); promoteErr != nil {
				trace.Record(iter, "executor", "promote failed", promoteErr.Error(), "failed", 0, execMs)
				prevFailure = promoteErr.Error()
				continue
			}

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
			sandbox.Commit()
			state = StateSuccess
			result.Reason = fmt.Sprintf("goal achieved — verify score %.2f after %d iteration(s)", vr.Score, iter+1)
			slog.InfoContext(ctx, "agent loop success",
				"trace_id", traceID, "score", vr.Score, "iters", iter+1)
			return result
		}

		// ❌ Verification failed — rollback production files, re-think.
		if rollbackErr := sandbox.Rollback(); rollbackErr != nil {
			slog.ErrorContext(ctx, "rollback failed — production state may be inconsistent",
				"trace_id", traceID, "error", rollbackErr)
		} else {
			slog.WarnContext(ctx, "production rolled back after verification failure",
				"trace_id", traceID, "score", vr.Score)
		}
		metricRollback()
		trace.Record(iter, "executor", "production rolled back", vr.Reason, "failed", 0, 0)

		prevFailure = fmt.Sprintf("verification failed (score=%.2f): %s", vr.Score, vr.Reason)
	}

	// All iterations exhausted.
	state = StateAborted
	result.Reason = fmt.Sprintf("max iterations (%d) reached — last failure: %s", MaxIterations, prevFailure)
	a.alert(ctx, state, traceID, result.Reason)
	return result
}

func boolResult(b bool) string {
	if b {
		return "ok"
	}
	return "failed"
}
