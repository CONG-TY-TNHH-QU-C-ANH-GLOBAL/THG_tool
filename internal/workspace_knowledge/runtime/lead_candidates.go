package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/products"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// CandidatesForLead is the Knowledge Intelligence Layer's retrieval step (P2a —
// specs/COMMENT_INTELLIGENCE_PIPELINE.md §4). It returns the GROUNDING-eligible
// candidate set for a lead — the only knowledge the agent may select from —
// plus the retrieval_id that joins to the replay trace.
//
// It reuses the same Searcher as BuildForLeadWithTrace (hybrid + pgvector) so
// the candidates are exactly what the existing retrieval would surface; it does
// NOT change the existing prompt-block path. Empty result (no searcher / no
// knowledge) returns (nil, "", nil) so the caller degrades to knowledge_gap.
func (b *Builder) CandidatesForLead(ctx context.Context, orgID int64, leadText string) ([]models.KnowledgeCandidate, string, error) {
	if b == nil || b.Searcher == nil || orgID <= 0 {
		return nil, "", nil
	}
	k := b.K
	if k <= 0 {
		k = 6
	}
	// Explicit approved-only filter (P2a.1 #6). EffectiveStates already defaults
	// to approved, but we state it so the guarantee does not depend on that
	// default; the per-hit skip below is belt-and-braces.
	filter := retrieval.SearchFilter{States: []assets.AssetState{assets.StateApproved}}
	hits, _, err := b.Searcher.TopKWithTrace(ctx, orgID, leadText, filter, k)
	if err != nil {
		return nil, "", err
	}
	if len(hits) == 0 {
		return nil, "", nil
	}
	out := make([]models.KnowledgeCandidate, 0, len(hits))
	for _, h := range hits {
		if h.Asset == nil {
			continue
		}
		// Defense-in-depth: never expose a non-approved or banned asset as a
		// grounding candidate, regardless of searcher behavior.
		if h.Asset.State != "" && h.Asset.State != assets.StateApproved {
			continue
		}
		if h.Asset.Type == assets.AssetBannedClaim {
			continue
		}
		out = append(out, candidateFromHit(h))
	}
	// retrieval_id (P2a.1 #1): we do NOT mint a fake id here — a fake would not
	// join any persisted replay trace. This path does not record its own trace
	// (the canonical generation path owns trace persistence). Returns "" =
	// "not persisted by this path"; a joinable id arrives when P2c makes this
	// the canonical retrieval and records the single trace.
	return out, "", nil
}

// candidateFromHit maps one retrieval hit to a grounding candidate, extracting
// the real SKU / price / primary image from the asset payload. Pure (no I/O) so
// the field-surfacing — including Images[0] for products — is unit-testable
// without a store.
func candidateFromHit(h retrieval.Hit) models.KnowledgeCandidate {
	a := h.Asset
	c := models.KnowledgeCandidate{
		AssetID: a.ID,
		Kind:    string(a.Type),
		Title:   a.Title,
		Summary: truncateRunes(a.Description, 200),
		Score:   h.Score,
	}
	if a.Type == assets.AssetPODProduct {
		var pv products.PayloadV1
		if len(a.Payload) > 0 && json.Unmarshal(a.Payload, &pv) == nil {
			c.SKU = pv.DisplaySKU
			c.PriceText = formatCandidatePrice(pv.PriceMin, pv.PriceMax, pv.Currency)
			if len(pv.Images) > 0 { // Images[0] is the primary — surface it (fixes cause #3)
				c.ImageURL = strings.TrimSpace(pv.Images[0])
			}
		}
		return c
	}
	// Non-product assets (proofs/playbooks/site) may carry object-shaped images.
	if imgs := assets.ImagesFromPayload(a.Payload); len(imgs) > 0 {
		c.ImageURL = imgs[0].URL
	}
	return c
}

// formatCandidatePrice renders the canonical price interval into the short
// candidate form ("4.5-9 USD" / "22 USD" / "22" / ""). Mirrors the assembly
// package's formatter; kept local to avoid exporting an internal helper.
func formatCandidatePrice(minPrice, maxPrice *float64, currency string) string {
	if minPrice == nil && maxPrice == nil {
		return ""
	}
	lo, hi := minPrice, maxPrice
	if lo == nil {
		lo = hi
	}
	if hi == nil {
		hi = lo
	}
	currency = strings.TrimSpace(currency)
	left := strconv.FormatFloat(*lo, 'f', -1, 64)
	if *lo == *hi {
		if currency == "" {
			return left
		}
		return left + " " + currency
	}
	out := left + "-" + strconv.FormatFloat(*hi, 'f', -1, 64)
	if currency != "" {
		out += " " + currency
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
