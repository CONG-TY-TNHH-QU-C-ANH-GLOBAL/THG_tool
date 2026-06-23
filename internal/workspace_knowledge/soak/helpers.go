package soak

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// newSoakRetrievalID returns an opaque identifier the soak harness
// uses when calling RecordKnowledgeRetrievalWithTrace. Format matches
// the runtime's `ret_<hex>` convention so the soak's events look
// indistinguishable from production events on the same table.
func newSoakRetrievalID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "ret_soak_fallback"
	}
	return "ret_soak_" + hex.EncodeToString(b[:])
}

// mustValidSoakSource builds a valid sources.Source for the harness.
// Lives in its own file so harness.go stays focused on the run loop.
// Validation is the same as production — the soak exercises the real
// store path, not a test bypass.
func mustValidSoakSource(orgID int64, cfg json.RawMessage) *sources.Source {
	return &sources.Source{
		OrgID:            orgID,
		Type:             sources.SourceCSV,
		Label:            "soak-harness",
		ConnectionConfig: cfg,
		SyncPolicy:       sources.SyncManual,
		Health: sources.Health{
			Status: sources.HealthHealthy,
		},
	}
}

// computeTrustVerdict converts the soak measurements into the
// composite trust score + verdict the operator reads first. The
// formula is intentionally explicit (not ML-tuned) so a future
// reader can answer "why did this run get 72/100?".
//
// Scoring weights:
//
//	40 pts  retrieval quality (precision @ k mean)
//	20 pts  fallback rate inversely (low fallback = healthy)
//	20 pts  replay completeness rate
//	10 pts  failure-mode pass rate
//	10 pts  zero compliance / hidden leaks across all prompts
//
// Verdict thresholds:
//
//	>= 80  READY        (substrate trusted)
//	60-79  DEGRADED     (ship with caveats; operator must monitor)
//	<  60  NOT_READY    (fix issues before relying on substrate)
//
// Blocking issues drop verdict to NOT_READY regardless of score.
// Compliance leaks, hidden-state leaks, and replay-completeness
// failures are blocking.
func computeTrustVerdict(r *Report) TrustVerdict {
	// Each component contributes to the same accumulator in a fixed order;
	// the per-component scoring rules live in trust_verdict.go.
	v := &TrustVerdict{}
	scoreRetrievalQuality(v, r)
	scoreFallbackRate(v, r)
	scoreReplayCompleteness(v, r)
	scoreFailureModes(v, r)
	scoreComplianceLeaks(v, r)

	// --- Final verdict ---
	switch {
	case len(v.BlockingIssues) > 0:
		v.Verdict = "NOT_READY"
	case v.Score >= 80:
		v.Verdict = "READY"
	case v.Score >= 60:
		v.Verdict = "DEGRADED"
	default:
		v.Verdict = "NOT_READY"
	}

	// Clamp score so blocking issues don't artificially inflate.
	if v.Score > 100 {
		v.Score = 100
	}
	if v.Score < 0 {
		v.Score = 0
	}
	return *v
}

func formatPct(f float64) string {
	// Two-decimal percent — "12.50%". Avoids fmt.Sprintf in this hot
	// path's typical 1-2 calls per report; keep simple.
	n := int(f * 10000)
	whole := n / 100
	frac := n % 100
	out := strconv.Itoa(whole) + "."
	if frac < 10 {
		out += "0"
	}
	out += strconv.Itoa(frac) + "%"
	return out
}

func joinComma(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	if len(xs) == 1 {
		return xs[0]
	}
	out := xs[0]
	for _, s := range xs[1:] {
		out += ", " + s
	}
	return out
}
