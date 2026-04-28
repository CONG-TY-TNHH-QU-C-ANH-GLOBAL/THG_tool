package agentloop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Snapshot takes file backups before any patch is applied, enabling full rollback
// if the build or verification fails.
//
// Lifecycle:
//
//	Take()     → copy originals to .agentbak.{ts} siblings
//	Rollback() → restore originals from backups, then delete backups
//	Commit()   → delete backups (patches are permanently live)
type Snapshot struct {
	entries []snapshotEntry
}

type snapshotEntry struct {
	original string // absolute path of the production file
	backup   string // absolute path of the .agentbak.{ts} copy
}

// NewSnapshot creates an empty snapshot ready to take entries.
func NewSnapshot() *Snapshot { return &Snapshot{} }

// Take copies each relative path (resolved against baseDir) to a sibling backup file.
// Safe to call on files that don't exist yet (they are skipped — no original to back up).
func (s *Snapshot) Take(baseDir string, relPaths []string) error {
	ts := time.Now().UnixNano()
	for _, rel := range relPaths {
		abs := filepath.Join(baseDir, filepath.FromSlash(rel))
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			continue // new file — nothing to back up
		}
		backup := fmt.Sprintf("%s.agentbak.%d", abs, ts)
		if err := copyFile(abs, backup); err != nil {
			return fmt.Errorf("snapshot %s: %w", rel, err)
		}
		s.entries = append(s.entries, snapshotEntry{original: abs, backup: backup})
	}
	return nil
}

// Rollback restores every snapshotted file from its backup and then
// deletes the backups. Safe to call multiple times.
func (s *Snapshot) Rollback() error {
	var lastErr error
	for _, e := range s.entries {
		if err := copyFile(e.backup, e.original); err != nil {
			lastErr = fmt.Errorf("rollback %s: %w", e.original, err)
		}
	}
	s.Commit() // always remove backups regardless of restore errors
	return lastErr
}

// Commit deletes all backup files. Call after successful production promotion.
func (s *Snapshot) Commit() {
	for _, e := range s.entries {
		_ = os.Remove(e.backup)
	}
	s.entries = nil
}

// Len returns how many files are currently snapshotted.
func (s *Snapshot) Len() int { return len(s.entries) }

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
