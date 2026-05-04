package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestClaimNextLocalJobIsAtomic(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const total = 5
	for i := 0; i < total; i++ {
		if _, err := db.CreateJob(&models.Job{
			Type: "scrape", Platform: "facebook",
			Target: "g" + string(rune('A'+i)), ExecutionMode: models.ExecutionLocal,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// 16 goroutines race for 5 jobs — exactly 5 must claim, 11 must miss.
	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	claimed := make(map[int64]string)
	var mu sync.Mutex
	misses := 0

	for i := 0; i < goroutines; i++ {
		worker := "w" + string(rune('A'+i))
		go func() {
			defer wg.Done()
			job, err := db.ClaimNextLocalJob(worker)
			if err != nil {
				t.Errorf("claim error: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if job == nil {
				misses++
				return
			}
			if prev, dup := claimed[job.ID]; dup {
				t.Errorf("job %d claimed twice (first by %s, again by %s)", job.ID, prev, worker)
			}
			claimed[job.ID] = worker
		}()
	}
	wg.Wait()

	if len(claimed) != total {
		t.Fatalf("expected exactly %d claims, got %d (misses=%d)", total, len(claimed), misses)
	}
	if misses != goroutines-total {
		t.Fatalf("expected %d misses, got %d", goroutines-total, misses)
	}
}

func TestRecoverStaleLocalJobs(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "jobs-recover.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.CreateJob(&models.Job{
		Type: "scrape", Platform: "facebook", Target: "stuck",
		ExecutionMode: models.ExecutionLocal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimNextLocalJob("w1"); err != nil {
		t.Fatal(err)
	}

	// Backdate claimed_at to simulate a worker that died long ago.
	if _, err := db.db.Exec(`UPDATE jobs SET claimed_at = datetime('now', '-30 minutes') WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	// 10-minute timeout → row should be eligible for recovery.
	n, err := db.RecoverStaleLocalJobs(10 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 recovered, got %d", n)
	}

	// And it should claim again successfully.
	job, err := db.ClaimNextLocalJob("w2")
	if err != nil {
		t.Fatal(err)
	}
	if job == nil || job.ID != id {
		t.Fatalf("expected reclaim of job %d, got %+v", id, job)
	}
}
