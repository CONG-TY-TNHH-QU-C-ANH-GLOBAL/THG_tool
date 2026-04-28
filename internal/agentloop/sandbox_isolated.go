package agentloop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsolatedSandbox implements true sandbox isolation.
//
// Production files are NEVER modified until after:
//   1. Patches applied in /tmp/agentloop-{traceID}/
//   2. `go build ./...` passes inside the sandbox
//   3. Verifier passes against the promoted production state
//
// Flow:
//   NewIsolatedSandbox → Apply → Build → Promote → [Verifier] → Commit | Rollback → Discard
//
// Contrast with SandboxValidator (old): that patched production then rolled back.
// IsolatedSandbox never touches production until the build is verified clean.
type IsolatedSandbox struct {
	traceID    string
	baseDir    string    // production repo root (read-only until Promote)
	sandboxDir string    // /tmp/agentloop-{traceID}/
	snapshot   *Snapshot // production snapshot taken at Promote time
	promoted   bool      // true after Promote() has run
}

// sandboxSkipDirs lists directories that are never copied to the sandbox.
// Keeping the sandbox small makes builds fast.
var sandboxSkipDirs = []string{
	".git", "node_modules", "frontend/.next", "data", ".agent-sandbox",
	"vendor", "tmp", "dist", "build",
}

// sandboxCopyExts lists file extensions that ARE copied.
var sandboxCopyExts = map[string]bool{
	".go":  true,
	".mod": true, // go.mod
	".sum": true, // go.sum
}

// NewIsolatedSandbox creates a temp directory and copies all Go source files
// from baseDir into it, preserving the directory structure.
func NewIsolatedSandbox(traceID, baseDir string) (*IsolatedSandbox, error) {
	sandboxDir := filepath.Join(os.TempDir(), "agentloop-"+traceID)
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("isolated sandbox: create dir: %w", err)
	}

	if err := copySourceTree(baseDir, sandboxDir); err != nil {
		_ = os.RemoveAll(sandboxDir)
		return nil, fmt.Errorf("isolated sandbox: copy source tree: %w", err)
	}

	return &IsolatedSandbox{
		traceID:    traceID,
		baseDir:    baseDir,
		sandboxDir: sandboxDir,
		snapshot:   NewSnapshot(),
	}, nil
}

// Apply applies patches inside the sandbox directory.
// Production is untouched.
func (s *IsolatedSandbox) Apply(patches []Patch) error {
	sandboxExec := NewExecutor(s.sandboxDir)
	if _, err := sandboxExec.ApplyAll(patches); err != nil {
		return fmt.Errorf("isolated sandbox: apply: %w", err)
	}
	return nil
}

// Build runs `go build ./...` inside the sandbox.
// Returns a human-readable error including compiler output on failure.
func (s *IsolatedSandbox) Build(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = s.sandboxDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = err.Error()
		}
		return fmt.Errorf("sandbox build failed:\n%s", output)
	}
	return nil
}

// Promote copies sandbox-patched files to production with a snapshot backup.
// After Promote(), production holds the patched code.
// The caller MUST call Rollback() on verify failure or Commit() on success.
func (s *IsolatedSandbox) Promote(patches []Patch, ex *Executor) error {
	// Snapshot production files before overwriting.
	files := uniqueFiles(patches)
	if err := s.snapshot.Take(s.baseDir, files); err != nil {
		return fmt.Errorf("isolated sandbox: snapshot: %w", err)
	}

	// Apply the same patches to production.
	if _, err := ex.ApplyAll(patches); err != nil {
		_ = s.snapshot.Rollback()
		return fmt.Errorf("isolated sandbox: promote apply: %w", err)
	}
	s.promoted = true
	return nil
}

// Rollback restores production from the snapshot taken at Promote time.
// Safe to call even if Promote was never called.
func (s *IsolatedSandbox) Rollback() error {
	if !s.promoted {
		return nil
	}
	return s.snapshot.Rollback()
}

// Commit removes snapshot backups. Call after verifier passes.
func (s *IsolatedSandbox) Commit() {
	s.snapshot.Commit()
}

// Discard removes the sandbox directory. Always defer this call.
func (s *IsolatedSandbox) Discard() {
	_ = os.RemoveAll(s.sandboxDir)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func copySourceTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable files
		}

		rel, _ := filepath.Rel(src, path)
		rel = filepath.ToSlash(rel)

		// Skip blocked directories.
		for _, skip := range sandboxSkipDirs {
			if rel == skip || strings.HasPrefix(rel, skip+"/") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0755)
		}

		// Only copy source-relevant files.
		if !sandboxCopyExts[filepath.Ext(path)] {
			return nil
		}

		return copyFile(path, filepath.Join(dst, rel))
	})
}
