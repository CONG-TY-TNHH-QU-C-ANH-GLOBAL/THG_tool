package agentloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// protectedFiles is the set of file paths that the Executor will NEVER modify.
// Invariant: modifications to these files require human review.
var protectedFiles = map[string]bool{
	"internal/session/statemachine.go": true,
	"internal/auth/auth.go":            true,
	"cmd/scraper/main.go":              true,
	"cmd/worker/main.go":               true,
}

// ErrProtectedFile is returned when a patch targets a protected file.
var ErrProtectedFile = fmt.Errorf("executor: patch targets a protected file")

// ErrFakeFile is returned when the patch file does not exist on disk.
var ErrFakeFile = fmt.Errorf("executor: file does not exist (FILE_REALITY invariant)")

// ErrTargetNotFound is returned when the patch target string is not found in the file.
var ErrTargetNotFound = fmt.Errorf("executor: target not found in file")

// Executor applies patches atomically with a risk gate and file reality check.
// All writes are: write → tmp → rename (atomic on POSIX; best-effort on Windows).
type Executor struct {
	baseDir string // absolute base directory for resolving relative paths
}

// NewExecutor creates an Executor rooted at baseDir.
func NewExecutor(baseDir string) *Executor {
	return &Executor{baseDir: baseDir}
}

// ApplyAll applies a slice of patches in order.
// Returns the index of the first failed patch and the error.
// All patches before the failure are already written — the caller is responsible
// for deciding whether to revert (not done here: atomic per-file, not transactional).
func (e *Executor) ApplyAll(patches []Patch) (int, error) {
	for i, p := range patches {
		if err := e.Apply(p); err != nil {
			return i, fmt.Errorf("patch[%d] %s: %w", i, p.File, err)
		}
	}
	return -1, nil
}

// Apply applies a single patch atomically.
func (e *Executor) Apply(p Patch) error {
	// ── Risk Gate ──────────────────────────────────────────────────────────────
	normalized := filepath.ToSlash(p.File)
	if protectedFiles[normalized] {
		return ErrProtectedFile
	}

	// ── File Reality Check ─────────────────────────────────────────────────────
	abs := e.abs(p.File)
	original, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrFakeFile, p.File)
	}

	// ── Patch Simulation (in-memory) ───────────────────────────────────────────
	patched, err := applyPatch(string(original), p)
	if err != nil {
		return err
	}

	// ── Atomic Write ───────────────────────────────────────────────────────────
	// Write to a sibling tmp file, then rename.
	tmp := abs + ".agentloop.tmp"
	if err := os.WriteFile(tmp, []byte(patched), 0644); err != nil {
		return fmt.Errorf("executor: write tmp: %w", err)
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp) // cleanup on rename failure
		return fmt.Errorf("executor: rename: %w", err)
	}

	return nil
}

// applyPatch transforms original content according to the patch action.
// Returns an error if the target cannot be found (when required by the action).
func applyPatch(original string, p Patch) (string, error) {
	switch p.Action {
	case ActionReplaceBlock:
		return replaceBlock(original, p.Target, p.Content)
	case ActionInsertAfter:
		return insertAfter(original, p.Target, p.Content)
	case ActionDeleteBlock:
		return deleteBlock(original, p.Target)
	case ActionPrependImport:
		return prependImport(original, p.Content)
	case ActionAppend:
		return original + "\n" + p.Content, nil
	default:
		return "", fmt.Errorf("executor: unknown action %q", p.Action)
	}
}

// replaceBlock replaces from the line containing target through the next matching `}` at the same indent.
// For simpler cases (no closing brace needed), it replaces just the matching line.
func replaceBlock(src, target, replacement string) (string, error) {
	lines := strings.Split(src, "\n")
	startIdx := -1
	for i, l := range lines {
		if strings.Contains(l, target) {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return "", fmt.Errorf("%w: %q", ErrTargetNotFound, target)
	}

	// Determine indentation of the target line.
	targetLine := lines[startIdx]
	indent := len(targetLine) - len(strings.TrimLeft(targetLine, " \t"))
	indentStr := targetLine[:indent]

	// Find closing brace at same indent level.
	endIdx := startIdx
	for i := startIdx + 1; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t")
		if l == indentStr+"}" || l == indentStr+"};" {
			endIdx = i
			break
		}
	}

	var out []string
	out = append(out, lines[:startIdx]...)
	out = append(out, replacement)
	out = append(out, lines[endIdx+1:]...)
	return strings.Join(out, "\n"), nil
}

// insertAfter inserts content after the first line containing target.
func insertAfter(src, target, content string) (string, error) {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.Contains(l, target) {
			var out []string
			out = append(out, lines[:i+1]...)
			out = append(out, content)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n"), nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrTargetNotFound, target)
}

// deleteBlock deletes from the line containing target to its closing brace.
func deleteBlock(src, target string) (string, error) {
	lines := strings.Split(src, "\n")
	startIdx := -1
	for i, l := range lines {
		if strings.Contains(l, target) {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return "", fmt.Errorf("%w: %q", ErrTargetNotFound, target)
	}

	targetLine := lines[startIdx]
	indent := len(targetLine) - len(strings.TrimLeft(targetLine, " \t"))
	indentStr := targetLine[:indent]

	endIdx := startIdx
	for i := startIdx + 1; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t")
		if l == indentStr+"}" || l == indentStr+"};" {
			endIdx = i
			break
		}
	}

	var out []string
	out = append(out, lines[:startIdx]...)
	out = append(out, lines[endIdx+1:]...)
	return strings.Join(out, "\n"), nil
}

// prependImport adds an import path to the file's import block.
// If the import already exists, returns the original unchanged.
func prependImport(src, importPath string) (string, error) {
	if strings.Contains(src, importPath) {
		return src, nil // already present
	}
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "import (" {
			var out []string
			out = append(out, lines[:i+1]...)
			out = append(out, "\t\""+importPath+"\"")
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n"), nil
		}
	}
	return "", fmt.Errorf("executor: no import block found in file")
}

func (e *Executor) abs(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(e.baseDir, rel)
}
