package reel

import (
	"context"
	"errors"
	"fmt"
)

// ApproveLatestScript approves a reel's latest script version and moves the
// reel to 'approved'. Cross-org calls fail with ErrNoScript because
// reel.Store.GetLatestScript already returns sql.ErrNoRows for a reel_id
// owned by a different org — tenant isolation is PR-R1's guarantee, not
// reimplemented here.
func (s *Service) ApproveLatestScript(ctx context.Context, orgID, reelID int64) error {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		return notFoundAs(err, ErrNoScript)
	}
	if err := s.store.ApproveScript(ctx, orgID, latest.ID); err != nil {
		return err
	}
	return s.store.UpdateReelStatus(ctx, orgID, reelID, StatusApproved)
}

// RenderFake requires the reel's latest script to be approved, runs it
// through the injected VideoRenderer, and moves the reel to 'done' (or
// 'failed' if the renderer errors).
func (s *Service) RenderFake(ctx context.Context, orgID, reelID int64) error {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		return notFoundAs(err, ErrNoScript)
	}
	if !latest.Approved {
		return ErrScriptNotApproved
	}

	req := RenderRequest{OrgID: orgID, ReelID: reelID, Script: latest.Content}
	if err := s.renderer.Render(ctx, req); err != nil {
		renderErr := fmt.Errorf("reel: render failed: %w", err)
		if statusErr := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusFailed); statusErr != nil {
			return errors.Join(renderErr, statusErr)
		}
		return renderErr
	}

	return s.store.UpdateReelStatus(ctx, orgID, reelID, StatusDone)
}
