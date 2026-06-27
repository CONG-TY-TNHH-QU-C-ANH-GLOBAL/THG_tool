package reel

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Per-shot cost estimates (USD). Providers don't return billing on the render path, so these
// are our accrual estimates surfaced as reel.total_cost_usd.
const (
	costAvatarShot = 0.30 // HeyGen talking_head
	costBrollShot  = 0.10 // FAL clip + FPT TTS
	shotRenderTTL  = 12 * time.Minute
	assembleTTL    = 5 * time.Minute // local ffmpeg concat deadline
)

// RenderConfig is the local-assembly config shared with the Service (ffmpeg + media dir).
type RenderConfig struct {
	MediaDir   string // root for downloaded clips + final.mp4 (relative paths re-rooted here)
	FFmpegPath string // ffmpeg binary path/name
	LogoPath   string // brand watermark overlaid top-left on the final reel; empty → none
}

// RealProviderConfig carries the three render-provider credentials + defaults.
type RealProviderConfig struct {
	FALKey        string
	FALVideoModel string
	HeyGenKey     string
	HeyGenAvatar  string
	HeyGenVoice   string
	FPTKey        string
	FPTVoice      string
}

// RealRenderer renders each shot with a real provider, stitches audio locally with ffmpeg,
// then self-POSTs a signed render webhook (mirroring FakeRenderer.autoPost) so the existing
// async state machine advances unchanged. StartRender never blocks; all provider work and
// any failure is reported out-of-band as done/failed.
type RealRenderer struct {
	prov       RealProviderConfig
	mediaDir   string
	ffmpegPath string
	webhookURL string
	secret     string
	http       *http.Client // long-timeout provider/download client
	post       *http.Client // short-timeout webhook client
}

// NewRealRenderer constructs the real render adapter.
func NewRealRenderer(prov RealProviderConfig, rc RenderConfig, webhookURL, secret string) *RealRenderer {
	return &RealRenderer{
		prov:       prov,
		mediaDir:   rc.MediaDir,
		ffmpegPath: rc.FFmpegPath,
		webhookURL: webhookURL,
		secret:     secret,
		http:       &http.Client{Timeout: 10 * time.Minute},
		post:       &http.Client{Timeout: 15 * time.Second},
	}
}

// Name identifies the provider.
func (r *RealRenderer) Name() string { return "real" }

// StartRender returns a deterministic job id immediately and renders the shot in the
// background, exactly like the auto-fake renderer (non-blocking, webhook-reported).
func (r *RealRenderer) StartRender(_ context.Context, req RenderRequest) (RenderHandle, error) {
	jobID := fmt.Sprintf("real_%d_%d", req.ReelID, req.Scene)
	go r.run(req, jobID)
	return RenderHandle{Provider: "real", ProviderJobID: jobID}, nil
}

// run does the provider dance with its own deadline, then self-reports done or failed.
func (r *RealRenderer) run(req RenderRequest, jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), shotRenderTTL)
	defer cancel()

	outputKey, cost, err := r.renderShot(ctx, req, jobID)
	if err != nil {
		log.Printf("[reel] real render shot reel=%d scene=%d failed: %v", req.ReelID, req.Scene, err)
		r.report(req, jobID, "failed", "", 0)
		return
	}
	r.report(req, jobID, "done", outputKey, cost)
}

// renderShot produces one shot's mp4 under mediaDir and returns its MediaDir-relative key
// (so the serve endpoint can safely re-root it) + the cost estimate.
func (r *RealRenderer) renderShot(ctx context.Context, req RenderRequest, jobID string) (string, float64, error) {
	relDir := fmt.Sprintf("reel-%d", req.ReelID)
	shotKey := filepath.ToSlash(filepath.Join(relDir, jobID+".mp4"))
	shotPath := filepath.Join(r.mediaDir, relDir, jobID+".mp4")
	voiceover := r.voiceoverText(req)

	if req.Kind == "talking_head" {
		if err := heygenAvatar(ctx, r.http, r.prov.HeyGenKey, r.prov.HeyGenAvatar, r.prov.HeyGenVoice, voiceover, shotPath); err != nil {
			return "", 0, err
		}
		return shotKey, costAvatarShot, nil
	}

	// broll / product: silent FAL clip + FPT voiceover, muxed locally.
	clipPath := filepath.Join(r.mediaDir, relDir, jobID+"_clip.mp4")
	if err := falVideo(ctx, r.http, r.prov.FALKey, r.prov.FALVideoModel, req.Prompt, clipPath); err != nil {
		return "", 0, err
	}
	audioPath := filepath.Join(r.mediaDir, relDir, jobID+".mp3")
	if err := fptTTS(ctx, r.http, r.prov.FPTKey, r.prov.FPTVoice, voiceover, audioPath); err != nil {
		return "", 0, err
	}
	if err := muxAudio(ctx, r.ffmpegPath, clipPath, audioPath, shotPath); err != nil {
		return "", 0, err
	}
	return shotKey, costBrollShot, nil
}

// voiceoverText prefers the script's Voiceover line and falls back to the visual Prompt so a
// shot always speaks something rather than rendering silent.
func (r *RealRenderer) voiceoverText(req RenderRequest) string {
	if v := strings.TrimSpace(req.Voiceover); v != "" {
		return v
	}
	return strings.TrimSpace(req.Prompt)
}

// report self-POSTs the signed render webhook (shared signer with the fake renderer).
func (r *RealRenderer) report(req RenderRequest, jobID, state, outputKey string, cost float64) {
	err := postRenderWebhook(r.post, r.webhookURL, r.secret, map[string]any{
		"org_id":          req.OrgID,
		"reel_id":         req.ReelID,
		"scene":           req.Scene,
		"provider_job_id": jobID,
		"state":           state,
		"output_key":      outputKey,
		"cost_usd":        cost,
	})
	if err != nil {
		log.Printf("[reel] real render webhook self-report reel=%d scene=%d state=%s failed: %v",
			req.ReelID, req.Scene, state, err)
	}
}
