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
	v := TrustVerdict{}

	// --- Component 1: retrieval quality (40 pts) ---
	// MeanPrecisionAtK is in [0, 1]. Map to [0, 40] linearly.
	v.Score += int(r.Quality.MeanPrecisionAtK * 40)
	if r.Quality.MeanPrecisionAtK < 0.30 {
		v.WarningIssues = append(v.WarningIssues,
			"Low mean Precision@K ("+formatPct(r.Quality.MeanPrecisionAtK)+") — review prompt fixtures or tune retrieval thresholds.")
	}

	// --- Component 2: fallback rate (20 pts) ---
	// Healthy = low fallback. 0% fallback = 20 pts. 50% fallback = 0 pts.
	// Beyond 50% we cap at 0 — system is essentially running on the
	// secondary path; the primary isn't earning its keep.
	fbScore := 20.0 - 40.0*r.FallbackBehaviour.FallbackRate
	if fbScore < 0 {
		fbScore = 0
	}
	v.Score += int(fbScore)
	if r.FallbackBehaviour.FallbackRate > 0.25 {
		v.WarningIssues = append(v.WarningIssues,
			"Elevated fallback rate ("+formatPct(r.FallbackBehaviour.FallbackRate)+") — primary searcher unreliable.")
	}

	// --- Component 3: replay completeness (20 pts) ---
	completenessScore := r.ReplayHealth.CompletenessRate * 20
	v.Score += int(completenessScore)
	if r.ReplayHealth.TracesProduced > 0 && r.ReplayHealth.CompletenessRate < 0.95 {
		v.BlockingIssues = append(v.BlockingIssues,
			"Replay traces incomplete ("+formatPct(r.ReplayHealth.CompletenessRate)+") — observability substrate broken.")
	}

	// --- Component 4: failure modes (10 pts) ---
	if len(r.FailureModes) > 0 {
		passed := 0
		for _, fm := range r.FailureModes {
			if fm.Verdict == "PASS" {
				passed++
			}
		}
		fmScore := float64(passed) / float64(len(r.FailureModes)) * 10
		v.Score += int(fmScore)
		failedScenarios := []string{}
		for _, fm := range r.FailureModes {
			if fm.Verdict != "PASS" {
				failedScenarios = append(failedScenarios, fm.ID+": "+fm.Name)
			}
		}
		if len(failedScenarios) > 0 {
			v.BlockingIssues = append(v.BlockingIssues,
				"Failure-mode scenarios failed: "+joinComma(failedScenarios))
		}
	}

	// --- Component 5: zero compliance / hidden leaks (10 pts) ---
	complianceLeaks := 0
	hiddenLeaks := 0
	for _, p := range r.PromptOutcomes {
		complianceLeaks += len(p.ComplianceLeaks)
		hiddenLeaks += len(p.HiddenLeaks)
	}
	if complianceLeaks == 0 && hiddenLeaks == 0 {
		v.Score += 10
	} else {
		v.BlockingIssues = append(v.BlockingIssues,
			"Compliance/hidden leaks detected — banned or hidden assets surfaced in retrieval results.")
	}

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
	return v
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
