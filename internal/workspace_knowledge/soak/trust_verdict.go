// Domain: knowledge soak. Per-component trust-score helpers extracted from
// computeTrustVerdict (helpers.go) so each scoring rule is small and
// independently readable. Behaviour is identical: the components run in the
// same order and mutate the same TrustVerdict accumulator.
package soak

// scoreRetrievalQuality maps MeanPrecisionAtK in [0,1] to [0,40] (component 1, 40 pts).
func scoreRetrievalQuality(v *TrustVerdict, r *Report) {
	v.Score += int(r.Quality.MeanPrecisionAtK * 40)
	if r.Quality.MeanPrecisionAtK < 0.30 {
		v.WarningIssues = append(v.WarningIssues,
			"Low mean Precision@K ("+formatPct(r.Quality.MeanPrecisionAtK)+") — review prompt fixtures or tune retrieval thresholds.")
	}
}

// scoreFallbackRate rewards a low fallback rate: 0% = 20 pts, 50%+ = 0 (component 2).
func scoreFallbackRate(v *TrustVerdict, r *Report) {
	fbScore := 20.0 - 40.0*r.FallbackBehaviour.FallbackRate
	if fbScore < 0 {
		fbScore = 0
	}
	v.Score += int(fbScore)
	if r.FallbackBehaviour.FallbackRate > 0.25 {
		v.WarningIssues = append(v.WarningIssues,
			"Elevated fallback rate ("+formatPct(r.FallbackBehaviour.FallbackRate)+") — primary searcher unreliable.")
	}
}

// scoreReplayCompleteness scores trace completeness (component 3, 20 pts).
func scoreReplayCompleteness(v *TrustVerdict, r *Report) {
	completenessScore := r.ReplayHealth.CompletenessRate * 20
	v.Score += int(completenessScore)
	if r.ReplayHealth.TracesProduced > 0 && r.ReplayHealth.CompletenessRate < 0.95 {
		v.BlockingIssues = append(v.BlockingIssues,
			"Replay traces incomplete ("+formatPct(r.ReplayHealth.CompletenessRate)+") — observability substrate broken.")
	}
}

// scoreFailureModes scores the failure-mode pass rate (component 4, 10 pts).
func scoreFailureModes(v *TrustVerdict, r *Report) {
	if len(r.FailureModes) == 0 {
		return
	}
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

// scoreComplianceLeaks awards 10 pts only when there are zero compliance/hidden
// leaks across all prompts; any leak is a blocking issue (component 5).
func scoreComplianceLeaks(v *TrustVerdict, r *Report) {
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
}
