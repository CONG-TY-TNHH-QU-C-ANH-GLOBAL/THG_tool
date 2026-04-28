package agentloop

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// SandboxValidator enforces the safe-deployment contract:
//
//	Architect → Snapshot → Apply → go build → [Verifier] → Promote OR Rollback
//
// Production files are NEVER left in a broken state:
//   - If apply fails mid-batch → rollback via snapshot
//   - If go build fails → rollback via snapshot
//   - If verifier fails → rollback via snapshot (caller responsibility)
//   - Only if everything passes → Commit() removes backups
//
// Invariant ATOMIC_PATCH: either all patches in a batch are live, or none.
type SandboxValidator struct {
	baseDir  string
	snapshot *Snapshot
}

// NewSandboxValidator creates a validator rooted at baseDir (the repo root).
func NewSandboxValidator(baseDir string) *SandboxValidator {
	return &SandboxValidator{
		baseDir:  baseDir,
		snapshot: NewSnapshot(),
	}
}

// ValidateAndApply is the safe entry point for Phase 3 (Executor).
// Steps:
//  1. Snapshot all target files (backup originals).
//  2. Apply all patches via executor.
//  3. Run `go build ./...` from baseDir.
//  4. If step 2 or 3 fails → auto-rollback → return error.
//  5. Return nil — patches are live, build passes.
//     Caller MUST call Rollback() if verifier fails, or Commit() on success.
func (sv *SandboxValidator) ValidateAndApply(ctx context.Context, patches []Patch, ex *Executor) error {
	// Step 1: snapshot all files that will be modified.
	files := uniqueFiles(patches)
	if err := sv.snapshot.Take(sv.baseDir, files); err != nil {
		return fmt.Errorf("sandbox: snapshot: %w", err)
	}

	// Step 2: apply patches.
	if _, applyErr := ex.ApplyAll(patches); applyErr != nil {
		_ = sv.snapshot.Rollback()
		return fmt.Errorf("sandbox: apply: %w", applyErr)
	}

	// Step 3: build validation — catches type errors, undefined symbols, etc.
	if buildErr := sv.build(ctx); buildErr != nil {
		_ = sv.snapshot.Rollback()
		return fmt.Errorf("sandbox: build failed (patches rolled back): %w", buildErr)
	}

	// Patches are live and build passes.
	// Do NOT commit yet — caller runs verifier first.
	return nil
}

// Rollback restores production files from the last snapshot.
// Must be called when the verifier fails after ValidateAndApply succeeded.
func (sv *SandboxValidator) Rollback() error {
	return sv.snapshot.Rollback()
}

// Commit removes backup files. Must be called when the verifier passes.
func (sv *SandboxValidator) Commit() {
	sv.snapshot.Commit()
}

// HasSnapshot reports whether there are files currently backed up.
func (sv *SandboxValidator) HasSnapshot() bool {
	return sv.snapshot.Len() > 0
}

// build runs `go build ./...` in baseDir and returns a human-readable error on failure.
func (sv *SandboxValidator) build(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = sv.baseDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = err.Error()
		}
		return fmt.Errorf("go build: %s", output)
	}
	return nil
}

// uniqueFiles returns deduplicated file paths from a patch list.
func uniqueFiles(patches []Patch) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range patches {
		if !seen[p.File] {
			seen[p.File] = true
			out = append(out, p.File)
		}
	}
	return out
}
