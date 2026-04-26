package engine

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// LoopController drives the autonomous CI engineering loop.
// It is a TOOL-AUTOMATED system — not AI inference — that reads
// build/test output, classifies failures, enforces retry limits,
// and emits structured FixRequests for patch tools (Superpowers, etc.).
//
// Safety invariants:
//   - never auto-deploys to production
//   - commits only when ALL tests pass
//   - max 3 retries per unique failure fingerprint
//   - idempotency model is read-only from this layer's perspective
type LoopController struct {
	maxRetries int
	workDir    string
	retryCount map[string]int // keyed by failure fingerprint
}

// BuildResult carries the structured output of a build or test run.
type BuildResult struct {
	Success     bool
	Output      string
	FailureType string // compile | test | lint | validation
	FailureMsg  string
}

// FixRequest is the structured payload sent to a patch tool on failure.
type FixRequest struct {
	Attempt      int       `json:"attempt"`
	MaxAttempts  int       `json:"max_attempts"`
	FailureType  string    `json:"failure_type"`
	FailureMsg   string    `json:"failure_msg"`
	Output       string    `json:"output"`
	Instructions string    `json:"instructions"`
	GeneratedAt  time.Time `json:"generated_at"`
}

func New(maxRetries int, workDir string) *LoopController {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &LoopController{
		maxRetries: maxRetries,
		workDir:    workDir,
		retryCount: make(map[string]int),
	}
}

// Validate runs architecture and compile validation (go build + go vet).
func (lc *LoopController) Validate(ctx context.Context) BuildResult {
	out, err := lc.run(ctx, "go", "build", "./...")
	if err != nil {
		return BuildResult{
			Success:     false,
			Output:      out,
			FailureType: "compile",
			FailureMsg:  err.Error(),
		}
	}
	out, err = lc.run(ctx, "go", "vet", "./...")
	if err != nil {
		return BuildResult{
			Success:     false,
			Output:      out,
			FailureType: "lint",
			FailureMsg:  err.Error(),
		}
	}
	return BuildResult{Success: true, Output: out}
}

// RunTests executes the full test suite with race detector.
func (lc *LoopController) RunTests(ctx context.Context) BuildResult {
	out, err := lc.run(ctx, "go", "test", "-race", "-timeout", "120s", "./...")
	if err != nil {
		return BuildResult{
			Success:     false,
			Output:      out,
			FailureType: detectFailureType(out),
			FailureMsg:  err.Error(),
		}
	}
	return BuildResult{Success: true, Output: out}
}

// HandleFailure processes a failed result, enforces retry budget,
// and returns a FixRequest for a downstream patch tool.
// Returns an error when the retry budget is exhausted.
func (lc *LoopController) HandleFailure(result BuildResult) (FixRequest, error) {
	key := failureFingerprint(result)
	lc.retryCount[key]++

	if lc.retryCount[key] > lc.maxRetries {
		return FixRequest{}, fmt.Errorf(
			"engine: max retries (%d) exhausted for %s failure: %s",
			lc.maxRetries, result.FailureType, truncate(result.FailureMsg, 120),
		)
	}

	req := FixRequest{
		Attempt:      lc.retryCount[key],
		MaxAttempts:  lc.maxRetries,
		FailureType:  result.FailureType,
		FailureMsg:   result.FailureMsg,
		Output:       result.Output,
		Instructions: buildInstructions(result),
		GeneratedAt:  time.Now().UTC(),
	}

	log.Printf("engine: fix request attempt=%d/%d type=%s msg=%s",
		req.Attempt, req.MaxAttempts, req.FailureType, truncate(req.FailureMsg, 80))

	return req, nil
}

// RunCI executes the full CI pipeline: validate → test.
// Returns (pass, FixRequest if failed).
func (lc *LoopController) RunCI(ctx context.Context) (bool, FixRequest, error) {
	if r := lc.Validate(ctx); !r.Success {
		fix, err := lc.HandleFailure(r)
		return false, fix, err
	}
	if r := lc.RunTests(ctx); !r.Success {
		fix, err := lc.HandleFailure(r)
		return false, fix, err
	}
	return true, FixRequest{}, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (lc *LoopController) run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = lc.workDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func detectFailureType(output string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "syntax error"),
		strings.Contains(lower, "cannot use"),
		strings.Contains(lower, "undefined:"),
		strings.Contains(lower, "declared and not used"):
		return "compile"
	case strings.Contains(lower, "--- fail"),
		strings.Contains(lower, "panic:"),
		strings.Contains(lower, "data race"):
		return "test"
	default:
		return "unknown"
	}
}

func failureFingerprint(r BuildResult) string {
	return r.FailureType + ":" + truncate(r.FailureMsg, 80)
}

func buildInstructions(r BuildResult) string {
	switch r.FailureType {
	case "compile":
		return "Fix all compile errors. Check imports, type mismatches, undefined symbols. " +
			"Do NOT change function signatures or public interfaces."
	case "test":
		return "Fix failing tests without deleting them. " +
			"Preserve all idempotency logic (task_id hash, INSERT OR IGNORE). " +
			"Do NOT weaken assertions."
	case "lint":
		return "Fix all go vet warnings. Do NOT change business logic or public APIs."
	default:
		return "Investigate and fix the failure: " + r.FailureMsg
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
