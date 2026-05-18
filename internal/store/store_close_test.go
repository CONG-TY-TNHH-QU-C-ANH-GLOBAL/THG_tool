package store

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestStore_CloseIsDeterministicUnderTimeout is the guardrail behind
// the CI hang fix (2026-05-19). It pins down four properties Close()
// must hold under the race detector:
//
//  1. Close returns within closeTimeout. The most concrete symptom of
//     a leaked rows/stmt/tx in modernc.org/sqlite is db.Close blocking
//     forever — that ate a full 120s CI run.
//  2. After a typical workload (read + write + read), Close returns nil
//     (no ErrCloseTimedOut). A timeout here means we re-introduced a
//     leak somewhere in the store.
//  3. Calling Close twice is safe (modernc.org/sqlite returns an
//     error on the second call, which we propagate; we just must not
//     panic or hang).
//  4. After Close, no goroutines linger past a small noise floor. The
//     database/sql cleanup goroutine + any modernc.org/sqlite background
//     work must terminate.
func TestStore_CloseIsDeterministicUnderTimeout(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "close_invariant.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Run a representative workload that previously caused hangs:
	// concurrent writes (queued through the single connection),
	// reads via rows iteration, and a final aggregate query.
	ctx := context.Background()

	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS close_probe (id INTEGER PRIMARY KEY, v INTEGER)`); err != nil {
		t.Fatalf("create probe: %v", err)
	}

	const writers = 8
	const writesPerWriter = 25
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for i := range writesPerWriter {
				_, _ = db.DB().ExecContext(ctx,
					`INSERT INTO close_probe (v) VALUES (?)`, idx*1000+i)
			}
		}(w)
	}
	wg.Wait()

	// Read path with proper rows.Close. If we forget defer rows.Close()
	// HERE, Close() below will fire ErrCloseTimedOut — that's the exact
	// failure mode this test guards against.
	rows, err := db.DB().QueryContext(ctx, `SELECT id, v FROM close_probe LIMIT 50`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var id, v int
		_ = rows.Scan(&id, &v)
	}
	rows.Close()

	gBefore := runtime.NumGoroutine()
	start := time.Now()
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	took := time.Since(start)
	if took > closeTimeout {
		t.Errorf("Close took %s, exceeded closeTimeout %s", took, closeTimeout)
	}

	// Second Close must NOT hang. modernc.org/sqlite returns an error
	// on double-close; we accept any outcome except a timeout.
	start = time.Now()
	err2 := db.Close()
	took2 := time.Since(start)
	if err2 == ErrCloseTimedOut {
		t.Errorf("second Close timed out: %v", err2)
	}
	if took2 > closeTimeout {
		t.Errorf("second Close took %s, exceeded closeTimeout", took2)
	}

	// Allow background cleanup to settle, then check for goroutine
	// leaks. Two stragglers is the noise floor (test runtime spawns
	// occasional goroutines unrelated to the store).
	time.Sleep(50 * time.Millisecond)
	gAfter := runtime.NumGoroutine()
	if gAfter > gBefore+2 {
		t.Errorf("goroutine leak after Close: before=%d after=%d", gBefore, gAfter)
	}
}

// TestStore_CloseUnderConcurrentLoad reproduces the workload shape
// that hung CI before the bounded-pool fix: many concurrent writes
// + an explicit query iteration + Close. Without MaxOpenConns(1)
// and bounded Close, the race detector + modernc.org/sqlite combo
// could hang past the 120s test timeout. This test must finish well
// under that bound.
func TestStore_CloseUnderConcurrentLoad(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "close_concurrent.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS load_probe (id INTEGER PRIMARY KEY, v INTEGER)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	const workers = 16
	const queriesPerWorker = 20
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for i := range queriesPerWorker {
				_, _ = db.DB().ExecContext(ctx,
					`INSERT INTO load_probe (v) VALUES (?)`, idx*1000+i)
				rows, err := db.DB().QueryContext(ctx,
					`SELECT id FROM load_probe WHERE v = ?`, idx*1000+i)
				if err != nil {
					continue
				}
				for rows.Next() {
					var id int
					_ = rows.Scan(&id)
				}
				rows.Close()
			}
		}(w)
	}
	wg.Wait()

	start := time.Now()
	if err := db.Close(); err != nil {
		t.Fatalf("Close after concurrent load: %v", err)
	}
	if took := time.Since(start); took > closeTimeout {
		t.Errorf("Close took %s under concurrent load", took)
	}
}
