package ingestion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Registry maps SourceType → Ingestor. Concrete ingestors register
// themselves at boot time; the dispatcher looks one up per Sync call.
//
// Registry is safe for concurrent reads; writes happen at boot only.
// Re-registering an existing type panics — having two implementations
// for the same type is a programmer error, not a runtime condition.
type Registry struct {
	mu sync.RWMutex
	by map[sources.SourceType]Ingestor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{by: map[sources.SourceType]Ingestor{}}
}

// Register adds one ingestor. Panics if its type is already registered.
func (r *Registry) Register(i Ingestor) {
	if i == nil {
		panic("ingestion: Register(nil)")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := i.Type()
	if _, exists := r.by[t]; exists {
		panic(fmt.Sprintf("ingestion: duplicate ingestor for type %q", t))
	}
	r.by[t] = i
}

// Lookup returns the ingestor for typ or (nil, false). The boolean
// is the explicit "not registered" branch — per
// feedback_no_implicit_business_meaning.md, callers never compare
// against nil to decide registration.
func (r *Registry) Lookup(typ sources.SourceType) (Ingestor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	i, ok := r.by[typ]
	return i, ok
}

// Types returns every SourceType the registry can handle. Used by
// boot-time validation and by handlers that need to filter the
// operator's "add source" picker to known types.
func (r *Registry) Types() []sources.SourceType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]sources.SourceType, 0, len(r.by))
	for t := range r.by {
		out = append(out, t)
	}
	return out
}

// HealthRecorder is the dispatcher's dependency on the persistence
// side. Defined here as an interface so the dispatcher does not import
// internal/store directly — *store.Store satisfies this contract.
type HealthRecorder interface {
	UpdateKnowledgeSourceHealth(ctx context.Context, sourceID, orgID int64, h sources.Health, lastAssetCount int) error
}

// Dispatcher runs Sync for one source: look up the ingestor, hand it
// an AssetWriter bound to the source, record the outcome on the source
// row, and translate the SyncResult into a health status.
//
// The dispatcher is stateless — instances can be reused across syncs
// and across orgs. Concurrency is the caller's responsibility (the
// scheduler may run N syncs in parallel; each holds its own context).
type Dispatcher struct {
	Registry      *Registry
	Health        HealthRecorder
	WriterFactory func(src *sources.Source) AssetWriter // typically NewStoreAssetWriter bound to a Store
	// Metrics is optional. The dispatcher only records sync events when
	// it is non-nil; the no-op observability.NoOp also satisfies this
	// dependency for callers that want to wire it explicitly.
	Metrics SyncRecorder
	// Clock is overridable for tests so we can assert on LastSyncAt
	// without time.Now flakiness. Defaults to time.Now.
	Clock func() time.Time
}

// SyncRecorder is the subset of observability.Metrics the dispatcher
// needs. Defined here to avoid the import cycle (observability
// imports retrieval which imports assets — and the dispatcher does
// too, so a separate narrow interface keeps the graph clean).
type SyncRecorder interface {
	RecordSync(ctx context.Context, orgID int64, sourceType sources.SourceType, assetsSeen, assetsCreated, assetsUpdated, assetsRejected int, durationMs int64, errs int)
}

// Run pulls fresh data for one source. Reports the outcome on the
// source row via Dispatcher.Health and returns the same SyncResult
// the ingestor produced.
//
// Errors from the ingestor are translated into the source's
// health_status: recoverable → stale (re-try later), permanent →
// error (operator action required). Auth errors are surfaced as
// needs_auth when the ingestor wraps them appropriately — see the
// docs on individual ingestors.
func (d *Dispatcher) Run(ctx context.Context, src *sources.Source) (SyncResult, error) {
	if d.Registry == nil || d.Health == nil || d.WriterFactory == nil {
		return SyncResult{}, errors.New("ingestion: dispatcher not initialised")
	}
	if src == nil || src.OrgID <= 0 || src.ID <= 0 {
		return SyncResult{}, errors.New("ingestion: source is missing OrgID or ID")
	}
	ingestor, ok := d.Registry.Lookup(src.Type)
	if !ok {
		// Permanent: operator must change the source type or wait for
		// the connector to be implemented.
		now := d.now()
		h := sources.Health{
			Status:     sources.HealthError,
			Message:    fmt.Sprintf("no ingestor registered for type %q", src.Type),
			LastSyncAt: &now,
		}
		_ = d.Health.UpdateKnowledgeSourceHealth(ctx, src.ID, src.OrgID, h, src.LastAssetCount)
		return SyncResult{}, fmt.Errorf("ingestion: no ingestor for type %q", src.Type)
	}

	// Mark the source as syncing for the duration of the call. This
	// is a fact: any concurrent reader sees the in-flight state and
	// the UI animates a spinner. We swallow the marker-write error —
	// failing to mark "syncing" is not worth aborting the actual sync.
	beforeMsg := src.Health.Message
	_ = d.Health.UpdateKnowledgeSourceHealth(ctx, src.ID, src.OrgID, sources.Health{
		Status:     sources.HealthSyncing,
		Message:    beforeMsg,
		LastSyncAt: src.Health.LastSyncAt,
	}, src.LastAssetCount)

	w := d.WriterFactory(src)
	started := d.now()
	res, syncErr := ingestor.Sync(ctx, src, w)
	duration := d.now().Sub(started)

	// Decide the post-sync health.
	finalHealth := healthFromOutcome(res, syncErr, d.now())
	if err := d.Health.UpdateKnowledgeSourceHealth(ctx, src.ID, src.OrgID, finalHealth, res.AssetsSeen); err != nil {
		// Persisting health is best-effort; the sync itself happened.
		// We surface the persistence failure in the returned error
		// so the scheduler can decide whether to alert.
		if syncErr == nil {
			syncErr = fmt.Errorf("ingestion: record health: %w", err)
		}
	}

	if d.Metrics != nil {
		errCount := len(res.Errors)
		if syncErr != nil {
			errCount++
		}
		d.Metrics.RecordSync(ctx, src.OrgID, src.Type,
			res.AssetsSeen, res.AssetsCreated, res.AssetsUpdated, res.AssetsRejected,
			duration.Milliseconds(), errCount,
		)
	}
	return res, syncErr
}

// healthFromOutcome is the single decision point for "what health does
// this sync result imply?". Kept as a pure function so it is testable
// in isolation. The mapping:
//
//	syncErr nil      → healthy (or stale if partial errors)
//	syncErr recoverable → stale (re-try later)
//	syncErr permanent   → error (operator action required)
func healthFromOutcome(res SyncResult, err error, now time.Time) sources.Health {
	at := now
	if err == nil {
		// Success — but if some rows were rejected, that's a partial
		// success the operator should see. We surface it as the message
		// rather than as a status change.
		h := sources.Health{Status: sources.HealthHealthy, LastSyncAt: &at}
		if res.AssetsRejected > 0 {
			h.Message = fmt.Sprintf("%d of %d rows rejected during sync",
				res.AssetsRejected, res.AssetsSeen+res.AssetsRejected)
		}
		return h
	}
	if IsRecoverable(err) {
		return sources.Health{
			Status:     sources.HealthStale,
			Message:    "transient: " + truncate(err.Error(), 240),
			LastSyncAt: &at,
		}
	}
	// Permanent or unwrapped — treat as a hard error.
	return sources.Health{
		Status:     sources.HealthError,
		Message:    truncate(err.Error(), 240),
		LastSyncAt: &at,
	}
}

func (d *Dispatcher) now() time.Time {
	if d.Clock != nil {
		return d.Clock()
	}
	return time.Now()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
