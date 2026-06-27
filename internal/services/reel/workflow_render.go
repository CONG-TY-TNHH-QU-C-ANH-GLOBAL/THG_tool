package reel

import (
	"context"
	"fmt"

	"github.com/thg/scraper/internal/models"
	reelstore "github.com/thg/scraper/internal/store/reel"
)

// RenderResult is the render webhook payload (one shot). OrgID is carried in the payload
// because the webhook is system-authenticated (HMAC), not user-scoped — the org cannot be
// derived from a JWT. State is "done" or "failed".
type RenderResult struct {
	OrgID         int64   `json:"org_id"`
	ReelID        int64   `json:"reel_id"`
	Scene         int64   `json:"scene"`
	ProviderJobID string  `json:"provider_job_id"`
	State         string  `json:"state"`
	OutputKey     string  `json:"output_key"`
	CostUSD       float64 `json:"cost_usd"`
}

// HandleRenderResult applies one shot's render outcome. It is idempotent: a redelivered
// "done" for an already-done shot adds NO cost (MarkShotDone only wins once). When every
// shot is done the reel advances render_done → assembled via assembleFinal (real ffmpeg
// concat when media is configured, else a synthetic placeholder for the fake/CI path).
func (s *Service) HandleRenderResult(_ context.Context, in RenderResult) error {
	if in.OrgID <= 0 || in.ReelID <= 0 || in.ProviderJobID == "" {
		return fmt.Errorf("reel: org_id, reel_id, provider_job_id required")
	}
	if in.State == reelstore.ShotFailed {
		return s.db.Reel().MarkShotFailed(in.OrgID, in.ReelID, in.ProviderJobID)
	}
	applied, err := s.db.Reel().MarkShotDone(in.OrgID, in.ReelID, in.ProviderJobID, in.OutputKey, in.CostUSD)
	if err != nil {
		return err
	}
	if applied && in.CostUSD != 0 {
		if err := s.db.Reel().AddCost(in.OrgID, in.ReelID, in.CostUSD); err != nil {
			return err
		}
	}
	total, done, err := s.db.Reel().CountShots(in.OrgID, in.ReelID)
	if err != nil {
		return err
	}
	if total > 0 && done == total {
		if err := s.db.Reel().UpdateReelStatus(in.OrgID, in.ReelID, reelstore.StatusRenderDone); err != nil {
			return err
		}
		return s.assembleFinal(in.OrgID, in.ReelID)
	}
	return nil
}

// PublishInput is the publish command. AccountID + TargetURL identify where the reel posts.
type PublishInput struct {
	AccountID int64
	TargetURL string
	CreatedBy int64
}

// PublishResult reports the queued outbound id (0 when the policy gate blocked it).
type PublishResult struct {
	OutboundID int64  `json:"outbound_id"`
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
}

// Publish queues the finished reel as a post_reel action through the outbound spine. The
// reel must be assembled. The video rides MediaPath/MediaType=video; the caption rides
// Content. Reuses PolicyGate → Claim → Connector → Ledger — no new posting path.
func (s *Service) Publish(_ context.Context, orgID, reelID int64, in PublishInput) (*PublishResult, error) {
	r, err := s.db.Reel().GetReel(orgID, reelID)
	if err != nil {
		return nil, err
	}
	if r.Status != reelstore.StatusAssembled {
		return nil, fmt.Errorf("reel: not ready to publish (status=%s)", r.Status)
	}
	if r.FinalOutputKey == "" {
		return nil, fmt.Errorf("reel: missing rendered video")
	}
	caption := ""
	if sc, _ := s.db.Reel().GetLatestScript(orgID, reelID); sc != nil {
		caption = sc.Caption
	}
	msg := &models.OutboundMessage{
		OrgID:     orgID,
		Type:      "post_reel",
		Platform:  models.PlatformFacebook,
		AccountID: in.AccountID,
		TargetURL: in.TargetURL,
		Content:   caption,
		MediaPath: r.FinalOutputKey,
		MediaType: "video",
		CreatedBy: in.CreatedBy,
	}
	res, err := s.db.Outbound().Queue(msg, 0)
	if err != nil {
		return nil, err
	}
	if !res.Decision.Allowed {
		return &PublishResult{Allowed: false, Reason: res.Decision.Reason}, nil
	}
	if err := s.db.Reel().UpdateReelStatus(orgID, reelID, reelstore.StatusPosting); err != nil {
		return nil, err
	}
	return &PublishResult{OutboundID: res.ID, Allowed: true, Reason: "ok"}, nil
}

// Get returns a reel with its latest script and shots (GET /api/reels/:id).
func (s *Service) Get(orgID, reelID int64) (*Result, error) {
	return s.load(orgID, reelID, true)
}

// ListReels returns an org's reels newest-first.
func (s *Service) ListReels(orgID int64, limit int) ([]reelstore.Reel, error) {
	return s.db.Reel().ListReels(orgID, limit)
}

// Progress summarises shot completion + accrued cost for GET /api/reels/:id.
type Progress struct {
	ShotsTotal   int     `json:"shots_total"`
	ShotsDone    int     `json:"shots_done"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// Progress returns shot counts + cost for a reel.
func (s *Service) GetProgress(orgID, reelID int64) (*Progress, error) {
	total, done, err := s.db.Reel().CountShots(orgID, reelID)
	if err != nil {
		return nil, err
	}
	r, err := s.db.Reel().GetReel(orgID, reelID)
	if err != nil {
		return nil, err
	}
	return &Progress{ShotsTotal: total, ShotsDone: done, TotalCostUSD: r.TotalCostUSD}, nil
}
