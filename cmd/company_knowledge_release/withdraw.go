package main

import (
	"context"
	"encoding/json"

	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func withdrawRelease(ctx context.Context, s *knowledge.Store, src *sources.Source, want []*assets.Asset) (string, error) {
	if err := verifyRelease(ctx, s, src, want, true); err != nil {
		return "", err
	}
	// Exclude the whole source first. Interruption while hiding individual
	// records cannot leave the remaining records eligible for reply retrieval.
	err := s.UpdateSourceHealth(ctx, src.ID, src.OrgID, sources.Health{
		Status: sources.HealthError, Message: "reviewed release withdrawn by operator"}, len(want))
	if err != nil {
		return "", err
	}
	all, err := s.ListAssetsForOrg(ctx, src.OrgID, assets.ListFilter{SourceID: src.ID})
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(map[string]string{"approval_reference": approvalReference, "operation": "withdraw_reviewed_public_release"})
	for _, a := range all {
		if a.State == assets.StateHidden {
			continue
		}
		if err := s.SetAssetState(ctx, a.ID, src.OrgID, assets.StateHidden); err != nil {
			return "", err
		}
		if err := s.RecordFeedback(ctx, knowledge.FeedbackEvent{OrgID: src.OrgID, AssetID: a.ID, Kind: knowledge.FeedbackReject, Data: data}); err != nil {
			return "", err
		}
	}
	return "withdrawn", nil
}
