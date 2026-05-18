// Package fallback implements the zero-regression-fallback wrapper
// required by PR-2 §1 of the goal directive. It composes two
// [retrieval.Searcher] instances — a primary (typically pgvector)
// and a secondary (typically hybrid) — and ensures the runtime
// NEVER:
//
//   - returns an empty result when the secondary could have answered
//   - panics on primary failure
//   - blocks past the primary's timeout
//   - lowers retrieval quality below the secondary's baseline
//
// The wrapper is transparent — callers see one Searcher; the
// fallback decision is encoded in the trace (SearcherImpl reveals
// which path served the result, and trace.Selected carries the
// chosen searcher's reasoning).
//
// Trace stitching: when fallback fires, the SECONDARY's trace is
// what gets surfaced — that is the actually-served retrieval. The
// PRIMARY's outcome is recorded as a single TotalByReason bump
// (RejectionReason="fallback_used") and an entry in
// trace.Rejected[] with the primary's SearcherImpl in the Title
// field so the Replay UI can show "pgvector tried + fell back to
// hybrid because ___".
package fallback

import (
	"context"
	"errors"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// SearcherImpl identifies the wrapper. The wrapped searcher's name is
// surfaced inside the trace; the SearcherImpl field of the OUTER
// trace carries this constant so dashboards can identify "wrapped"
// from "single-searcher" deployments.
const SearcherImpl = "fallback-v1"

// Reason values added by this wrapper to TotalByReason. Documented
// strings live here (not in retrieval/trace.go) because they are
// fallback-specific — the retrieval package's RejectionReason taxonomy
// stays minimal; fallback-specific reasons live where the fallback
// implementation does.
const (
	ReasonFallbackError    retrieval.RejectionReason = "fallback_primary_error"
	ReasonFallbackTimeout  retrieval.RejectionReason = "fallback_primary_timeout"
	ReasonFallbackEmpty    retrieval.RejectionReason = "fallback_primary_empty"
)

// EmptinessTest is the optional hook the primary Searcher exposes to
// tell the wrapper whether its empty result should trigger fallback.
// pgvector implements this via [pgvector.IsEmptyForFallback] — the
// trace's CandidatesConsidered field is the signal.
//
// When a Searcher does NOT implement this, the wrapper falls back
// only on (a) error and (b) timeout. Len(hits)==0 without
// EmptinessTest is treated as a legitimate empty result.
type EmptinessTest interface {
	IsEmptyForFallback(trace retrieval.Trace) bool
}

// Searcher wraps a primary and a secondary [retrieval.Searcher].
// Construct via [New].
type Searcher struct {
	Primary   retrieval.Searcher
	Secondary retrieval.Searcher
	// EmptinessTester is optional. When non-nil, the wrapper consults
	// it to decide whether the primary's empty result should trigger
	// fallback. The pgvector Searcher provides IsEmptyForFallback;
	// pass `pgvector.Searcher` itself as the tester.
	EmptinessTester EmptinessTest
}

// New constructs a fallback Searcher.
func New(primary, secondary retrieval.Searcher) *Searcher {
	return &Searcher{Primary: primary, Secondary: secondary}
}

// TopK satisfies retrieval.Searcher.
func (s *Searcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := s.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}

// TopKWithTrace runs the primary; if it errors / times out / returns
// "empty per EmptinessTester" — falls back to the secondary. The
// returned trace is whichever searcher SERVED the result, with a
// single fallback-reason note appended.
func (s *Searcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	if s.Primary == nil && s.Secondary == nil {
		return nil, retrieval.Trace{}, errors.New("fallback: no searchers configured")
	}
	if s.Primary == nil {
		// No primary configured — straight to secondary. This is the
		// SQLite path: capability detection found no pgvector, so the
		// runtime built fallback{nil, hybrid}. We don't fail here, we
		// just delegate.
		return s.Secondary.TopKWithTrace(ctx, orgID, query, filter, k)
	}

	primaryHits, primaryTrace, primaryErr := s.Primary.TopKWithTrace(ctx, orgID, query, filter, k)

	// Decide fallback.
	fallbackReason, fallbackNeeded := s.decideFallback(primaryHits, primaryTrace, primaryErr)
	if !fallbackNeeded {
		return primaryHits, primaryTrace, primaryErr
	}
	if s.Secondary == nil {
		// Primary failed but no secondary to fall back to. Surface
		// the error and the primary's trace — better than silent empty.
		return primaryHits, primaryTrace, primaryErr
	}

	// Run secondary. Use the ORIGINAL ctx (not derived from primary's
	// timeout) so the secondary has fresh budget.
	secondaryHits, secondaryTrace, secondaryErr := s.Secondary.TopKWithTrace(ctx, orgID, query, filter, k)
	annotateFallback(&secondaryTrace, s.Primary, fallbackReason, primaryErr)
	return secondaryHits, secondaryTrace, secondaryErr
}

// decideFallback inspects the primary's outcome and returns:
//   - reason : the fallback-trigger reason to annotate
//   - needed : whether to invoke the secondary
//
// Decision tree:
//   - primary returned an error → fallback (reason: error or timeout)
//   - primary returned hits → no fallback (use primary)
//   - primary returned empty AND EmptinessTester says yes → fallback
//   - primary returned empty AND no tester → no fallback (trust empty)
func (s *Searcher) decideFallback(hits []retrieval.Hit, trace retrieval.Trace, err error) (retrieval.RejectionReason, bool) {
	if err != nil {
		// Distinguish timeout from generic error so Replay UI can
		// surface "primary too slow" vs "primary broken" separately.
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
			return ReasonFallbackTimeout, true
		}
		return ReasonFallbackError, true
	}
	if len(hits) > 0 {
		return "", false
	}
	// Empty result. Consult tester.
	if s.EmptinessTester == nil {
		return "", false
	}
	if s.EmptinessTester.IsEmptyForFallback(trace) {
		return ReasonFallbackEmpty, true
	}
	return "", false
}

// annotateFallback adds the fallback-trigger record to the secondary's
// trace. We do NOT clobber the secondary's own data — we ADD to its
// TotalByReason and append one synthetic RejectedCandidate that
// names the primary's SearcherImpl in the Title field so the Replay
// UI shows "pgvector-v1 fell back to hybrid-v1 (timeout)".
func annotateFallback(trace *retrieval.Trace, primary retrieval.Searcher, reason retrieval.RejectionReason, primaryErr error) {
	if trace == nil {
		return
	}
	if trace.TotalByReason == nil {
		trace.TotalByReason = map[retrieval.RejectionReason]int{}
	}
	trace.TotalByReason[reason]++

	primaryName := "primary"
	if traceProducer, ok := primary.(interface{ SearcherName() string }); ok {
		primaryName = traceProducer.SearcherName()
	}
	title := primaryName + " → fallback"
	if primaryErr != nil {
		// Truncate so a long stack trace doesn't bloat the events
		// table. 200 chars is enough for "pgvector: query: connect:
		// connection refused".
		msg := primaryErr.Error()
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		title = title + ": " + msg
	}
	trace.Rejected = append(trace.Rejected, retrieval.RejectedCandidate{
		AssetID: 0,
		Title:   title,
		Type:    assets.AssetType(""),
		Reason:  reason,
		Score:   0,
	})
}
