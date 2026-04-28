package agentloop

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// PoisonThreshold is the number of times the same patch must fail before
// it is marked as poisoned and the loop is aborted.
// Invariant NO_REPEAT_FAILURE: fail 3× → stop.
const PoisonThreshold = 3

// ActionLedger tracks every patch applied in one agent run.
// Prevents duplicate patches (idempotency) and detects poison patches.
// Thread-safe.
type ActionLedger struct {
	mu      sync.Mutex
	entries map[string]*LedgerEntry // patchHash → entry
}

func newActionLedger() *ActionLedger {
	return &ActionLedger{entries: make(map[string]*LedgerEntry)}
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
