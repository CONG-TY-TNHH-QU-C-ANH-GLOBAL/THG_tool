package knowledge_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func TestReviewedReleaseRollsBackAssetsAuditAndPublication(t *testing.T) {
	s := newKnowledgeStore(t, "reviewed-release.db")
	ctx := context.Background()
	src, err := s.UpsertSource(ctx, &sources.Source{OrgID: 1, Type: sources.SourceWebsite,
		Label: "synthetic review", SyncPolicy: sources.SyncManual, Health: sources.Health{Status: sources.HealthSyncing}})
	if err != nil {
		t.Fatal(err)
	}
	item := func(org int64) *assets.Asset {
		return &assets.Asset{OrgID: org, SourceID: src.ID, ExternalID: "duplicate", Type: assets.AssetFAQ,
			Title: "Public fact", Description: "THG offers sourcing.", State: assets.StateApproved,
			Payload: json.RawMessage(`{"schema":"company_reply_v1","audience":"public_reference","verification":"public_source_checked","facts":["THG offers sourcing."],"source_ids":["public-site"]}`)}
	}
	// The second insertion fails the real unique constraint after the first
	// asset and its audit row have been inserted in the transaction.
	if err := s.PublishReviewedAssets(ctx, 1, src.ID, "synthetic approval", []*assets.Asset{item(1), item(1)}); err == nil {
		t.Fatal("duplicate release accepted")
	}
	all, err := s.ListAssetsForOrg(ctx, 1, assets.ListFilter{SourceID: src.ID})
	if err != nil || len(all) != 0 {
		t.Fatal("partial assets survived rollback")
	}
	audit, err := s.ListFeedbackForOrg(ctx, 1, 20)
	if err != nil || len(audit) != 0 {
		t.Fatal("partial approval audit survived rollback")
	}
	saved, err := s.GetSource(ctx, src.ID, 1)
	if err != nil || saved.Health.Status != sources.HealthSyncing {
		t.Fatal("failed transaction published source")
	}
	if err := s.PublishReviewedAssets(ctx, 1, src.ID, "synthetic approval", []*assets.Asset{item(2)}); err == nil {
		t.Fatal("cross-org asset accepted")
	}
	private := item(1)
	private.Payload = json.RawMessage(`{"schema":"company_reply_v1","audience":"internal_only"}`)
	if err := s.PublishReviewedAssets(ctx, 1, src.ID, "synthetic approval", []*assets.Asset{private}); err == nil {
		t.Fatal("private facts accepted")
	}
}
