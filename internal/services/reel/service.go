package reel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	reelstore "github.com/thg/scraper/internal/store/reel"
)

// Service orchestrates the Reel Studio v1 workflow: draft -> script ->
// approve -> render. See package doc for the architecture boundary.
type Service struct {
	store    *reelstore.Store
	renderer VideoRenderer
}

// NewService constructs a reel Service. store owns all persistence;
// renderer is the consumer-owned VideoRenderer port (FakeRenderer{} in this
// PR).
func NewService(store *reelstore.Store, renderer VideoRenderer) *Service {
	return &Service{store: store, renderer: renderer}
}

// CreateDraft creates a new reel in 'draft' status. org_id validation
// already lives in reel.Store.CreateReel — not duplicated here.
func (s *Service) CreateDraft(ctx context.Context, orgID, createdBy int64, title, brief string) (int64, error) {
	return s.store.CreateReel(ctx, orgID, title, brief, createdBy)
}

// GenerateScript creates the next version of a reel's script draft with
// deterministic fake content and moves the reel to 'scripting'.
//
// ponytail: nextScriptVersion reads the latest version then CreateScript
// inserts it in a separate statement — a race window between two concurrent
// calls for the same reel. UNIQUE(org_id, reel_id, version) guarantees no
// duplicate/corrupt row can land, just an unhandled unique-violation error
// on the loser. Upgrade path if concurrent generation becomes real: compute
// the version inside CreateScript's own INSERT (e.g. a subquery) or retry
// once on conflict.
func (s *Service) GenerateScript(ctx context.Context, orgID, reelID int64) (*reelstore.Script, error) {
	r, err := s.store.GetReel(ctx, orgID, reelID)
	if err != nil {
		return nil, notFoundAs(err, ErrReelNotFound)
	}

	version, err := s.nextScriptVersion(ctx, orgID, reelID)
	if err != nil {
		return nil, err
	}
	content := fakeScriptContent(r.Title, r.Brief, version)

	scriptID, err := s.store.CreateScript(ctx, orgID, reelID, version, content)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusScripting); err != nil {
		return nil, err
	}

	return &reelstore.Script{ID: scriptID, OrgID: orgID, ReelID: reelID, Version: version, Content: content}, nil
}

// nextScriptVersion returns 1 if the reel has no script yet, else the
// latest version + 1.
func (s *Service) nextScriptVersion(ctx context.Context, orgID, reelID int64) (int, error) {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return latest.Version + 1, nil
}

// fakeScriptContent produces deterministic (non-random, non-time-based)
// fake script JSON for a given title/brief/version — the same inputs
// always produce the same output.
func fakeScriptContent(title, brief string, version int) string {
	return fmt.Sprintf(
		`{"dialogue":"Fake script for %q (brief: %q), v%d","shots":[{"scene":1,"kind":"broll"}]}`,
		title, brief, version,
	)
}
