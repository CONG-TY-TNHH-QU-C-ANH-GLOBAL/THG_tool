package soak

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// Test doubles used by the failure-mode scenarios — an always-erroring embedder
// and an artificially slow searcher. Split from failure_modes.go (same package).

// --- Helpers ---

// brokenEmbedder always errors. Used in failure mode A.
type brokenEmbedder struct {
	err error
}

func (b *brokenEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, embedding.WrapRecoverable(b.err)
}
func (b *brokenEmbedder) ModelVersion() string { return "broken:v1" }
func (b *brokenEmbedder) Dimensions() int      { return 8 }

// slowSearcher simulates a backend that times out. Returns err
// (typically context.DeadlineExceeded) after the configured delay.
type slowSearcher struct {
	delay time.Duration
	err   error
}

func (s *slowSearcher) TopK(ctx context.Context, _ int64, _ string, _ retrieval.SearchFilter, _ int) ([]retrieval.Hit, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, s.err
}
func (s *slowSearcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	hits, err := s.TopK(ctx, orgID, query, filter, k)
	return hits, retrieval.Trace{SearcherImpl: "slow-mock", TotalByReason: map[retrieval.RejectionReason]int{}}, err
}
func (s *slowSearcher) SearcherName() string { return "slow-mock" }
