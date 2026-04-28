package agentloop

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PoisonThreshold is the number of times the same patch must fail before
// it is marked as poisoned and the loop is aborted.
// Invariant NO_REPEAT_FAILURE: fail 3× → stop.
const PoisonThreshold = 3

// ActionLedger tracks every patch applied in one agent run.
// Prevents duplicate patches (idempotency) and detects poison patches.
// Persisted to disk so poison history survives process restarts.
// Thread-safe.
type ActionLedger struct {
	mu       sync.Mutex
	entries  map[string]*LedgerEntry // patchHash → entry
	filePath string                  // empty = in-memory only
}

// newPersistentLedger loads or creates a ledger persisted at baseDir/.agentloop/ledger.json.
func newPersistentLedger(baseDir string) *ActionLedger {
	dir := filepath.Join(baseDir, ".agentloop")
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "ledger.json")

	l := &ActionLedger{
		entries:  make(map[string]*LedgerEntry),
		filePath: path,
	}
	if data, err := os.ReadFile(path); err == nil {
		var loaded []LedgerEntry
		if json.Unmarshal(data, &loaded) == nil {
			for i := range loaded {
				e := loaded[i]
				l.entries[e.PatchHash] = &e
			}
			slog.Info("agentloop ledger loaded", "entries", len(l.entries), "path", path)
		}
	}
	return l
}

// save flushes the ledger to disk. Must be called under l.mu.
func (l *ActionLedger) save() {
	if l.filePath == "" {
		return
	}
	entries := make([]LedgerEntry, 0, len(l.entries))
	for _, e := range l.entries {
		entries = append(entries, *e)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	tmp := l.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, l.filePath)
}

// PatchHash returns a stable SHA-256 hash for a patch (file + action + target + content).
func PatchHash(p Patch) string {
	h := sha256.New()
	b, _ := json.Marshal(struct {
		File    string
		Action  PatchAction
		Target  string
		Content string
	}{p.File, p.Action, p.Target, p.Content})
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// IsApplied returns true if this exact patch was already successfully applied.
func (l *ActionLedger) IsApplied(hash string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[hash]
	return ok && e.Status == "applied"
}

// IsPoisoned returns true when the same patch has failed PoisonThreshold times.
func (l *ActionLedger) IsPoisoned(hash string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[hash]
	return ok && e.FailCount >= PoisonThreshold
}

// RecordApplied marks a patch as successfully applied.
func (l *ActionLedger) RecordApplied(hash, file string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.getOrCreate(hash, file)
	e.Status = "applied"
	e.At = time.Now().UTC()
	l.save()
}

// RecordFailed increments the fail counter and returns true if the patch
// has now reached PoisonThreshold (caller should abort the loop).
func (l *ActionLedger) RecordFailed(hash, file string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.getOrCreate(hash, file)
	e.Status = "failed"
	e.FailCount++
	e.At = time.Now().UTC()
	l.save()
	return e.FailCount >= PoisonThreshold
}

// Snapshot returns all ledger entries at this moment.
func (l *ActionLedger) Snapshot() []LedgerEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LedgerEntry, 0, len(l.entries))
	for _, e := range l.entries {
		out = append(out, *e)
	}
	return out
}

func (l *ActionLedger) getOrCreate(hash, file string) *LedgerEntry {
	if e, ok := l.entries[hash]; ok {
		return e
	}
	e := &LedgerEntry{PatchHash: hash, File: file}
	l.entries[hash] = e
	return e
}
