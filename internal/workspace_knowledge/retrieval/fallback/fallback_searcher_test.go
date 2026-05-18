package fallback

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// scriptedSearcher returns canned results based on its config. Lets
// fallback tests reproduce primary-error / primary-empty / primary-OK
// scenarios deterministically.
type scriptedSearcher struct {
	name      string
	hits      []retrieval.Hit
	trace     retrieval.Trace
	err       error
	calls     int
}

func (s *scriptedSearcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := s.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}
func (s *scriptedSearcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	s.calls++
	return s.hits, s.trace, s.err
}
func (s *scriptedSearcher) SearcherName() string { return s.name }

// fakeTester implements EmptinessTest — returns true based on the
// trace's CandidatesConsidered field, mimicking pgvector's behaviour.
type fakeTester struct{}

func (fakeTester) IsEmptyForFallback(trace retrieval.Trace) bool {
	return trace.CandidatesConsidered == 0
}

func mkHit(id int64, title string) retrieval.Hit {
	return retrieval.Hit{
		Asset: &assets.Asset{ID: id, Title: title, Type: assets.AssetPODProduct},
		Score: 0.9,
	}
}

// Primary success: secondary is never called, primary trace surfaces verbatim.
func TestFallback_PrimarySuccess_BypassesSecondary(t *testing.T) {
	primary := &scriptedSearcher{
		name:  "primary",
		hits:  []retrieval.Hit{mkHit(1, "Primary Hit")},
		trace: retrieval.Trace{SearcherImpl: "primary-v1"},
	}
	secondary := &scriptedSearcher{name: "secondary"}
	wrap := New(primary, secondary)
	hits, trace, err := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("TopKWithTrace: %v", err)
	}
	if len(hits) != 1 || hits[0].Asset.Title != "Primary Hit" {
		t.Errorf("primary hits should pass through; got %+v", hits)
	}
	if trace.SearcherImpl != "primary-v1" {
		t.Errorf("trace should carry primary's SearcherImpl; got %q", trace.SearcherImpl)
	}
	if secondary.calls != 0 {
		t.Errorf("secondary should not be called on primary success; calls=%d", secondary.calls)
	}
}

// Primary errors → secondary serves; trace annotates fallback reason.
// Goal directive PR-2 §1.
func TestFallback_PrimaryError_DelegatesToSecondary(t *testing.T) {
	primary := &scriptedSearcher{
		name: "pgvector",
		err:  errors.New("vector backend down"),
	}
	secondary := &scriptedSearcher{
		name: "hybrid",
		hits: []retrieval.Hit{mkHit(2, "Hybrid Hit")},
		trace: retrieval.Trace{SearcherImpl: "hybrid-v1"},
	}
	wrap := New(primary, secondary)
	hits, trace, err := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("fallback should swallow primary's error; got %v", err)
	}
	if len(hits) != 1 || hits[0].Asset.Title != "Hybrid Hit" {
		t.Errorf("hybrid hits should serve; got %+v", hits)
	}
	if trace.TotalByReason[ReasonFallbackError] != 1 {
		t.Errorf("trace should carry fallback_primary_error reason; got %v", trace.TotalByReason)
	}
	// Annotation: a synthetic RejectedCandidate names the primary.
	found := false
	for _, r := range trace.Rejected {
		if r.Reason == ReasonFallbackError && contains(r.Title, "pgvector") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Rejected[] annotation mentioning primary searcher; got %+v", trace.Rejected)
	}
}

// Primary timeout → fallback fires with timeout-specific reason.
func TestFallback_PrimaryTimeout_DistinguishedFromError(t *testing.T) {
	primary := &scriptedSearcher{
		name: "pgvector",
		err:  context.DeadlineExceeded,
	}
	secondary := &scriptedSearcher{
		name:  "hybrid",
		hits:  []retrieval.Hit{mkHit(2, "Hybrid Hit")},
		trace: retrieval.Trace{SearcherImpl: "hybrid-v1"},
	}
	wrap := New(primary, secondary)
	_, trace, _ := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if trace.TotalByReason[ReasonFallbackTimeout] != 1 {
		t.Errorf("timeout should map to fallback_primary_timeout; got %v", trace.TotalByReason)
	}
	if trace.TotalByReason[ReasonFallbackError] != 0 {
		t.Errorf("timeout must NOT also count as generic error")
	}
}

// Primary empty + EmptinessTester says yes → fallback.
func TestFallback_EmptyTriggersTester(t *testing.T) {
	primary := &scriptedSearcher{
		name:  "pgvector",
		hits:  nil,
		trace: retrieval.Trace{SearcherImpl: "pgvector-v1", CandidatesConsidered: 0},
	}
	secondary := &scriptedSearcher{
		name: "hybrid",
		hits: []retrieval.Hit{mkHit(2, "Hybrid Hit")},
	}
	wrap := New(primary, secondary)
	wrap.EmptinessTester = fakeTester{}
	hits, trace, _ := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if len(hits) != 1 {
		t.Errorf("empty primary + tester=yes should fall back; got %d hits", len(hits))
	}
	if trace.TotalByReason[ReasonFallbackEmpty] != 1 {
		t.Errorf("expected fallback_primary_empty reason")
	}
}

// Primary empty + EmptinessTester says NO → don't fall back.
// This is the threshold-rejection case: pgvector tried, considered
// candidates, none crossed the bar — falling back to hybrid wouldn't
// add signal.
func TestFallback_EmptyButTesterSaysNo_NoFallback(t *testing.T) {
	primary := &scriptedSearcher{
		name:  "pgvector",
		hits:  nil,
		trace: retrieval.Trace{SearcherImpl: "pgvector-v1", CandidatesConsidered: 5},
	}
	secondary := &scriptedSearcher{
		name: "hybrid",
		hits: []retrieval.Hit{mkHit(2, "Hybrid Hit")},
	}
	wrap := New(primary, secondary)
	wrap.EmptinessTester = fakeTester{}
	hits, _, _ := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if len(hits) != 0 {
		t.Errorf("threshold-empty should NOT trigger fallback; got %d hits", len(hits))
	}
	if secondary.calls != 0 {
		t.Errorf("secondary should not be invoked when threshold rejected")
	}
}

// Primary empty + NO tester → don't fall back (legitimate empty).
// Defensive: a searcher without IsEmptyForFallback opts out of
// empty-triggered fallback by default.
func TestFallback_EmptyWithoutTester_TreatsAsLegitimate(t *testing.T) {
	primary := &scriptedSearcher{name: "x", hits: nil}
	secondary := &scriptedSearcher{name: "y", hits: []retrieval.Hit{mkHit(1, "Y")}}
	wrap := New(primary, secondary)
	// No EmptinessTester set.
	hits, _, _ := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if len(hits) != 0 {
		t.Errorf("empty without tester should not trigger fallback; got %d hits", len(hits))
	}
}

// SQLite path: Primary=nil means "no vector capability, delegate to
// hybrid". The wrapper does NOT error; it just hands the call straight
// to secondary.
func TestFallback_NilPrimary_DelegatesDirectly(t *testing.T) {
	secondary := &scriptedSearcher{
		name: "hybrid",
		hits: []retrieval.Hit{mkHit(1, "Hybrid")},
		trace: retrieval.Trace{SearcherImpl: "hybrid-v1"},
	}
	wrap := New(nil, secondary)
	hits, trace, err := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("nil-primary should not error; got %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("secondary should serve directly; got %d hits", len(hits))
	}
	if trace.SearcherImpl != "hybrid-v1" {
		t.Errorf("trace should be secondary's untouched; got %q", trace.SearcherImpl)
	}
}

// No-searchers configured: error. Defensive — never returns empty
// without explanation.
func TestFallback_BothNil_ReturnsError(t *testing.T) {
	wrap := New(nil, nil)
	_, _, err := wrap.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if err == nil {
		t.Error("both-nil should error explicitly")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
