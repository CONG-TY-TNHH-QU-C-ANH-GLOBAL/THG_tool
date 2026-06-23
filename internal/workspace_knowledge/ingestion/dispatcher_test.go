package ingestion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// stubIngestor lets tests script Sync() outcomes.
type stubIngestor struct {
	typ    sources.SourceType
	result SyncResult
	err    error
}

func (s *stubIngestor) Type() sources.SourceType { return s.typ }
func (s *stubIngestor) Sync(_ context.Context, _ *sources.Source, _ AssetWriter) (SyncResult, error) {
	return s.result, s.err
}

// recordingHealth captures every UpdateSourceHealth call.
type recordingHealth struct {
	mu    sync.Mutex
	calls []recordedHealth
}

type recordedHealth struct {
	sourceID       int64
	orgID          int64
	h              sources.Health
	lastAssetCount int
}

func (r *recordingHealth) UpdateSourceHealth(_ context.Context, sourceID, orgID int64, h sources.Health, lastAssetCount int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedHealth{sourceID, orgID, h, lastAssetCount})
	return nil
}

func (r *recordingHealth) last() recordedHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return recordedHealth{}
	}
	return r.calls[len(r.calls)-1]
}

func makeDispatcher(t *testing.T) (*Dispatcher, *recordingHealth, *Registry) {
	t.Helper()
	reg := NewRegistry()
	hr := &recordingHealth{}
	fixed := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	d := &Dispatcher{
		Registry: reg,
		Health:   hr,
		WriterFactory: func(_ *sources.Source) AssetWriter {
			return noopWriter{}
		},
		Clock: func() time.Time { return fixed },
	}
	return d, hr, reg
}

type noopWriter struct{}

func (noopWriter) Write(_ context.Context, _ *assets.Asset) error { return nil }

func TestDispatcher_HealthyAfterSuccessfulSync(t *testing.T) {
	d, hr, reg := makeDispatcher(t)
	reg.Register(&stubIngestor{
		typ:    sources.SourceCSV,
		result: SyncResult{AssetsSeen: 5, AssetsCreated: 3, AssetsUpdated: 2},
	})

	src := &sources.Source{ID: 1, OrgID: 7, Type: sources.SourceCSV}
	res, err := d.Run(context.Background(), src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.AssetsSeen != 5 {
		t.Errorf("AssetsSeen passthrough: got %d", res.AssetsSeen)
	}

	last := hr.last()
	if last.h.Status != sources.HealthHealthy {
		t.Errorf("status: got %q want healthy", last.h.Status)
	}
	if last.lastAssetCount != 5 {
		t.Errorf("last_asset_count: got %d want 5", last.lastAssetCount)
	}
}

func TestDispatcher_StaleAfterRecoverableError(t *testing.T) {
	d, hr, reg := makeDispatcher(t)
	reg.Register(&stubIngestor{
		typ: sources.SourceCSV,
		err: WrapRecoverable(errors.New("network timeout")),
	})

	src := &sources.Source{ID: 1, OrgID: 7, Type: sources.SourceCSV}
	_, err := d.Run(context.Background(), src)
	if err == nil {
		t.Fatal("expected error from recoverable failure")
	}
	last := hr.last()
	if last.h.Status != sources.HealthStale {
		t.Errorf("recoverable error → stale; got %q", last.h.Status)
	}
}

func TestDispatcher_ErrorAfterPermanentFailure(t *testing.T) {
	d, hr, reg := makeDispatcher(t)
	reg.Register(&stubIngestor{
		typ: sources.SourceCSV,
		err: WrapPermanent(errors.New("schema mismatch")),
	})

	src := &sources.Source{ID: 1, OrgID: 7, Type: sources.SourceCSV}
	_, _ = d.Run(context.Background(), src)
	last := hr.last()
	if last.h.Status != sources.HealthError {
		t.Errorf("permanent error → error; got %q", last.h.Status)
	}
}

func TestDispatcher_PartialSuccessSurfacesInMessage(t *testing.T) {
	d, hr, reg := makeDispatcher(t)
	reg.Register(&stubIngestor{
		typ: sources.SourceCSV,
		result: SyncResult{
			AssetsSeen:     8,
			AssetsRejected: 2,
		},
	})

	src := &sources.Source{ID: 1, OrgID: 7, Type: sources.SourceCSV}
	if _, err := d.Run(context.Background(), src); err != nil {
		t.Fatalf("Run: %v", err)
	}
	last := hr.last()
	if last.h.Status != sources.HealthHealthy {
		t.Errorf("partial success is still healthy; got %q", last.h.Status)
	}
	if last.h.Message == "" {
		t.Error("partial success should surface a message")
	}
}

func TestDispatcher_UnregisteredTypeFailsCleanly(t *testing.T) {
	d, hr, _ := makeDispatcher(t)

	src := &sources.Source{ID: 1, OrgID: 7, Type: "not-registered"}
	_, err := d.Run(context.Background(), src)
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
	last := hr.last()
	if last.h.Status != sources.HealthError {
		t.Errorf("unregistered type → error; got %q", last.h.Status)
	}
}

func TestRegistry_DuplicateRegistrationPanics(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubIngestor{typ: sources.SourceCSV})
	defer func() {
		if recover() == nil {
			t.Error("duplicate Register should panic")
		}
	}()
	reg.Register(&stubIngestor{typ: sources.SourceCSV})
}
