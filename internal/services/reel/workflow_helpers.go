package reel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/ai"
	reelstore "github.com/thg/scraper/internal/store/reel"
)

// dispatchShots reads the approved shot list, inserts each as a planned shot, then CASes
// it to rendering and asks the provider to start it. Idempotent per scene via the UNIQUE
// constraint and the planned→rendering CAS.
func (s *Service) dispatchShots(ctx context.Context, orgID, reelID int64) error {
	sc, err := s.db.Reel().GetLatestScript(orgID, reelID)
	if err != nil || sc == nil {
		return fmt.Errorf("reel: no script to render")
	}
	var shots []Shot
	if err := json.Unmarshal([]byte(sc.ShotList), &shots); err != nil || len(shots) == 0 {
		return fmt.Errorf("reel: empty/invalid shot list")
	}
	for _, sh := range shots {
		scene := int64(sh.Scene)
		if err := s.db.Reel().CreateShot(reelstore.Shot{
			ReelID: reelID, OrgID: orgID, Scene: int(scene), Kind: sh.Kind, RenderState: reelstore.ShotPlanned,
		}); err != nil {
			return err
		}
		handle, err := s.renderer.StartRender(ctx, RenderRequest{
			OrgID: orgID, ReelID: reelID, Scene: scene, Kind: sh.Kind,
			Prompt: sh.Prompt, Voiceover: sh.Voiceover, DurationSec: sh.DurSec,
			IdempotencyKey: fmt.Sprintf("reel-%d-scene-%d", reelID, scene),
		})
		if err != nil {
			return err
		}
		if _, err := s.db.Reel().ClaimShotForRender(orgID, reelID, scene, handle.Provider, handle.ProviderJobID, shotLeaseSeconds); err != nil {
			return err
		}
	}
	return nil
}

// persistScript serialises a draft into a reel_scripts row.
func (s *Service) persistScript(orgID, reelID int64, version int, draft ScriptDraft) (int64, error) {
	shotsJSON, _ := json.Marshal(draft.Shots)
	flagsJSON, _ := json.Marshal(draft.VerifyFlags)
	return s.db.Reel().InsertScript(reelstore.Script{
		ReelID: reelID, OrgID: orgID, Version: version, Dialogue: draft.Dialogue,
		ShotList: string(shotsJSON), Caption: draft.Caption, VerifyFlags: string(flagsJSON),
	})
}

// businessBlock loads the org's grounded business profile for the script prompt.
func (s *Service) businessBlock(orgID int64) string {
	p := ai.LoadProfileForOrg(s.db, orgID)
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.ToPromptBlock())
}

// load assembles a Result (reel + latest script, optionally shots).
func (s *Service) load(orgID, reelID int64, withShots bool) (*Result, error) {
	r, err := s.db.Reel().GetReel(orgID, reelID)
	if err != nil {
		return nil, err
	}
	sc, err := s.db.Reel().GetLatestScript(orgID, reelID)
	if err != nil {
		return nil, err
	}
	res := &Result{Reel: r, Script: sc}
	if withShots {
		shots, err := s.db.Reel().ListShots(orgID, reelID)
		if err != nil {
			return nil, err
		}
		res.Shots = shots
	}
	return res, nil
}
