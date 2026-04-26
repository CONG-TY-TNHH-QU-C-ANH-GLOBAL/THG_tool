package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Handler processes a single job.
type Handler func(ctx context.Context, job models.Job) error

// Queue manages a concurrent job processing system using Go channels.
type Queue struct {
	mu       sync.RWMutex
	db       *store.Store
	jobs     chan models.Job
	handlers map[models.JobType]Handler
	workers  int
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	running  map[int64]context.CancelFunc // jobID -> cancel
}

// New creates a new job queue.
func New(db *store.Store, workers int) *Queue {
	return &Queue{
		db:       db,
		jobs:     make(chan models.Job, 100),
		handlers: make(map[models.JobType]Handler),
		workers:  workers,
		running:  make(map[int64]context.CancelFunc),
	}
}

// Register registers a handler for a specific job type.
func (q *Queue) Register(jobType models.JobType, handler Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[jobType] = handler
	log.Printf("[Queue] Registered handler for %s", jobType)
}

// Start begins processing jobs with the configured number of workers.
func (q *Queue) Start(ctx context.Context) {
	ctx, q.cancel = context.WithCancel(ctx)

	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
	log.Printf("[Queue] Started %d workers", q.workers)
}

// Stop gracefully shuts down the queue.
func (q *Queue) Stop() {
	if q.cancel != nil {
		q.cancel()
	}
	q.wg.Wait()
	log.Println("[Queue] All workers stopped")
}

// Submit adds a new job to the queue.
// Jobs with ExecutionMode=local are saved to DB only — the local agent polls and executes them.
func (q *Queue) Submit(job models.Job) (int64, error) {
	jobID, err := q.db.CreateJob(&job)
	if err != nil {
		return 0, fmt.Errorf("create job: %w", err)
	}
	job.ID = jobID

	if job.ExecutionMode == models.ExecutionLocal {
		log.Printf("[Queue] Local job %d created: %s %s (waiting for agent)", jobID, job.Type, job.Target)
		return jobID, nil
	}

	select {
	case q.jobs <- job:
		log.Printf("[Queue] Job %d submitted: %s %s", jobID, job.Type, job.Target)
		return jobID, nil
	default:
		return 0, fmt.Errorf("job queue full")
	}
}

// Cancel cancels a running job.
func (q *Queue) Cancel(jobID int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if cancelFn, ok := q.running[jobID]; ok {
		cancelFn()
		delete(q.running, jobID)
		_ = q.db.UpdateJobStatus(jobID, models.JobCanceled, "", "canceled by user")
		return nil
	}
	return fmt.Errorf("job %d not running", jobID)
}

// RunningCount returns the number of currently running jobs.
func (q *Queue) RunningCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.running)
}

func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()
	log.Printf("[Worker-%d] Started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker-%d] Shutting down", id)
			return
		case job := <-q.jobs:
			q.processJob(ctx, id, job)
		}
	}
}

// jobTimeouts defines per-type timeout budgets.
// Comment/inbox jobs need more time (multiple browser ops); scrape jobs are fast.
var jobTimeouts = map[models.JobType]time.Duration{
	models.JobScrapePost:    10 * time.Minute,
	models.JobScrapeComment: 5 * time.Minute,
	models.JobAutoComment:   20 * time.Minute, // 5 comments × ~3 min each
	models.JobAutoInbox:     20 * time.Minute,
}

func timeoutFor(t models.JobType) time.Duration {
	if d, ok := jobTimeouts[t]; ok {
		return d
	}
	return 5 * time.Minute
}

func (q *Queue) processJob(ctx context.Context, workerID int, job models.Job) {
	log.Printf("[Worker-%d] Processing job %d: %s %s", workerID, job.ID, job.Type, job.Target)

	// Mark as running
	_ = q.db.UpdateJobStatus(job.ID, models.JobRunning, "", "")

	// Create cancellable context with per-job-type timeout
	jobCtx, jobCancel := context.WithTimeout(ctx, timeoutFor(job.Type))
	defer jobCancel()

	q.mu.Lock()
	q.running[job.ID] = jobCancel
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		delete(q.running, job.ID)
		q.mu.Unlock()
	}()

	// Find handler
	q.mu.RLock()
	handler, ok := q.handlers[job.Type]
	q.mu.RUnlock()

	if !ok {
		errMsg := fmt.Sprintf("no handler for job type: %s", job.Type)
		_ = q.db.UpdateJobStatus(job.ID, models.JobFailed, "", errMsg)
		log.Printf("[Worker-%d] %s", workerID, errMsg)
		return
	}

	// Execute
	startTime := time.Now()
	if err := handler(jobCtx, job); err != nil {
		_ = q.db.UpdateJobStatus(job.ID, models.JobFailed, "", err.Error())
		log.Printf("[Worker-%d] Job %d failed: %v", workerID, job.ID, err)
	} else {
		duration := time.Since(startTime).Seconds()
		result := fmt.Sprintf(`{"duration_sec": %.1f}`, duration)
		_ = q.db.UpdateJobStatus(job.ID, models.JobDone, result, "")
		log.Printf("[Worker-%d] Job %d completed in %.1fs", workerID, job.ID, duration)
	}
}
