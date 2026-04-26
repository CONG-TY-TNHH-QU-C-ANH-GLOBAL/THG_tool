package jobs

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

// Scheduler polls the store, claims pending jobs, and dispatches to registered handlers.
// It is the only component permitted to call Store.Claim or Store.Complete/Fail.
type Scheduler struct {
	store        *Store
	registry     *Registry
	workerID     string
	pollInterval time.Duration
	claimTimeout time.Duration
}

func NewScheduler(store *Store, registry *Registry) *Scheduler {
	return &Scheduler{
		store:        store,
		registry:     registry,
		workerID:     uuid.New().String(),
		pollInterval: 500 * time.Millisecond,
		claimTimeout: 5 * time.Minute,
	}
}

// Run starts the scheduler loop. Blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	recoveryTicker := time.NewTicker(s.claimTimeout / 2)
	defer recoveryTicker.Stop()

	pollTicker := time.NewTicker(s.pollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-recoveryTicker.C:
			if err := s.store.RecoverStale(ctx, s.claimTimeout); err != nil {
				log.Printf("scheduler: stale recovery error: %v", err)
			}
		case <-pollTicker.C:
			job, err := s.store.Claim(ctx, s.workerID)
			if err != nil {
				log.Printf("scheduler: claim error: %v", err)
				continue
			}
			if job == nil {
				continue
			}
			go s.execute(ctx, job)
		}
	}
}

func (s *Scheduler) execute(ctx context.Context, job *Job) {
	h, ok := s.registry.Get(job.Intent)
	if !ok {
		if err := s.store.Fail(ctx, job.ID, "no handler registered for intent: "+job.Intent); err != nil {
			log.Printf("scheduler: fail error for job %d: %v", job.ID, err)
		}
		return
	}

	result, err := h.Handle(ctx, job)
	if err != nil {
		if ferr := s.store.Fail(ctx, job.ID, err.Error()); ferr != nil {
			log.Printf("scheduler: fail error for job %d: %v", job.ID, ferr)
		}
		return
	}

	if cerr := s.store.Complete(ctx, job.ID, result); cerr != nil {
		log.Printf("scheduler: complete error for job %d: %v", job.ID, cerr)
	}
}
