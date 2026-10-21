package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/runtime"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func release(ctx context.Context, s *knowledge.Store, orgID int64, records []releaseRecord, operation string) (string, error) {
	src, err := findReleaseSource(ctx, s, orgID)
	if err != nil {
		return "", err
	}
	if src == nil && operation != "apply" {
		return "not_loaded", nil
	}
	if src == nil {
		src, err = createReleaseSource(ctx, s, orgID, records)
		if err != nil {
			return "", err
		}
	}
	if !sameJSON(src.ConnectionConfig, releaseConfig(records)) {
		return "", errors.New("release source provenance differs; refusing modification")
	}
	want := materialize(records, orgID, src.ID)
	src, err = publishIfNeeded(ctx, s, src, want, operation)
	if err != nil {
		return "", err
	}
	if operation == "withdraw" {
		return withdrawRelease(ctx, s, src, want)
	}
	if src.Health.Status != sources.HealthHealthy {
		return "", errors.New("release source is not healthy; will not re-approve")
	}
	if err := verifyRelease(ctx, s, src, want, false); err != nil {
		return "", err
	}
	return "loaded_and_retrievable", verifyRetrieval(ctx, s, orgID, want)
}

func publishIfNeeded(ctx context.Context, s *knowledge.Store, src *sources.Source, want []*assets.Asset, operation string) (*sources.Source, error) {
	if operation != "apply" || src.Health.Status != sources.HealthSyncing {
		return src, nil
	}
	if err := s.PublishReviewedAssets(ctx, src.OrgID, src.ID, approvalReference, want); err != nil {
		return nil, err
	}
	return s.GetSource(ctx, src.ID, src.OrgID)
}

func createReleaseSource(ctx context.Context, s *knowledge.Store, orgID int64, records []releaseRecord) (*sources.Source, error) {
	return s.UpsertSource(ctx, &sources.Source{OrgID: orgID, Type: sources.SourceWebsite,
		Label: releaseLabel, ConnectionConfig: releaseConfig(records), SyncPolicy: sources.SyncManual,
		Health: sources.Health{Status: sources.HealthSyncing}})
}

func findReleaseSource(ctx context.Context, s *knowledge.Store, orgID int64) (*sources.Source, error) {
	all, err := s.ListSourcesForOrg(ctx, orgID, sources.ListFilter{})
	if err != nil {
		return nil, err
	}
	var found *sources.Source
	for _, src := range all {
		if src.Label != releaseLabel {
			continue
		}
		if found != nil || src.Type != sources.SourceWebsite || src.SyncPolicy != sources.SyncManual {
			return nil, errors.New("ambiguous or modified release source")
		}
		found = src
	}
	return found, nil
}

func releaseConfig(records []releaseRecord) json.RawMessage {
	refs := make(map[string]json.RawMessage)
	for _, r := range records {
		refs[r.ID] = r.Sources
	}
	data, _ := json.Marshal(map[string]any{"manual_review_only": true, "review_sha256": releaseHash,
		"approval_reference": approvalReference, "reviewed_sources": refs})
	return data
}

func sameJSON(a, b json.RawMessage) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

func verifyRelease(ctx context.Context, s *knowledge.Store, src *sources.Source, want []*assets.Asset, allowHidden bool) error {
	got, err := s.ListAssetsForOrg(ctx, src.OrgID, assets.ListFilter{SourceID: src.ID})
	if err != nil {
		return err
	}
	if len(got) != len(want) {
		return errors.New("release asset count mismatch")
	}
	byID := make(map[string]*assets.Asset)
	for _, a := range got {
		byID[a.ExternalID] = a
	}
	for _, a := range want {
		if !matchesReleaseAsset(byID[a.ExternalID], a, allowHidden) {
			return errors.New("release changed; refusing overwrite or re-approval")
		}
	}
	return nil
}

func matchesReleaseAsset(got, want *assets.Asset, allowHidden bool) bool {
	if got == nil || got.Type != want.Type || got.Title != want.Title || got.Description != want.Description {
		return false
	}
	validState := got.State == assets.StateApproved || (allowHidden && got.State == assets.StateHidden)
	return validState && sameJSON(got.Payload, want.Payload) &&
		reflect.DeepEqual(assets.NormalizeTags(got.Tags), assets.NormalizeTags(want.Tags))
}

func verifyRetrieval(ctx context.Context, s *knowledge.Store, orgID int64, want []*assets.Asset) error {
	builder := runtime.NewBuilder(s)
	for _, a := range want {
		text, err := builder.ReplyKnowledgeForLead(ctx, orgID, a.Title+" "+strings.Join(a.Tags, " "))
		if err != nil {
			return err
		}
		var items []assets.ReplyKnowledgeV1
		if json.Unmarshal([]byte(text), &items) != nil || !containsFacts(items, a.Payload) {
			return errors.New("release retrieval verification failed")
		}
	}
	return nil
}

func containsFacts(items []assets.ReplyKnowledgeV1, payload json.RawMessage) bool {
	want, ok := assets.ReplyKnowledgeFacts(payload)
	for _, item := range items {
		if ok && reflect.DeepEqual(item, want) {
			return true
		}
	}
	return false
}
