package reel

import (
	"context"
	"errors"
	"fmt"
)

// ApproveLatestScript approves a reel's latest script version and moves the
// reel to 'approved' in one atomic store call. Cross-org calls fail with
// ErrNoScript because reel.Store.GetLatestScript already returns
// sql.ErrNoRows for a reel_id owned by a different org — tenant isolation is
// PR-R1's guarantee, not reimplemented here.
//
// Approval gates RenderFake, so the approve write and the reel status write
// must be atomic: ApproveScriptAndSetReelStatus wraps them in one Postgres
// transaction (see internal/store/reel/workflow.go), removing the partial
// "script approved but reel un-approved" state before PR-R3 exposes this
// over a public API where a caller cannot just retry in-process.
func (s *Service) ApproveLatestScript(ctx context.Context, orgID, reelID int64) error {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		return notFoundAs(err, ErrNoScript)
	}
	return s.store.ApproveScriptAndSetReelStatus(ctx, orgID, reelID, latest.ID, StatusApproved)
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

	if err := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusDone); err != nil {
		return errors.Join(ErrRenderBookkeepingFailed, err)
	}
	return nil
}
