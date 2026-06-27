package reel

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// FakeRenderer is the zero-cost render adapter used in dev/CI and for the Postman proof.
// It accepts a shot and returns a deterministic handle WITHOUT calling any provider or
// spending money.
//
// Two modes:
//   - default (NewFakeRenderer): INERT. Completion is driven externally by the render
//     webhook — used by tests/Postman so the flow is deterministic (no background timers).
//   - auto (NewAutoFakeRenderer, gated by REEL_FAKE_AUTOCOMPLETE): DEV-ONLY. After a shot
//     starts, it self-POSTs a signed "done" webhook back to the local server after a short,
//     scene-staggered delay so the full approve→render→publish flow completes smoothly in
//     the dashboard without a real provider. Never enabled in tests or production.
type FakeRenderer struct {
	autoComplete bool
	webhookURL   string
	secret       string
	delay        time.Duration
	client       *http.Client
}

// NewFakeRenderer constructs the inert renderer (completion via webhook).
func NewFakeRenderer() *FakeRenderer { return &FakeRenderer{} }

// NewAutoFakeRenderer constructs the dev auto-completing renderer.
func NewAutoFakeRenderer(webhookURL, secret string) *FakeRenderer {
	return &FakeRenderer{
		autoComplete: true,
		webhookURL:   webhookURL,
		secret:       secret,
		delay:        2 * time.Second,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Name identifies the provider.
func (f *FakeRenderer) Name() string { return "fake" }

// StartRender returns a deterministic provider job id derived from the reel+scene so the
// webhook can match it. No spend, no I/O (auto mode schedules a self-webhook).
func (f *FakeRenderer) StartRender(_ context.Context, req RenderRequest) (RenderHandle, error) {
	jobID := fmt.Sprintf("fake_%d_%d", req.ReelID, req.Scene)
	if f.autoComplete {
		go f.autoPost(req, jobID)
	}
	return RenderHandle{Provider: "fake", ProviderJobID: jobID}, nil
}

// autoPost (dev only) waits a scene-staggered delay, then posts a signed done-webhook so
// shots complete one-by-one (nice progress) and concurrent writers don't pile up.
func (f *FakeRenderer) autoPost(req RenderRequest, jobID string) {
	time.Sleep(f.delay + time.Duration(req.Scene-1)*600*time.Millisecond)
	_ = postRenderWebhook(f.client, f.webhookURL, f.secret, map[string]any{
		"org_id":          req.OrgID,
		"reel_id":         req.ReelID,
		"scene":           req.Scene,
		"provider_job_id": jobID,
		"state":           "done",
		"output_key":      "renders/" + jobID + ".mp4",
		"cost_usd":        0.06,
	})
}
