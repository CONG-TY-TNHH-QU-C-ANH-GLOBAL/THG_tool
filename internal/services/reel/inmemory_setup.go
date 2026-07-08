package reel

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	reelstore "github.com/thg/scraper/internal/store/reel"
)

// Setup/read helpers on InMemoryStore that tests and the local PoC use to
// drive state — these are NOT part of the EnrichedStore port (the service
// never calls them). Kept in a separate file from the port implementation so
// each file stays a single concern and under the size limit.

// CreateReel inserts a draft reel (reel_type defaults to 'enriched').
func (m *InMemoryStore) CreateReel(_ context.Context, orgID int64, title, brief string, createdBy int64) (int64, error) {
	if orgID <= 0 {
		return 0, fmt.Errorf("reel: org_id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.id()
	now := time.Now().UTC()
	m.reels[key{orgID, id}] = &memReel{
		r:   reelstore.Reel{ID: id, OrgID: orgID, Title: title, Brief: brief, Status: StatusDraft, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now},
		enr: reelstore.Enriched{ReelType: "enriched"},
	}
	return id, nil
}

// GetReel returns the reel or sql.ErrNoRows.
func (m *InMemoryStore) GetReel(_ context.Context, orgID, reelID int64) (*reelstore.Reel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mr, ok := m.reels[key{orgID, reelID}]
	if !ok {
		return nil, sql.ErrNoRows
	}
	r := mr.r
	return &r, nil
}

// ApproveScript marks a script (by id, within org) approved.
func (m *InMemoryStore) ApproveScript(_ context.Context, orgID, scriptID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, list := range m.scripts {
		for _, sc := range list {
			if sc.ID == scriptID && sc.OrgID == orgID {
				sc.Approved = true
			}
		}
	}
	return nil
}

// GetLatestTranscript returns the most recent transcript or sql.ErrNoRows.
func (m *InMemoryStore) GetLatestTranscript(_ context.Context, orgID, reelID int64) (*reelstore.Transcript, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.transcripts[key{orgID, reelID}]
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	tr := *list[len(list)-1]
	return &tr, nil
}
