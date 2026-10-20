package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// This identifier exists only inside this process, never in a real database.
const localOrgID int64 = 1

type record struct {
	ID           string           `json:"id"`
	Topic        string           `json:"topic"`
	Type         assets.AssetType `json:"asset_type"`
	Title        string           `json:"title"`
	Verification string           `json:"verification"`
	Audience     string           `json:"audience"`
	SourceIDs    []string         `json:"source_ids"`
	Facts        []string         `json:"facts"`
	Guidance     []string         `json:"guidance"`
	Avoid        []string         `json:"avoid"`
	AppliesWhen  []string         `json:"applies_when"`
	Inputs       []string         `json:"required_inputs"`
	Steps        []string         `json:"steps"`
	Outputs      []string         `json:"outputs"`
	RelatedIDs   []string         `json:"related_ids"`
	SearchTerms  []string         `json:"search_terms"`
}

type source struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url,omitempty"`
	Repository string `json:"repository,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Coverage   string `json:"coverage"`
}

func loadDraft(dir string, publicOnly bool) (*memoryDraft, map[string]record, map[string]source, error) {
	var pack struct {
		Version      int      `json:"schema_version"`
		ReviewState  string   `json:"review_state"`
		RuntimeState string   `json:"runtime_state"`
		Records      []record `json:"records"`
	}
	var registry struct {
		Version int      `json:"schema_version"`
		Sources []source `json:"sources"`
	}
	for name, target := range map[string]any{"knowledge.json": &pack, "sources.json": &registry} {
		if err := readDraftJSON(dir, name, target); err != nil {
			return nil, nil, nil, err
		}
	}
	if pack.Version != 1 || registry.Version != 1 || pack.ReviewState != "pending" || pack.RuntimeState != "not_loaded" {
		return nil, nil, nil, fmt.Errorf("preview accepts only schema v1 pending/not_loaded drafts")
	}
	sources, err := indexDraftSources(registry.Sources)
	if err != nil {
		return nil, nil, nil, err
	}
	store, records := &memoryDraft{}, make(map[string]record)
	for index, item := range pack.Records {
		if err := validateDraftRecord(item, records, sources); err != nil {
			return nil, nil, nil, err
		}
		records[item.ID] = item
		if !includedInPreview(item, publicOnly) {
			continue
		}
		asset := materializeDraftRecord(index, item, publicOnly)
		if err := asset.Validate(); err != nil {
			return nil, nil, nil, err
		}
		store.items = append(store.items, asset)
	}
	return store, records, sources, nil
}

type memoryDraft struct{ items []*assets.Asset }

func (s *memoryDraft) ListAssetsForOrg(_ context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	var out []*assets.Asset
	for _, item := range s.items {
		if item.OrgID != orgID || !hasState(filter.States, item.State) || !hasType(filter.Types, item.Type) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func hasState(states []assets.AssetState, state assets.AssetState) bool {
	if len(states) == 0 {
		return state == assets.StateApproved
	}
	for _, candidate := range states {
		if candidate == state {
			return true
		}
	}
	return false
}

func hasType(types []assets.AssetType, kind assets.AssetType) bool {
	if len(types) == 0 {
		return true
	}
	for _, candidate := range types {
		if candidate == kind {
			return true
		}
	}
	return false
}

func readDraftJSON(dir, name string, target any) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func validateDraftRecord(item record, records map[string]record, sources map[string]source) error {
	if _, exists := records[item.ID]; exists || item.ID == "" || !item.Type.IsKnown() {
		return fmt.Errorf("invalid record %q", item.ID)
	}
	if len(item.SourceIDs) == 0 {
		return fmt.Errorf("record %q has no source", item.ID)
	}
	for _, id := range item.SourceIDs {
		if _, exists := sources[id]; !exists {
			return fmt.Errorf("record %q references missing source %q", item.ID, id)
		}
	}
	return nil
}

func materializeDraftRecord(index int, item record, publicOnly bool) *assets.Asset {
	text := append([]string{}, item.Facts...)
	if !publicOnly {
		text = append(text, item.Guidance...)
		text = append(text, item.AppliesWhen...)
		text = append(text, item.Inputs...)
		text = append(text, item.Steps...)
		text = append(text, item.Outputs...)
	}
	var payload json.RawMessage
	if includedInPreview(item, true) {
		payload, _ = json.Marshal(assets.ReplyKnowledgeV1{Schema: "company_reply_v1", Audience: item.Audience,
			Verification: item.Verification, Facts: item.Facts, SourceIDs: item.SourceIDs})
	}
	return &assets.Asset{ID: int64(index + 1), OrgID: localOrgID, SourceID: 1, Payload: payload,
		ExternalID: item.ID, Type: item.Type, Title: item.Title,
		Description: strings.Join(text, "\n"), Tags: append([]string{item.Topic}, item.SearchTerms...), State: assets.StatePending}
}

func includedInPreview(item record, publicOnly bool) bool {
	return !publicOnly || (item.Audience == "public_reference" && item.Verification == "public_source_checked" && len(item.Facts) > 0)
}

func indexDraftSources(items []source) (map[string]source, error) {
	sources := make(map[string]source)
	for _, item := range items {
		if _, exists := sources[item.ID]; exists || item.ID == "" {
			return nil, fmt.Errorf("invalid source ID %q", item.ID)
		}
		sources[item.ID] = item
	}
	return sources, nil
}
