package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SessionsGauge tracks browser_sessions grouped by status.
	SessionsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "thg_browser_sessions",
			Help: "Number of browser sessions by status",
		},
		[]string{"status"},
	)

	// JobsTotal counts jobs processed by the worker.
	JobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thg_jobs_total",
			Help: "Total jobs processed by intent and final status",
		},
		[]string{"intent", "status"},
	)

	// JobDuration measures end-to-end job latency.
	JobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "thg_job_duration_seconds",
			Help:    "Job execution duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"intent"},
	)

	// ContainerRestarts counts container restart events.
	ContainerRestarts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thg_container_restarts_total",
			Help: "Container restart events by reason",
		},
		[]string{"reason"},
	)

	// LeadsExtracted counts leads extracted by score category.
	LeadsExtracted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thg_leads_extracted_total",
			Help: "Leads extracted by score category",
		},
		[]string{"score"},
	)

	// CDPErrors counts CDPError occurrences by error code.
	CDPErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thg_cdp_errors_total",
			Help: "CDP/browser errors by error code",
		},
		[]string{"code"},
	)

	// CircuitBreakerOpen indicates whether a circuit breaker scope is open.
	CircuitBreakerOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "thg_circuit_breaker_open",
			Help: "1 if circuit breaker is open for the given scope",
		},
		[]string{"scope"},
	)

	// AllocationAttempts counts session allocation attempts by result.
	AllocationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thg_allocation_attempts_total",
			Help: "Session allocation attempts by outcome (acquired|no_session|error)",
		},
		[]string{"outcome"},
	)

	// HealthCheckResults counts health check outcomes.
	HealthCheckResults = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thg_health_check_results_total",
			Help: "Health check results by outcome (healthy|unhealthy)",
		},
		[]string{"outcome"},
	)
)
