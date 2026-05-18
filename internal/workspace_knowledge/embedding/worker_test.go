package embedding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockEmbedder is a controllable [Embedder] for state-machine tests.
// Returns deterministic vectors (one-hot at the input length) so test
// assertions can inspect what got stored.
type mockEmbedder struct {
	mu            sync.Mutex
	calls         int
	err           error
	dimensions    int
	modelVersion  string
}

func newMockEmbedder() *mockEmbedder {
	return &mockEmbedder{dimensions: 8, modelVersion: "mock:v1"}
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.calls++
	err := m.err
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, m.dimensions)
		v[0] = float32(i + 1)
		out[i] = v
	}
	return out, nil
}

func (m *mockEmbedder) ModelVersion() string { return m.modelVersion }
func (m *mockEmbedder) Dimensions() int      { return m.dimensions }

// recordingStore captures every Worker callback so tests assert on
// the transition sequence without standing up a real DB.
type recordingStore struct {
	mu            sync.Mutex
	pending       []Pending
	succeeded     []succeededCall
	attempts      []attemptCall
	pendingCalls  int
}
type succeededCall struct {
	AssetID, OrgID int64
	Vector         []float32
	ModelVersion   string
	InputHash      string
}
type attemptCall struct {
	AssetID, OrgID int64
	ErrMsg         string
	MaxAttempts    int
}

func (r *recordingStore) ListPendingEmbeddings(_ context.Context, _ int) ([]Pending, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingCalls++
	out := r.pending
	r.pending = nil // simulate "row moved out of pending" after worker processes
	return out, nil
}
func (r *recordingStore) UpdateEmbeddingSuccess(_ context.Context, aid, oid int64, v []float32, mv, h string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.succeeded = append(r.succeeded, succeededCall{aid, oid, v, mv, h})
	return nil
}
func (r *recordingStore) RecordEmbeddingAttempt(_ context.Context, aid, oid int64, msg string, max int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts = append(r.attempts, attemptCall{aid, oid, msg, max})
	return nil
}

func TestWorker_HappyPath_PendingToGenerated(t *testing.T) {
	store := &recordingStore{
		pending: []Pending{
			{AssetID: 1, OrgID: 7, Title: "Cat Tee", Description: "premium", Tags: `["cat","tee"]`, AssetType: "POD_product"},
			{AssetID: 2, OrgID: 7, Title: "Dog Mug", Description: "ceramic", Tags: `["dog","mug"]`, AssetType: "POD_product"},
		},
	}
	emb := newMockEmbedder()
	w := NewWorker(store, emb)
	w.Clock = func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }

	n, err := w.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n != 2 {
		t.Errorf("processed count: got %d want 2", n)
	}
	if len(store.succeeded) != 2 {
		t.Errorf("succeeded count: got %d want 2", len(store.succeeded))
	}
	if len(store.attempts) != 0 {
		t.Errorf("happy path should have 0 attempt records; got %d", len(store.attempts))
	}
	for i, s := range store.succeeded {
		if s.ModelVersion != "mock:v1" {
			t.Errorf("succeeded[%d].ModelVersion: got %q", i, s.ModelVersion)
		}
		if len(s.Vector) != 8 {
			t.Errorf("succeeded[%d] vector len: got %d", i, len(s.Vector))
		}
		if s.InputHash == "" {
			t.Errorf("succeeded[%d] InputHash should be set", i)
		}
	}
}

// Recoverable error: worker calls RecordEmbeddingAttempt for every
// asset in the batch (not UpdateEmbeddingSuccess). The error stays
// recoverable so RecordEmbeddingAttempt's max-attempts logic decides
// when to flip to 'failed'.
func TestWorker_RecoverableError_RecordsAttempts(t *testing.T) {
	store := &recordingStore{
		pending: []Pending{
			{AssetID: 1, OrgID: 7, Title: "X", AssetType: "POD_product"},
		},
	}
	emb := newMockEmbedder()
	emb.err = WrapRecoverable(errors.New("rate limited"))
	w := NewWorker(store, emb)

	_, err := w.Tick(context.Background())
	if err == nil {
		t.Fatal("expected error from recoverable embedder")
	}
	if !IsRecoverable(err) {
		t.Errorf("error should be recoverable; got %v", err)
	}
	if len(store.succeeded) != 0 {
		t.Error("recoverable error must not record any successes")
	}
	if len(store.attempts) != 1 {
		t.Errorf("expected 1 attempt record; got %d", len(store.attempts))
	}
}

// Permanent error: same shape but the error metadata marks it
// permanent. The worker still records the attempt; the
// RecordEmbeddingAttempt logic decides what status to write.
func TestWorker_PermanentError_RecordsAttempts(t *testing.T) {
	store := &recordingStore{
		pending: []Pending{{AssetID: 1, OrgID: 7, Title: "X", AssetType: "POD_product"}},
	}
	emb := newMockEmbedder()
	emb.err = WrapPermanent(errors.New("invalid API key"))
	w := NewWorker(store, emb)

	_, err := w.Tick(context.Background())
	if err == nil {
		t.Fatal("expected error from permanent embedder")
	}
	if !IsPermanent(err) {
		t.Errorf("error should be permanent; got %v", err)
	}
	if len(store.attempts) != 1 {
		t.Errorf("expected 1 attempt; got %d", len(store.attempts))
	}
}

// Idle workspace: no pending rows → Tick returns (0, nil) cleanly.
func TestWorker_IdleReturnsZero(t *testing.T) {
	store := &recordingStore{} // no pending
	w := NewWorker(store, newMockEmbedder())
	n, err := w.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n != 0 {
		t.Errorf("idle worker should process 0; got %d", n)
	}
}

// BuildInputText / InputHash determinism: same asset content always
// produces the same hash. This is the load-bearing invariant that
// makes the change-detection hook work.
func TestInputHash_Deterministic(t *testing.T) {
	a1 := &mockAssetForHash{title: "Cat Tee", desc: "premium", tags: []string{"cat", "tee"}, typ: "POD_product"}
	a2 := &mockAssetForHash{title: "Cat Tee", desc: "premium", tags: []string{"tee", "cat"}, typ: "POD_product"} // tags reordered
	a3 := &mockAssetForHash{title: "Cat Tee", desc: "different", tags: []string{"cat", "tee"}, typ: "POD_product"}

	h1 := hashFromMock(a1)
	h2 := hashFromMock(a2)
	h3 := hashFromMock(a3)

	if h1 != h2 {
		t.Errorf("tag reorder must not change hash; got %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("description change must change hash; both = %q", h1)
	}
}

// MetricsRecorder: when wired, Tick emits one batch record.
func TestWorker_MetricsRecorderFires(t *testing.T) {
	store := &recordingStore{
		pending: []Pending{{AssetID: 1, OrgID: 7, Title: "X", AssetType: "POD_product"}},
	}
	rec := &mockMetricsRecorder{}
	w := NewWorker(store, newMockEmbedder())
	w.MetricsRecorder = rec
	if _, err := w.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if rec.batches != 1 {
		t.Errorf("expected 1 metrics call; got %d", rec.batches)
	}
}

// --- test helpers ---

type mockMetricsRecorder struct {
	batches int
}

func (m *mockMetricsRecorder) RecordEmbeddingBatch(_ context.Context, _, _, _ int, _ int64, _ bool) {
	m.batches++
}

type mockAssetForHash struct {
	title, desc string
	tags        []string
	typ         string
}

func hashFromMock(m *mockAssetForHash) string {
	// Build a minimal Asset for hashing via the public API.
	a := pendingToAsset(Pending{
		Title:       m.title,
		Description: m.desc,
		AssetType:   m.typ,
	})
	a.Tags = m.tags
	return InputHash(a)
}
