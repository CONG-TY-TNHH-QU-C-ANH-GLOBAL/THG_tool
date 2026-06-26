package ai

import "github.com/thg/scraper/internal/models"

// grounder resolves proposed items to real knowledge candidates (by asset id,
// else SKU) and accumulates grounding stats across a selection's role slots.
// Pure (in-memory). Extracted from GroundSelection so its per-role grounding
// loop no longer inflates that function's cognitive complexity (go:S3776).
type grounder struct {
	byAsset map[int64]models.KnowledgeCandidate
	bySKU   map[string]models.KnowledgeCandidate
	stats   *GroundingStats
}

// ground keeps only the proposed items backed by a candidate and allowed for
// role, enriching survivors; offer slots also feed OfferKept/OfferDropped/BestScore.
func (g *grounder) ground(items []ProposedItem, role string, isOffer bool) []models.GroundedItem {
	var out []models.GroundedItem
	for _, p := range items {
		gi, ok := groundForRole(p, role, g.byAsset, g.bySKU)
		if !ok {
			if isOffer {
				g.stats.OfferDropped++
			}
			continue
		}
		out = append(out, gi)
		if isOffer {
			g.stats.OfferKept++
			if gi.Score > g.stats.BestScore {
				g.stats.BestScore = gi.Score
			}
		}
	}
	return out
}
