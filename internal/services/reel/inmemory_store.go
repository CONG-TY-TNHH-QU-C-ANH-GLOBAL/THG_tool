package reel

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	reelstore "github.com/thg/scraper/internal/store/reel"
)

// InMemoryStore is a hermetic, dependency-free EnrichedStore double for
// local runs and tests. It mirrors the Postgres store's org-scoping,
// sql.ErrNoRows semantics, version increment, composite-FK rejection, and
// the ClaimRender CAS (money invariant) — WITHOUT a database. It is NOT a
// production store and introduces no SQLite reel schema; production uses the
// Postgres *reelstore.Store, which satisfies EnrichedStore.
type InMemoryStore struct {
	mu          sync.Mutex
	nextID      int64
	reels       map[key]*memReel
	scripts     map[key][]*reelstore.Script
	transcripts map[key][]*reelstore.Transcript
}

type key struct{ org, reel int64 }

type memReel struct {
	r         reelstore.Reel
	enr       reelstore.Enriched
	renderKey string
}

// NewInMemoryStore returns an empty in-memory reel store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		reels:       map[key]*memReel{},
		scripts:     map[key][]*reelstore.Script{},
		transcripts: map[key][]*reelstore.Transcript{},
	}
}

func (m *InMemoryStore) id() int64 { m.nextID++; return m.nextID }

// --- EnrichedStore implementation ---

func (m *InMemoryStore) GetEnriched(_ context.Context, orgID, reelID int64) (*reelstore.Enriched, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mr, ok := m.reels[key{orgID, reelID}]
	if !ok {
		return nil, sql.ErrNoRows
	}
	e := mr.enr
	return &e, nil
}

func (m *InMemoryStore) SetSource(_ context.Context, orgID, reelID int64, sourceKey, inputBranch string) error {
	return m.mutate(orgID, reelID, func(mr *memReel) {
		mr.enr.SourceKey = sourceKey
		mr.enr.InputBranch = inputBranch
	})
}

func (m *InMemoryStore) SetAvatarKey(_ context.Context, orgID, reelID int64, avatarKey string) error {
	return m.mutate(orgID, reelID, func(mr *memReel) { mr.enr.AvatarKey = avatarKey })
}

func (m *InMemoryStore) SetFinalOutput(_ context.Context, orgID, reelID int64, finalKey string) error {
	return m.mutate(orgID, reelID, func(mr *memReel) { mr.enr.FinalOutputKey = finalKey })
}

func (m *InMemoryStore) AddCost(_ context.Context, orgID, reelID int64, deltaUSD float64) error {
	return m.mutate(orgID, reelID, func(mr *memReel) { mr.enr.TotalCostUSD += deltaUSD })
}

func (m *InMemoryStore) UpdateReelStatus(_ context.Context, orgID, reelID int64, status string) error {
	return m.mutate(orgID, reelID, func(mr *memReel) { mr.r.Status = status })
}

// ClaimRender sets the idempotency key only if unset (CAS). Returns false on
// a foreign/absent reel or an already-claimed one — the money invariant.
func (m *InMemoryStore) ClaimRender(_ context.Context, orgID, reelID int64, k string, _ time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mr, ok := m.reels[key{orgID, reelID}]
	if !ok || mr.renderKey != "" {
		return false, nil
	}
	mr.renderKey = k
	mr.enr.RenderIdempotencyKey = k
	return true, nil
}

func (m *InMemoryStore) CreateTranscript(_ context.Context, orgID, reelID int64, in reelstore.TranscriptInput) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reels[key{orgID, reelID}]; !ok {
		return 0, fmt.Errorf("reel: transcript FK violation (org=%d reel=%d)", orgID, reelID)
	}
	id := m.id()
	k := key{orgID, reelID}
	m.transcripts[k] = append(m.transcripts[k], &reelstore.Transcript{
		ID: id, OrgID: orgID, ReelID: reelID, Segments: in.Segments,
		LangSrc: in.LangSrc, LangTgt: in.LangTgt, Source: in.Source, CostUSD: in.CostUSD, CreatedAt: time.Now().UTC(),
	})
	return id, nil
}

func (m *InMemoryStore) GetLatestScript(_ context.Context, orgID, reelID int64) (*reelstore.Script, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.scripts[key{orgID, reelID}]
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	sc := *list[len(list)-1]
	return &sc, nil
}

func (m *InMemoryStore) CreateScript(_ context.Context, orgID, reelID int64, version int, content string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reels[key{orgID, reelID}]; !ok {
		return 0, fmt.Errorf("reel: script FK violation (org=%d reel=%d)", orgID, reelID)
	}
	id := m.id()
	k := key{orgID, reelID}
	m.scripts[k] = append(m.scripts[k], &reelstore.Script{
		ID: id, OrgID: orgID, ReelID: reelID, Version: version, Content: content, CreatedAt: time.Now().UTC(),
	})
	return id, nil
}

// mutate applies fn to an existing reel; a missing reel is a silent no-op,
// matching the Postgres UPDATE ... WHERE returning 0 rows.
func (m *InMemoryStore) mutate(orgID, reelID int64, fn func(*memReel)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mr, ok := m.reels[key{orgID, reelID}]; ok {
		fn(mr)
		mr.r.UpdatedAt = time.Now().UTC()
	}
	return nil
}

// Compile-time proof the in-memory store satisfies the port.
var _ EnrichedStore = (*InMemoryStore)(nil)
