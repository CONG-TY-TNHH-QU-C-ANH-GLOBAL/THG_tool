package soak

import (
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func TestAssetMatchesIntent(t *testing.T) {
	wanted := map[string]struct{}{"cat": {}, "shirt": {}}

	cases := []struct {
		name  string
		asset *assets.Asset
		want  bool
	}{
		{"tag match", &assets.Asset{Tags: []string{"Cat", "pod"}}, true},      // case-insensitive tag
		{"title-token fallback", &assets.Asset{Title: "Premium shirt"}, true}, // no tags, title hits
		{"no match", &assets.Asset{Tags: []string{"dog"}, Title: "Mug"}, false},
		{"empty asset", &assets.Asset{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assetMatchesIntent(tc.asset, wanted); got != tc.want {
				t.Errorf("assetMatchesIntent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComputePrecisionAtK_FractionRelevant(t *testing.T) {
	h := &Harness{}
	// guards: empty hits or tags → 0.
	if got := h.computePrecisionAtK(nil, []string{"cat"}); got != 0 {
		t.Errorf("empty hits = %v, want 0", got)
	}
}
