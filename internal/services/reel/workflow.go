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
//
// ponytail: ApproveScript and UpdateReelStatus below are two separate
// statements, not one transaction — reel.Store has no transaction-spanning
// primitive today (the one BeginTx example in the store layer,
// internal/store/knowledge/sources.go, is a single Store method's own
// intra-domain cascade, not a cross-call seam a Service could reuse; adding
// one would be new infrastructure, out of scope for this PR). If
// ApproveScript succeeds but UpdateReelStatus fails, the script is
// genuinely approved and reels.status just lags — harmless, because
// RenderFake below gates on the script's own Approved flag, never on
// reels.status. Same reasoning applies to GenerateScript's GetReel ->
// CreateScript -> UpdateReelStatus sequence. Revisit before PR-R3 exposes
// this over a public API (where a caller can no longer just retry), or in
// a dedicated store/service transactionality PR.
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

	if err := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusDone); err != nil {
		return errors.Join(ErrRenderBookkeepingFailed, err)
	}
	return nil
}
