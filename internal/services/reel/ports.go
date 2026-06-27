package reel

import "context"

// VideoRenderer is the consumer-owned port for an external short-video render provider.
// StartRender commits spend for ONE shot and returns a handle whose ProviderJobID is the
// key the webhook echoes back. Implementations MUST be idempotent on IdempotencyKey so a
// retry of the same shot does not double-charge. Render completion is reported out-of-band
// via the render webhook (HandleRenderResult), never synchronously here.
type VideoRenderer interface {
	StartRender(ctx context.Context, req RenderRequest) (RenderHandle, error)
	// Name identifies the provider for diagnostics and shot.provider stamping.
	Name() string
}

// RenderRequest is one shot's render job.
type RenderRequest struct {
	OrgID          int64
	ReelID         int64
	Scene          int64
	Kind           string // broll|product|talking_head
	Prompt         string // visual prompt for the clip
	Voiceover      string // Vietnamese line the TTS/avatar speaks (may be empty)
	DurationSec    int
	IdempotencyKey string // reel-scoped; provider must dedupe on this
}

// RenderHandle is what a provider returns after accepting a shot.
type RenderHandle struct {
	Provider      string
	ProviderJobID string
}

// ScriptInput is the grounded brief the script engine turns into a shot list.
type ScriptInput struct {
	OrgID          int64
	BriefStyle     string
	Keywords       []string
	TargetDuration int
	BusinessBlock  string // grounding: profile.ToPromptBlock() output, may be empty
	MarketingGuide string // brand marketing playbook (tone/voice/hashtags), may be empty
}

// ScriptDraft is the engine's structured output, persisted into reel_scripts.
type ScriptDraft struct {
	Dialogue    string `json:"dialogue"`
	Caption     string `json:"caption"`
	Shots       []Shot `json:"shots"`
	VerifyFlags []string `json:"verify_flags"`
}

// Shot is one planned scene in a script draft.
type Shot struct {
	Scene     int    `json:"scene"`
	Kind      string `json:"kind"`
	Prompt    string `json:"prompt"`
	DurSec    int    `json:"dur_sec"`
	Voiceover string `json:"voiceover"`
}
