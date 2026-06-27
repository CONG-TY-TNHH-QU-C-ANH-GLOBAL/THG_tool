package reel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/store"
	reelstore "github.com/thg/scraper/internal/store/reel"
)

// Lease budgets for the render spend path. The reel lease is the orphan-detection
// window (webhook silent past this → render_stuck, a human resolves it, NEVER auto
// re-render). The shot lease bounds a single shot's provider job.
const (
	renderLeaseSeconds = 1800
	shotLeaseSeconds   = 900
)

// Service is the reel application/workflow layer. It owns orchestration only; durable
// state lives in store.Reel() and posting flows through store.Outbound().
type Service struct {
	db       *store.Store
	renderer VideoRenderer
	engine   *scriptEngine
	render   RenderConfig // local assembly (ffmpeg + media dir); empty → synthetic stitch
}

// ScriptConfig carries provider settings for the script engine. AnthropicAPIKey enables the
// real Claude path; empty leaves the engine on OpenAI/deterministic.
type ScriptConfig struct {
	AnthropicAPIKey string
	AnthropicModel  string
	MarketingGuide  string // brand marketing playbook content grounding the script prompt
}

// NewService wires the reel workflow. mgGet resolves the OpenAI generator lazily (wired
// after routes register); sc enables the Anthropic script path; rc enables local ffmpeg
// assembly (empty → synthetic stitch). With neither AI configured the script engine degrades
// to a deterministic plan so the backend runs offline.
func NewService(db *store.Store, renderer VideoRenderer, mgGet func() *ai.MessageGenerator, sc ScriptConfig, rc RenderConfig) *Service {
	return &Service{
		db:       db,
		renderer: renderer,
		engine:   &scriptEngine{mgGet: mgGet, claude: newClaudeClient(sc.AnthropicAPIKey, sc.AnthropicModel), marketing: sc.MarketingGuide},
		render:   rc,
	}
}

// RequestInput is the create-reel command.
type RequestInput struct {
	CreatedBy         int64
	MissionID         string
	Source            string
	BriefStyle        string
	Keywords          []string
	ProductRefs       []string
	TargetDurationSec int
}

// Result bundles the reel and its current script/shots for the HTTP layer.
type Result struct {
	Reel   *reelstore.Reel   `json:"reel"`
	Script *reelstore.Script `json:"script"`
	Shots  []reelstore.Shot  `json:"shots,omitempty"`
}

// RequestReel creates a draft, runs the script engine, persists script v1, and lands the
// reel in script_ready. The script engine never fails the request (it degrades).
func (s *Service) RequestReel(ctx context.Context, orgID int64, in RequestInput) (*Result, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("reel: org_id required")
	}
	kw, _ := json.Marshal(in.Keywords)
	refs, _ := json.Marshal(in.ProductRefs)
	id, err := s.db.Reel().CreateReel(reelstore.Reel{
		OrgID: orgID, MissionID: in.MissionID, CreatedBy: in.CreatedBy, Source: in.Source,
		Status: reelstore.StatusScripting, BriefStyle: in.BriefStyle,
		Keywords: string(kw), ProductRefs: string(refs), TargetDurationSec: in.TargetDurationSec,
	})
	if err != nil {
		return nil, err
	}

	draft := s.engine.Generate(ctx, ScriptInput{
		OrgID: orgID, BriefStyle: in.BriefStyle, Keywords: in.Keywords,
		TargetDuration: in.TargetDurationSec, BusinessBlock: s.businessBlock(orgID),
	})
	if _, err := s.persistScript(orgID, id, 1, draft); err != nil {
		return nil, err
	}
	if err := s.db.Reel().UpdateReelStatus(orgID, id, reelstore.StatusScriptReady); err != nil {
		return nil, err
	}
	return s.load(orgID, id, false)
}

// UpdateScript appends a new script version. Optional dialogue/caption overrides are
// applied on top of the latest version's shot list; empty overrides keep the prior value.
func (s *Service) UpdateScript(orgID, reelID int64, dialogue, caption *string) (*Result, error) {
	latest, err := s.db.Reel().GetLatestScript(orgID, reelID)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, reelstore.ErrReelNotFound
	}
	next := *latest
	next.Version = latest.Version + 1
	if dialogue != nil {
		next.Dialogue = *dialogue
	}
	if caption != nil {
		next.Caption = *caption
	}
	if _, err := s.db.Reel().InsertScript(reelstore.Script{
		ReelID: reelID, OrgID: orgID, Version: next.Version, Dialogue: next.Dialogue,
		ShotList: next.ShotList, Caption: next.Caption, VerifyFlags: next.VerifyFlags,
	}); err != nil {
		return nil, err
	}
	return s.load(orgID, reelID, false)
}

// Approve is the spend gate. It atomically commits the render (StartRenderCAS) and, only
// on a true first start, creates + claims shots and dispatches them to the renderer.
// Calling it again while rendering returns current state and creates NO new shots.
func (s *Service) Approve(ctx context.Context, orgID, reelID int64) (*Result, error) {
	// Existence/tenant check — GetReel returns ErrReelNotFound for another org.
	if _, err := s.db.Reel().GetReel(orgID, reelID); err != nil {
		return nil, err
	}
	idemKey := fmt.Sprintf("reel-%d-render", reelID)
	started, err := s.db.Reel().StartRenderCAS(orgID, reelID, idemKey, renderLeaseSeconds)
	if err != nil {
		return nil, err
	}
	if !started {
		// Already rendering or past it — idempotent: return current state, no new spend.
		return s.load(orgID, reelID, true)
	}
	if err := s.db.Reel().ApproveLatestScript(orgID, reelID); err != nil {
		return nil, err
	}
	if err := s.dispatchShots(ctx, orgID, reelID); err != nil {
		return nil, err
	}
	return s.load(orgID, reelID, true)
}

