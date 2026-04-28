package agentloop

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Agent OS — Prometheus metrics.
// All metrics are prefixed with "agentloop_" to avoid collision.
var (
	// runsTotal counts completed agent runs by final state and domain.
	runsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentloop_runs_total",
		Help: "Completed agent loop runs partitioned by final state and domain",
	}, []string{"state", "domain"})

	// stepDuration tracks how long each phase takes.
	stepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentloop_step_duration_seconds",
		Help:    "Duration of each agent step (planner/architect/executor/verifier)",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"step"})

	// verifyScore records the last verification score per domain.
	// Allows Grafana to alert when score drops below VerifyPassThreshold.
	verifyScore = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agentloop_verify_score",
		Help: "Last verification score (0-1) per domain",
	}, []string{"domain"})

	// patchesTotal counts individual patch outcomes.
	patchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentloop_patches_total",
		Help: "Individual patches by result: applied|failed|poison|skipped",
	}, []string{"result"})

	// iterationsTotal counts how many re-think iterations were needed.
	iterationsHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "agentloop_iterations",
		Help:    "Re-think iterations per successful/failed run",
		Buckets: []float64{1, 2, 3, 4, 5},
	})

	// rollbacksTotal counts production rollbacks triggered by failed verification.
	rollbacksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentloop_rollbacks_total",
		Help: "Production rollbacks triggered after sandbox build or verifier failure",
	})

	// buildFailuresTotal counts how often the sandbox go build step rejects a patch batch.
	buildFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentloop_build_failures_total",
		Help: "Sandbox go build failures (patches rejected before production apply)",
	})

	// humanEscalationsTotal counts times confidence thresholds triggered human escalation.
	humanEscalationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentloop_human_escalations_total",
		Help: "Human escalations by phase: planner|architect",
	}, []string{"phase"})
)

func metricRun(state AgentState, domain Domain) {
	runsTotal.WithLabelValues(string(state), string(domain)).Inc()
}

func metricStep(step string, durationSec float64) {
	stepDuration.WithLabelValues(step).Observe(durationSec)
}

func metricVerifyScore(domain Domain, score float64) {
	verifyScore.WithLabelValues(string(domain)).Set(score)
}

func metricPatch(result string) {
	patchesTotal.WithLabelValues(result).Inc()
}

func metricIterations(n int) {
	iterationsHistogram.Observe(float64(n))
}

func metricRollback() {
	rollbacksTotal.Inc()
}

func metricBuildFailure() {
	buildFailuresTotal.Inc()
}

func metricHumanEscalation(phase string) {
	humanEscalationsTotal.WithLabelValues(phase).Inc()
}
