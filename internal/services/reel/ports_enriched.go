package reel

import (
	"context"
	"io"
	"time"
)

// Ports and value types for the "enriched" reel format: an existing
// company-shot video is enriched with a HeyGen avatar (green-screen,
// chroma-keyed into the corner) and Remotion-rendered translated subtitles.
// These are consumer-owned ports — the service is the consumer, adapters
// live at the composition root (PR-E3..E7). Fakes (fakes_enriched.go) are
// the only implementations in this PR.
//
// Design note (Task 1 spike, 2026-07-08): HeyGen v2 has no transparent/alpha
// output (background.type ∈ {color,image,video} only), so AvatarReq carries
// no "transparent" flag — the adapter always renders on green and Remotion
// keys it out. See docs/architecture/decisions/ADR-reel-studio-platform-module.md.

// Cue is one timed caption span: text shown from FromMS to ToMS of the
// source video. Timing comes from the transcriber's word timestamps and is
// what keeps subtitles synced to speech (the biggest quality risk if lost).
type Cue struct {
	Text   string `json:"text"`
	FromMS int    `json:"from_ms"`
	ToMS   int    `json:"to_ms"`
}

// Transcript is the understood content of a source video plus timing.
// Source is 'whisper' (audio branch) or 'vision' (silent branch).
type Transcript struct {
	Cues    []Cue   `json:"cues"`
	LangSrc string  `json:"lang_src"`
	Source  string  `json:"source"`
	CostUSD float64 `json:"cost_usd"`
}

// EnrichedScript is the script-engine output: translated, timed subtitles
// to overlay plus the words the avatar speaks. Persisted as the reel's
// reel_scripts.content JSON.
type EnrichedScript struct {
	Subtitles    []Cue   `json:"subtitles"`
	AvatarScript string  `json:"avatar_script"`
	LangTgt      string  `json:"lang_tgt"`
	CostUSD      float64 `json:"cost_usd"`
}

// ScriptInput is what the script engine grounds on. MarketingGuide is
// optional grounding text; the engine must not invent business facts.
type ScriptInput struct {
	Brief          string
	Transcript     Transcript
	MarketingGuide string
}

// AvatarReq asks the avatar renderer for a talking-head clip on a green
// screen. VoiceID/AvatarID identify the HeyGen assets.
type AvatarReq struct {
	Text     string
	VoiceID  string
	AvatarID string
}

// AvatarResult is a rendered avatar clip on local disk (the service uploads
// it to object storage; the renderer does not own R2).
type AvatarResult struct {
	TempPath    string
	ContentType string
	CostUSD     float64
}

// ComposeReq is the final-assembly request: the Remotion composition reads
// the source and avatar from signed URLs, overlays timed subtitles, and
// keys the avatar into AvatarPos (e.g. "bottom-right").
type ComposeReq struct {
	SourceURL string
	AvatarURL string
	Subtitles []Cue
	AvatarPos string
}

// ComposeResult is the composed final video, already persisted to object
// storage by the composer (Remotion-Lambda writes to R2 itself).
type ComposeResult struct {
	FinalKey string
	CostUSD  float64
}

// ObjectStore is the media object-storage port (R2/S3-compatible). Postgres
// stores keys; blobs live here.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader, contentType string) error
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// Transcriber understands a source video into timed cues. sourceURL is a
// signed URL to the uploaded clip.
type Transcriber interface {
	Transcribe(ctx context.Context, sourceURL string) (Transcript, error)
}

// ScriptEngine turns a transcript + brief into translated, timed subtitles
// and the avatar's spoken script.
type ScriptEngine interface {
	Generate(ctx context.Context, in ScriptInput) (EnrichedScript, error)
}

// AvatarRenderer renders the green-screen talking-head clip (HeyGen).
type AvatarRenderer interface {
	RenderAvatar(ctx context.Context, req AvatarReq) (AvatarResult, error)
}

// Composer assembles source + subtitles + keyed avatar into the final video
// (Remotion on Lambda), returning the object-storage key of the result.
type Composer interface {
	Compose(ctx context.Context, req ComposeReq) (ComposeResult, error)
}
