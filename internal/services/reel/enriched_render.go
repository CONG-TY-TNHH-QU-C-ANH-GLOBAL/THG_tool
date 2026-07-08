package reel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	// signedURLTTL is how long the source/avatar signed URLs handed to the
	// transcriber and composer stay valid — long enough for a slow render.
	signedURLTTL = 2 * time.Hour
	// renderLeaseTTL marks a claimed render for orphan detection.
	renderLeaseTTL = 30 * time.Minute
)

// RenderEnriched renders the approved reel: render the avatar (green screen),
// upload it, then compose source + subtitles + keyed avatar into the final
// video. Gated on the latest script being approved. The money invariant is
// enforced by ClaimRender: only the first call proceeds to paid rendering; a
// retry finds the slot taken and returns nil without a second charge.
func (s *EnrichedService) RenderEnriched(ctx context.Context, orgID, reelID int64) error {
	script, err := s.approvedScript(ctx, orgID, reelID)
	if err != nil {
		return err
	}
	enr, err := s.store.GetEnriched(ctx, orgID, reelID)
	if err != nil {
		return notFoundAs(err, ErrReelNotFound)
	}

	claimed, err := s.store.ClaimRender(ctx, orgID, reelID, renderKey(orgID, reelID), time.Now().Add(renderLeaseTTL))
	if err != nil {
		return err
	}
	if !claimed {
		// Already rendering or rendered — not an error, just a no-op retry.
		return nil
	}
	if err := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusRendering); err != nil {
		return err
	}

	if err := s.renderAndCompose(ctx, orgID, reelID, enr.SourceKey, script); err != nil {
		renderErr := fmt.Errorf("reel: enriched render failed: %w", err)
		if stErr := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusFailed); stErr != nil {
			return fmt.Errorf("%w (and status write failed: %v)", renderErr, stErr)
		}
		return renderErr
	}
	return s.store.UpdateReelStatus(ctx, orgID, reelID, StatusDone)
}

// renderAndCompose is the paid section: avatar render + upload, then compose.
// Split out so RenderEnriched's claim/gate/status flow stays flat (low
// cognitive complexity) and the error paths share one wrapper.
func (s *EnrichedService) renderAndCompose(ctx context.Context, orgID, reelID int64, sourceKey string, script enrichedContent) error {
	av, err := s.avatar.RenderAvatar(ctx, AvatarReq{Text: script.AvatarScript, VoiceID: s.deps.VoiceID, AvatarID: s.deps.AvatarID})
	if err != nil {
		return fmt.Errorf("avatar render: %w", err)
	}
	avatarKey := avatarKeyFor(orgID, reelID)
	if err := s.putFile(ctx, avatarKey, av.TempPath, av.ContentType); err != nil {
		return err
	}
	if err := s.store.SetAvatarKey(ctx, orgID, reelID, avatarKey); err != nil {
		return err
	}
	if err := s.store.AddCost(ctx, orgID, reelID, av.CostUSD); err != nil {
		return err
	}

	srcURL, err := s.obj.SignedURL(ctx, sourceKey, signedURLTTL)
	if err != nil {
		return fmt.Errorf("sign source: %w", err)
	}
	avURL, err := s.obj.SignedURL(ctx, avatarKey, signedURLTTL)
	if err != nil {
		return fmt.Errorf("sign avatar: %w", err)
	}

	res, err := s.composer.Compose(ctx, ComposeReq{SourceURL: srcURL, AvatarURL: avURL, Subtitles: script.Subtitles, AvatarPos: s.deps.AvatarPos})
	if err != nil {
		return fmt.Errorf("compose: %w", err)
	}
	if err := s.store.SetFinalOutput(ctx, orgID, reelID, res.FinalKey); err != nil {
		return err
	}
	return s.store.AddCost(ctx, orgID, reelID, res.CostUSD)
}

// approvedScript loads and unmarshals the reel's latest script, requiring it
// to be approved before any paid render runs.
func (s *EnrichedService) approvedScript(ctx context.Context, orgID, reelID int64) (enrichedContent, error) {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		return enrichedContent{}, notFoundAs(err, ErrNoScript)
	}
	if !latest.Approved {
		return enrichedContent{}, ErrScriptNotApproved
	}
	var c enrichedContent
	if err := json.Unmarshal([]byte(latest.Content), &c); err != nil {
		return enrichedContent{}, fmt.Errorf("reel: decode script content: %w", err)
	}
	return c, nil
}

// putFile streams a local file into object storage under key.
func (s *EnrichedService) putFile(ctx context.Context, key, path, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if err := s.obj.Put(ctx, key, f, contentType); err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

// GetEnrichedScript reads back the enriched subtitles/avatar-script for a
// reel's latest script version — the read path the server/frontend uses for
// the approval UI (kept here so callers never parse content JSON themselves).
func (s *EnrichedService) GetEnrichedScript(ctx context.Context, orgID, reelID int64) (EnrichedScript, error) {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		return EnrichedScript{}, notFoundAs(err, ErrNoScript)
	}
	var c enrichedContent
	if err := json.Unmarshal([]byte(latest.Content), &c); err != nil {
		return EnrichedScript{}, fmt.Errorf("reel: decode script content: %w", err)
	}
	return EnrichedScript{Subtitles: c.Subtitles, AvatarScript: c.AvatarScript, LangTgt: c.LangTgt}, nil
}

// renderKey / avatarKeyFor produce deterministic object-storage keys and the
// idempotency token. Deterministic so a retry claims the same slot.
func renderKey(orgID, reelID int64) string {
	return fmt.Sprintf("org/%d/reel/%d/render", orgID, reelID)
}

func avatarKeyFor(orgID, reelID int64) string {
	return fmt.Sprintf("org/%d/reel/%d/avatar.mp4", orgID, reelID)
}
