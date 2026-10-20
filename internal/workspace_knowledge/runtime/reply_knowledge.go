package runtime

import (
	"context"
	"encoding/json"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

var replyKnowledgeTypes = []assets.AssetType{assets.AssetFAQ, assets.AssetShippingPolicy, assets.AssetSalesPlaybook, assets.AssetPricingRule}

// ReplyKnowledgeForLead has its own budget so policies cannot crowd out SKUs.
// It never serializes raw descriptions, internal SOPs or pending knowledge.
func (b *Builder) ReplyKnowledgeForLead(ctx context.Context, orgID int64, leadText string) (string, error) {
	if b == nil || b.Searcher == nil || orgID <= 0 {
		return "", nil
	}
	filter := retrieval.SearchFilter{States: []assets.AssetState{assets.StateApproved}, Types: replyKnowledgeTypes}
	// Older FAQ rows may lack the explicit public projection. Inspect a
	// bounded larger pool before choosing at most four eligible entries.
	hits, _, err := b.Searcher.TopKWithTrace(ctx, orgID, leadText, filter, 12)
	if err != nil {
		return "", err
	}
	var selected []assets.ReplyKnowledgeV1
	budget := 0
	for _, hit := range hits {
		if len(selected) == 4 {
			break
		}
		if !eligibleReplyKnowledge(hit.Asset, orgID) {
			continue
		}
		item, ok := assets.ReplyKnowledgeFacts(hit.Asset.Payload)
		encoded, _ := json.Marshal(item)
		if !ok || budget+len(encoded) > 10000 {
			continue
		}
		selected = append(selected, item)
		budget += len(encoded)
	}
	if len(selected) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(selected)
	return string(encoded), err
}

func eligibleReplyKnowledge(a *assets.Asset, orgID int64) bool {
	if a == nil || a.OrgID != orgID || a.State != assets.StateApproved {
		return false
	}
	for _, kind := range replyKnowledgeTypes {
		if a.Type == kind {
			return true
		}
	}
	return false
}

// ProductCandidatesForLead reserves the existing product budget for products.
// General knowledge consumers continue using CandidatesForLead unchanged.
func (b *Builder) ProductCandidatesForLead(ctx context.Context, orgID int64, leadText string) ([]models.KnowledgeCandidate, string, error) {
	return b.candidatesForLead(ctx, orgID, leadText, []assets.AssetType{assets.AssetPODProduct, assets.AssetSupplierProduct})
}
