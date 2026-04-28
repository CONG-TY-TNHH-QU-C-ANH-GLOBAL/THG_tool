package agentloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// BlastRadiusConfig constrains how much an agent run is allowed to change.
// Prevents a single run from destabilising the whole codebase.
type BlastRadiusConfig struct {
	// MaxFiles is the maximum number of distinct files that can be patched per run.
	// Default: 5. Set to 0 to disable.
	MaxFiles int

	// MaxContentBytes is the maximum total size of all patch Content fields combined.
	// A rough proxy for "lines changed". Default: 10_000 bytes (~250 lines).
	MaxContentBytes int

	// BlockedDirs are directory prefixes (slash-separated) the agent is not allowed
	// to touch even if not in protectedFiles. Examples: "cmd/", ".github/".
	BlockedDirs []string
}

// DefaultBlastRadius is the production-safe default.
var DefaultBlastRadius = BlastRadiusConfig{
	MaxFiles:        5,
	MaxContentBytes: 10_000,
	BlockedDirs: []string{
		"cmd/",
		"deploy/",
		".github/",
		"docker/",
		"migrations/",
	},
}

// ErrBlastRadiusExceeded is returned when any blast radius limit is breached.
var ErrBlastRadiusExceeded = errors.New("blast radius limit exceeded")

// BlastRadiusChecker validates a patch batch before the Executor runs.
type BlastRadiusChecker struct {
	cfg BlastRadiusConfig
}

// NewBlastRadiusChecker creates a checker with the provided config.
func NewBlastRadiusChecker(cfg BlastRadiusConfig) *BlastRadiusChecker {
	return &BlastRadiusChecker{cfg: cfg}
}

// Check returns an error if the patch batch exceeds any configured limit.
// This is called BEFORE the Executor touches any files.
func (b *BlastRadiusChecker) Check(patches []Patch) error {
	files := uniqueFiles(patches)

	// 1. File count limit.
	if b.cfg.MaxFiles > 0 && len(files) > b.cfg.MaxFiles {
		return fmt.Errorf("%w: %d files > max %d",
			ErrBlastRadiusExceeded, len(files), b.cfg.MaxFiles)
	}

	// 2. Total content size limit.
	var totalBytes int
	for _, p := range patches {
		totalBytes += len(p.Content)
	}
	if b.cfg.MaxContentBytes > 0 && totalBytes > b.cfg.MaxContentBytes {
		return fmt.Errorf("%w: patch content %d bytes > max %d",
			ErrBlastRadiusExceeded, totalBytes, b.cfg.MaxContentBytes)
	}

	// 3. Blocked directory check.
	for _, f := range files {
		norm := filepath.ToSlash(f)
		for _, dir := range b.cfg.BlockedDirs {
			if strings.HasPrefix(norm, dir) {
				return fmt.Errorf("%w: file %q is in blocked directory %q",
					ErrBlastRadiusExceeded, f, dir)
			}
		}
	}

	return nil
}
