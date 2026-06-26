package soak

import (
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// computePrecisionAtK: what fraction of returned assets share at least
// one cluster (tag) with the prompt's expected intent. Proxy for
// "retrieval was relevant" without ground-truth relevance labels.
func (h *Harness) computePrecisionAtK(hits []retrieval.Hit, intentTags []string) float64 {
	if len(hits) == 0 || len(intentTags) == 0 {
		return 0
	}
	wanted := map[string]struct{}{}
	for _, t := range intentTags {
		wanted[strings.ToLower(t)] = struct{}{}
	}
	relevant := 0
	for _, h := range hits {
		if h.Asset == nil {
			continue
		}
		if assetMatchesIntent(h.Asset, wanted) {
			relevant++
		}
	}
	return float64(relevant) / float64(len(hits))
}

// assetMatchesIntent reports whether an asset shares any tag — or, as a
// fallback for assets with a sparse tag list but a descriptive title,
// any title token — with the wanted intent set. The wanted set must be
// pre-lowercased; tags are lowercased here, title tokens come from
// tokenise (already normalised).
func assetMatchesIntent(a *assets.Asset, wanted map[string]struct{}) bool {
	for _, tag := range a.Tags {
		if _, ok := wanted[strings.ToLower(tag)]; ok {
			return true
		}
	}
	for _, tok := range tokenise(a.Title) {
		if _, ok := wanted[tok]; ok {
			return true
		}
	}
	return false
}
