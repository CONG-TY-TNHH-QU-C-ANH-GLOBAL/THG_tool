package reel

import (
	"context"
	"encoding/json"
	"fmt"

	reelstore "github.com/thg/scraper/internal/store/reel"
)

// EnrichedService orchestrates the "enriched" reel pipeline: transcribe the
// uploaded source, generate translated+timed subtitles and an avatar script,
// then (after human approval) render the avatar and compose the final video.
// It owns sequencing and state/cost bookkeeping only — every provider is a
// consumer-owned port (ports_enriched.go); every write goes through the
// Postgres reel store. See the ADR and the Reel v2 plan.
type EnrichedService struct {
	store       EnrichedStore
	obj         ObjectStore
	transcriber Transcriber
	engine      ScriptEngine
	avatar      AvatarRenderer
	composer    Composer
	deps        EnrichedDeps
}

// EnrichedDeps carries the non-port configuration the pipeline needs.
// AvatarID/VoiceID pick the HeyGen assets; MarketingGuide grounds the script
// engine; AvatarPos is where Remotion keys the avatar (e.g. "bottom-right").
type EnrichedDeps struct {
	AvatarID       string
	VoiceID        string
	MarketingGuide string
	AvatarPos      string
}

// NewEnrichedService wires the enriched pipeline. All ports are required;
// pass the fakes (fakes_enriched.go) for dev/CI.
func NewEnrichedService(store EnrichedStore, obj ObjectStore, tr Transcriber, engine ScriptEngine, avatar AvatarRenderer, composer Composer, deps EnrichedDeps) *EnrichedService {
	if deps.AvatarPos == "" {
		deps.AvatarPos = "bottom-right"
	}
	return &EnrichedService{store: store, obj: obj, transcriber: tr, engine: engine, avatar: avatar, composer: composer, deps: deps}
}

// enrichedContent is the reel_scripts.content JSON shape for the enriched
// format: the timed subtitles Remotion overlays plus the avatar's spoken
// script. Kept private — callers read it back via GetEnrichedScript.
type enrichedContent struct {
	Subtitles    []Cue  `json:"subtitles"`
	AvatarScript string `json:"avatar_script"`
	LangTgt      string `json:"lang_tgt"`
}

// PrepareScript runs the automated pre-approval stages for an already-created
// reel whose source clip has been uploaded (SetSource done): transcribe the
// source, generate the enriched script, persist a new script version, and
// move the reel to 'scripting' for human approval. Idempotent per call in
// the sense that each invocation appends a new script version.
func (s *EnrichedService) PrepareScript(ctx context.Context, orgID, reelID int64) (*reelstore.Script, error) {
	enr, err := s.store.GetEnriched(ctx, orgID, reelID)
	if err != nil {
		return nil, notFoundAs(err, ErrReelNotFound)
	}
	if enr.SourceKey == "" {
		return nil, ErrNoSource
	}

	srcURL, err := s.obj.SignedURL(ctx, enr.SourceKey, signedURLTTL)
	if err != nil {
		return nil, fmt.Errorf("reel: sign source url: %w", err)
	}
	tr, err := s.transcriber.Transcribe(ctx, srcURL)
	if err != nil {
		return nil, fmt.Errorf("reel: transcribe: %w", err)
	}
	trIn := reelstore.TranscriptInput{Segments: cuesJSON(tr.Cues), LangSrc: tr.LangSrc, Source: tr.Source, CostUSD: tr.CostUSD}
	if _, err := s.store.CreateTranscript(ctx, orgID, reelID, trIn); err != nil {
		return nil, err
	}
	if err := s.store.AddCost(ctx, orgID, reelID, tr.CostUSD); err != nil {
		return nil, err
	}

	script, err := s.engine.Generate(ctx, ScriptInput{Brief: enr.SourceKey, Transcript: tr, MarketingGuide: s.deps.MarketingGuide})
	if err != nil {
		return nil, fmt.Errorf("reel: script engine: %w", err)
	}
	content, err := json.Marshal(enrichedContent{Subtitles: script.Subtitles, AvatarScript: script.AvatarScript, LangTgt: script.LangTgt})
	if err != nil {
		return nil, fmt.Errorf("reel: marshal script: %w", err)
	}

	version, err := s.nextScriptVersion(ctx, orgID, reelID)
	if err != nil {
		return nil, err
	}
	scriptID, err := s.store.CreateScript(ctx, orgID, reelID, version, string(content))
	if err != nil {
		return nil, err
	}
	if err := s.store.AddCost(ctx, orgID, reelID, script.CostUSD); err != nil {
		return nil, err
	}
	if err := s.store.UpdateReelStatus(ctx, orgID, reelID, StatusScripting); err != nil {
		return nil, err
	}
	return &reelstore.Script{ID: scriptID, OrgID: orgID, ReelID: reelID, Version: version, Content: string(content)}, nil
}

// nextScriptVersion returns 1 if the reel has no script yet, else latest+1.
// Mirrors Service.nextScriptVersion (same race note applies).
func (s *EnrichedService) nextScriptVersion(ctx context.Context, orgID, reelID int64) (int, error) {
	latest, err := s.store.GetLatestScript(ctx, orgID, reelID)
	if err != nil {
		if isNoRows(err) {
			return 1, nil
		}
		return 0, err
	}
	return latest.Version + 1, nil
}

// cuesJSON marshals cues for reel_transcripts.segments. Cues hold only
// strings/ints, so Marshal cannot fail on this shape; the fallback keeps the
// function non-erroring and deterministic.
func cuesJSON(cues []Cue) string {
	b, err := json.Marshal(cues)
	if err != nil {
		return "[]"
	}
	return string(b)
}
