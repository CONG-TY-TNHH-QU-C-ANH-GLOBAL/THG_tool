package runtime

import (
	"context"
	"log"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// RetrieveImagesForLead returns the primary image URLs for the
// best-matching product assets for a lead. The outbound runtime
// uses this AFTER drafting comment text but BEFORE submitting to
// the browser layer — when image attachment is enabled, the URLs
// flow into the Facebook compose surface alongside the text.
//
// CONSTRAINTS (from CLAUDE.md "Do not generate AI images"):
//   - Images come from operator-approved assets only (state=approved).
//   - We never generate, augment, or transform image bytes — only
//     hand URLs to the browser layer.
//   - Hidden / pending assets are excluded, just like text retrieval.
//
// max controls how many images to return. 0 means "no images" — the
// caller can disable image attachment by passing 0 without needing
// to skip the call.
//
// Returns nil (no error, no images) when no qualifying assets are
// found. Network / DB errors are logged and absorbed — text-only
// outreach is the safe fallback per the design doc.
func (b *Builder) RetrieveImagesForLead(ctx context.Context, orgID int64, leadText string, max int) []LeadImage {
	if b == nil || b.Searcher == nil || orgID <= 0 || max <= 0 {
		return nil
	}
	// Only POD products carry image payloads in practice. Restricting
	// the type filter saves a re-rank pass that would drop FAQs /
	// policies (which never have images) anyway.
	hits, err := b.Searcher.TopK(ctx, orgID, leadText, retrieval.SearchFilter{
		Types: []assets.AssetType{assets.AssetPODProduct},
	}, max*2) // 2x headroom: some products have no image payload
	if err != nil {
		log.Printf("[knowledge.RetrieveImagesForLead] org=%d retrieval failed: %v", orgID, err)
		return nil
	}
	out := make([]LeadImage, 0, max)
	for _, h := range hits {
		if h.Asset == nil {
			continue
		}
		img := assets.PrimaryImage(h.Asset.Payload)
		if img == nil {
			continue
		}
		out = append(out, LeadImage{
			AssetID: h.Asset.ID,
			Title:   h.Asset.Title,
			URL:     img.URL,
			Alt:     img.Alt,
			Score:   h.Score,
		})
		if len(out) >= max {
			break
		}
	}
	return out
}

// LeadImage is the cross-boundary type the browser layer receives.
// Carries enough context for the operator-replay surface to show
// "this image attached because Cat Tee scored 0.91 on retrieval"
// without re-running the search.
type LeadImage struct {
	AssetID int64
	Title   string
	URL     string
	Alt     string
	Score   float64
}
