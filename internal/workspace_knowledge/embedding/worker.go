package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Worker drives the embedding pipeline. It polls the [PendingStore]
// for assets with embedding_status='pending', batches them, calls
// the [Embedder], and writes the resulting vectors back through the
// store. Failures are tracked per-asset with a retry budget.
//
// Lifecycle:
//
//	w := embedding.NewWorker(store, embedder)
//	go w.Run(ctx)        // production: long-running goroutine
//	w.Tick(ctx)          // tests: drive one batch synchronously
//
// Concurrency on Postgres: SAFE to run multiple Workers against the
// same database. The store-side ListPendingEmbeddings does not lock,
// so workers MAY pick up overlapping batches — the Embedder is
// idempotent for the same input text, and the UPDATE happens on a
// per-row basis. The wasted API call is the only consequence. For
// strict no-overlap, switch ListPendingEmbeddings to a "FOR UPDATE
// SKIP LOCKED" variant in a follow-up.
//
// Concurrency on SQLite: single-Worker only. SQLite's single-writer
// model means parallel workers would serialise on the write anyway.
type Worker struct {
	Store    PendingStore
	Embedder Embedder

	// BatchSize: how many pending rows to process per Tick. Default 32.
	// OpenAI accepts up to 2048 per request but smaller batches reduce
	// blast radius when one bad input poisons a request.
	BatchSize int

	// MaxAttempts: per-row retry budget before status='failed'.
	// Default 3. After failure, operator action is required (call
	// ResetEmbeddingFailures or fix the underlying problem).
	MaxAttempts int

	// PollInterval: how long Run sleeps between empty Ticks. Default 5s.
	// On busy workspaces the loop never sleeps because ticks return
	// non-empty. On idle workspaces this is the wakeup cadence.
	PollInterval time.Duration

	// MetricsRecorder is optional. When non-nil, the worker logs
	// per-batch outcomes (success / failure / latency / token count)
	// via the Layer-7 observability port. nil means "no metrics" —
	// pipeline still works.
	MetricsRecorder MetricsRecorder

	// Clock is overridable for tests. Defaults to time.Now.
	Clock func() time.Time
}

// PendingStore is the narrow store surface the worker depends on.
// Defined here (not on the store side) so the embedding package
// compiles without importing internal/store — keeps the dependency
// arrow pointing the right way. *store.Store satisfies this.
type PendingStore interface {
	ListPendingEmbeddings(ctx context.Context, limit int) ([]Pending, error)
	UpdateEmbeddingSuccess(ctx context.Context, assetID, orgID int64, vector []float32, modelVersion, inputHash string) error
	RecordEmbeddingAttempt(ctx context.Context, assetID, orgID int64, errMsg string, maxAttempts int) error
}

// Pending is the worker-facing projection of a pending asset. We
// re-export this type from the store-side PendingEmbeddingRow shape
// to avoid the embedding package leaking store types.
type Pending struct {
	AssetID     int64
	OrgID       int64
	Title       string
	Description string
	Tags        string // raw JSON
	AssetType   string
	InputHash   string
	Attempts    int
}

// MetricsRecorder is the optional observability hook. *store.Store
// satisfies this via RecordEmbeddingBatch (defined alongside the
// other knowledge_events recording methods).
type MetricsRecorder interface {
	RecordEmbeddingBatch(ctx context.Context, batchSize, succeeded, failed int, durationMs int64, recoverable bool)
}

// NewWorker builds a Worker with sensible defaults. Callers MAY
// overwrite any field on the returned struct before calling Run.
func NewWorker(store PendingStore, embedder Embedder) *Worker {
	return &Worker{
		Store:        store,
		Embedder:     embedder,
		BatchSize:    32,
		MaxAttempts:  3,
		PollInterval: 5 * time.Second,
		Clock:        time.Now,
	}
}

// Tick processes one batch and returns the number of assets it
// touched. Returns (0, nil) when no rows are pending — that's the
// idle case, not an error.
//
// Tick is the unit of test instrumentation. Production code calls
// Run; tests call Tick directly to assert state transitions.
func (w *Worker) Tick(ctx context.Context) (int, error) {
	if w.Store == nil || w.Embedder == nil {
		return 0, fmt.Errorf("embedding worker: not initialised")
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 32
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = 3
	}
	if w.Clock == nil {
		w.Clock = time.Now
	}

	started := w.Clock()
	pending, err := w.Store.ListPendingEmbeddings(ctx, w.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("list pending: %w", err)
	}
	if len(pending) == 0 {
		return 0, nil
	}

	// Build the texts in the EXACT shape the InputHash hook used.
	// Drift here would cause every freshly-embedded row to re-mark
	// pending on the next UpsertKnowledgeAsset — infinite re-embed loop.
	texts := make([]string, len(pending))
	hashes := make([]string, len(pending))
	for i, p := range pending {
		a := pendingToAsset(p)
		texts[i] = BuildInputText(a)
		hashes[i] = InputHash(a)
	}

	vectors, embedErr := w.Embedder.Embed(ctx, texts)
	duration := w.Clock().Sub(started)

	// All-batch failure: mark each row's attempt count up.
	if embedErr != nil {
		recoverable := IsRecoverable(embedErr) || (!IsPermanent(embedErr))
		errMsg := embedErr.Error()
		for _, p := range pending {
			if err := w.Store.RecordEmbeddingAttempt(ctx, p.AssetID, p.OrgID, errMsg, w.MaxAttempts); err != nil {
				log.Printf("[embedding.worker] record attempt org=%d asset=%d: %v", p.OrgID, p.AssetID, err)
			}
		}
		if w.MetricsRecorder != nil {
			w.MetricsRecorder.RecordEmbeddingBatch(ctx, len(pending), 0, len(pending), duration.Milliseconds(), recoverable)
		}
		return len(pending), embedErr
	}

	// Per-row success path. The Embedder contract guarantees
	// len(vectors) == len(texts), but defensive bound-check anyway.
	if len(vectors) != len(pending) {
		return len(pending), fmt.Errorf("embedder returned %d vectors for %d inputs", len(vectors), len(pending))
	}

	succeeded := 0
	failed := 0
	modelVersion := w.Embedder.ModelVersion()
	for i, p := range pending {
		if err := w.Store.UpdateEmbeddingSuccess(ctx, p.AssetID, p.OrgID, vectors[i], modelVersion, hashes[i]); err != nil {
			failed++
			// The vector generated successfully but persistence failed.
			// Treat as a recoverable error — next tick will retry the
			// row (it's still pending). Record the attempt so the row
			// doesn't spin forever.
			_ = w.Store.RecordEmbeddingAttempt(ctx, p.AssetID, p.OrgID, "persist failed: "+err.Error(), w.MaxAttempts)
			continue
		}
		succeeded++
	}
	if w.MetricsRecorder != nil {
		w.MetricsRecorder.RecordEmbeddingBatch(ctx, len(pending), succeeded, failed, duration.Milliseconds(), false)
	}
	return len(pending), nil
}

// Run loops Tick until ctx is cancelled. PollInterval gates only the
// wait between EMPTY ticks — a Tick that processed rows immediately
// fires another. This drains the backlog quickly while keeping the
// idle workspace quiet.
func (w *Worker) Run(ctx context.Context) error {
	if w.PollInterval <= 0 {
		w.PollInterval = 5 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := w.Tick(ctx)
		if err != nil {
			log.Printf("[embedding.worker] tick: %v", err)
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.PollInterval):
			}
		}
	}
}

// pendingToAsset builds an *assets.Asset compatible with BuildInputText
// from the worker-side projection. Only the fields used by the hash
// computation are populated — payload, state, pinned, boost are not
// part of the embedding-input.
func pendingToAsset(p Pending) *assets.Asset {
	a := &assets.Asset{
		ID:          p.AssetID,
		OrgID:       p.OrgID,
		Type:        assets.AssetType(p.AssetType),
		Title:       p.Title,
		Description: p.Description,
	}
	// Tags are persisted as JSON. Parse defensively — a corrupt tags
	// column should NOT crash the embedding worker.
	if p.Tags != "" {
		_ = json.Unmarshal([]byte(p.Tags), &a.Tags)
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	return a
}
