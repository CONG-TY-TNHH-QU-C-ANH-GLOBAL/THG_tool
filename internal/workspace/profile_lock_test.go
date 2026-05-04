package workspace

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireProfileLockExclusive(t *testing.T) {
	dir := t.TempDir()

	first, err := AcquireProfileLock(dir)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Release()

	if _, err := AcquireProfileLock(dir); err == nil {
		t.Fatal("second acquire should have failed while first holds the lock")
	}
}

func TestProfileLockReleaseAllowsReacquire(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireProfileLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	first.Release()

	second, err := AcquireProfileLock(dir)
	if err != nil {
		t.Fatalf("expected to reacquire after release, got %v", err)
	}
	second.Release()
}

func TestProfileLockConcurrentSingleWinner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "concurrent")

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var winners int64
	start := make(chan struct{})
	acquired := make(chan struct{}, 1)
	release := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			lock, err := AcquireProfileLock(dir)
			if err != nil {
				return
			}
			if atomic.AddInt64(&winners, 1) == 1 {
				acquired <- struct{}{}
			}
			<-release
			lock.Release()
		}()
	}
	close(start)
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		close(release)
		wg.Wait()
		t.Fatal("expected one goroutine to acquire the profile lock")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if winners != 1 {
		t.Fatalf("expected exactly one winner, got %d", winners)
	}
}
